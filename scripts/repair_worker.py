"""Consume the Barn repair queue from the trusted WSL runner machine."""

import argparse
import base64
from datetime import datetime
import json
import os
from pathlib import Path
import pwd
import subprocess
import tempfile
import time
from urllib.error import URLError
from urllib.request import Request, urlopen

from repair_provider import classify

from repair_completion import companion_pull, finish_companion, wait_for_repair_checks


BARN = "MongooseMoo/barn"
SUITE = "MongooseMoo/moo-conformance-tests"
STAGES = ("conformance", "repair", "investigation")
RESULT = {
    "type": "object",
    "properties": {key: {"type": "string"} for key in ("outcome", "title", "evidence")},
    "required": ["outcome", "title", "evidence"],
    "additionalProperties": False,
}
PUBLISHER_TOKEN = None


class PublisherToken:
    def __init__(self, config):
        self.config = config
        self.token = None
        self.expires = 0

    def get(self):
        now = int(time.time())
        if self.expires > now + 120:
            return self.token
        def encode(value):
            return base64.urlsafe_b64encode(value).rstrip(b"=")
        header = encode(b'{"alg":"RS256","typ":"JWT"}')
        claims = encode(json.dumps({"iat": now - 60, "exp": now + 480,
                                    "iss": self.config["client_id"]}).encode())
        unsigned = header + b"." + claims
        signed = subprocess.run(["openssl", "dgst", "-sha256", "-sign", self.config["key_file"]],
                                input=unsigned, capture_output=True, check=True, timeout=10).stdout
        jwt = (unsigned + b"." + encode(signed)).decode()
        installation = int(self.config["installation_id"])
        request = Request(f"https://api.github.com/app/installations/{installation}/access_tokens",
                          data=b'{"repositories":["barn","moo-conformance-tests"]}',
                          headers={"Authorization": f"Bearer {jwt}", "Accept": "application/vnd.github+json",
                                   "Content-Type": "application/json", "User-Agent": "Barn repair worker"})
        with urlopen(request, timeout=30) as response:
            result = json.load(response)
        self.token = result["token"]
        self.expires = datetime.fromisoformat(result["expires_at"].replace("Z", "+00:00")).timestamp()
        return self.token


def run(*args, cwd=None, user=None, data=None, timeout=120, cache=None):
    options = {}
    if user:
        account = pwd.getpwnam(user)
        home = account.pw_dir
        options = {
            "user": account.pw_uid, "group": account.pw_gid, "extra_groups": [],
            "env": {"HOME": home, "USER": user, "LOGNAME": user, "LANG": "C.UTF-8",
                    "PATH": "/usr/local/libexec/barn-codex:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
                    "GIT_TERMINAL_PROMPT": "0", "TOAST_MOO": "/usr/local/libexec/barn-codex/moo"},
        }
        if cache:
            options["env"].update(GOCACHE=str(cache / "go-build"), GOPATH=str(cache / "go"),
                                  UV_CACHE_DIR=str(cache / "uv"),
                                  UV_PROJECT_ENVIRONMENT=str(cache / "conformance"),
                                  CONFORMANCE_ENV=str(cache / "conformance"))
    elif PUBLISHER_TOKEN and args[0] in {"gh", "git"}:
        options = {"env": {**os.environ, "GH_TOKEN": PUBLISHER_TOKEN.get(),
                           "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null"}}
    result = subprocess.run(args, cwd=cwd, input=data, text=True, check=True,
                            capture_output=True, timeout=timeout, **options)
    return result.stdout


def api(path, *args):
    output = run("gh", "api", path, *args)
    return json.loads(output) if output else None


def label(number, name, add):
    endpoint = f"repos/{BARN}/issues/{number}/labels"
    if add:
        api(endpoint, "-X", "POST", "-f", f"labels[]={name}")
    elif name in {entry["name"] for entry in api(endpoint)}:
        api(f"{endpoint}/{name}", "-X", "DELETE", "--silent")


def git(directory, *args, user=None, data=None):
    return run("git", "-c", "core.hooksPath=/dev/null", *args, cwd=directory, user=user, data=data)


def clone(directory, repository, branch, user=None):
    git(directory.parent, "clone", "--quiet", "--branch", branch,
        f"https://github.com/{repository}.git", str(directory), user=user)


def codex_profile(home):
    # The managed harness listens and connects on 127.0.0.1. The proxy also
    # lets Go and uv restore build dependencies in the disposable checkout.
    domains = ("127.0.0.1", "proxy.golang.org", "sum.golang.org", "storage.googleapis.com",
               "pypi.org", "files.pythonhosted.org")
    allowlist = ",".join(f'{json.dumps(domain)}="allow"' for domain in domains)
    credentials = json.dumps(str(Path(home) / ".codex" / "auth.json"))
    return (
        "-c", 'forced_login_method="chatgpt"',
        "-c", 'default_permissions="barn"',
        "-c", 'permissions.barn.extends=":workspace"',
        "-c", "features.network_proxy=true",
        "-c", "permissions.barn.network.enabled=true",
        "-c", f"permissions.barn.network.domains={{{allowlist}}}",
        "-c", f'permissions.barn.filesystem={{{credentials}="deny"}}',
    )


def proposal(root, stage, number, report, user, codex):
    account = pwd.getpwnam(user)
    with tempfile.TemporaryDirectory(dir=account.pw_dir, prefix="job-") as temp:
        work = Path(temp)
        os.chown(work, account.pw_uid, account.pw_gid)
        barn = work / "barn"
        suite = work / "moo-conformance-tests"
        cache = work / "cache"
        clone(barn, BARN, "master", user=user)
        if stage == "conformance":
            pr = report["pull"]
            git(barn, "fetch", "origin", f"refs/pull/{number}/head", user=user)
            head = pr["merge_commit_sha"] if pr["merged"] else pr["head"]["sha"]
            git(barn, "checkout", "--detach", head, user=user)
        if stage in {"repair", "conformance"}:
            run("go", "mod", "download", cwd=barn, user=user, cache=cache, timeout=300)
            clone(suite, SUITE, "main", user=user)
            git(suite, "submodule", "update", "--init", "--recursive", user=user)
            # The frozen suite includes a Git dependency. Fetch it before the model's network sandbox.
            run("uv", "sync", "--frozen", "--no-python-downloads", cwd=suite, user=user,
                cache=cache, timeout=300)
        baseline = git(barn, "rev-parse", "HEAD", user=user).strip()
        prompt = (root / ".github" / "repair-prompts" / f"{stage}.md").read_text(encoding="utf-8")
        for name, contents in (("finding.json", report), ("result-schema.json", RESULT)):
            with (work / name).open("x", encoding="utf-8") as output:
                os.fchmod(output.fileno(), 0o600)
                os.fchown(output.fileno(), account.pw_uid, account.pw_gid)
                json.dump(contents, output)
        try:
            stream = run(codex, "-a", "never", *codex_profile(account.pw_dir),
                         "exec", "--json", "--add-dir", str(work),
                         "--output-schema", str(work / "result-schema.json"),
                         "--output-last-message", str(work / "result.json"), "-",
                         cwd=barn, user=user, data=prompt, timeout=10800, cache=cache)
        except subprocess.CalledProcessError as error:
            reason, reset = classify(error.stdout or "", "codex")
            return {"outcome": reason, "retry_at": reset}, "", baseline
        if not (work / "result.json").is_file():
            reason, reset = classify(stream, "codex")
            return {"outcome": reason, "retry_at": reset}, "", baseline
        result = json.loads(run("/usr/bin/cat", str(work / "result.json"), user=user))
        target = suite if stage == "conformance" else barn
        if result["outcome"] not in {"patch", "test"}:
            return result, "", baseline
        base = baseline if target == barn else "origin/main"
        paths = git(target, "ls-files", "--others", "--exclude-standard", "-z", user=user).split("\0")[:-1]
        if paths:
            git(target, "--literal-pathspecs", "add", "--intent-to-add", "--", *paths, user=user)
        patch = git(target, "diff", "--no-ext-diff", "--binary", base, "--", user=user)
        if not patch:
            raise ValueError("The worker reported a change but left an empty diff")
        return result, patch, baseline


def open_pr(repository, branch, base, result, patch, number, expected=None, tested_head=None):
    owner = repository.split("/")[0]
    existing = api(f"repos/{repository}/pulls?state=open&head={owner}:{branch}")
    if existing:
        if tested_head and f"<!-- barn-tested-head: {tested_head} -->" not in (existing[0]["body"] or ""):
            raise ValueError("The existing conformance PR has no result for the current Barn head")
        return existing[0]["number"]
    with tempfile.TemporaryDirectory(prefix="barn-publish-") as temp:
        directory = Path(temp) / "repository"
        clone(directory, repository, base)
        tip = git(directory, "rev-parse", "HEAD").strip()
        if expected and tip != expected:
            git(directory, "merge-base", "--is-ancestor", expected, tip)
            git(directory, "checkout", "--detach", expected)
        git(directory, "apply", "--index", "--binary", "-", data=patch)
        if repository == SUITE:
            allow = "src/moo_conformance/_db/startup/startup-fixtures.sha256"
            for change in git(directory, "diff", "--cached", "--name-status").splitlines():
                status, path = change.split("\t", 1)
                if path == allow or (status == "A" and path.startswith(("src/moo_conformance/_tests/", "src/moo_conformance/_db/"))):
                    continue
                raise ValueError("The conformance proposal changes a file outside the test additions")
        git(directory, "-c", "user.name=Barn repair", "-c", "user.email=actions@users.noreply.github.com",
            "commit", "-m", result["title"].strip())
        ref = f"refs/heads/{branch}"
        if git(directory, "ls-remote", "--heads", "origin", ref).strip():
            git(directory, "fetch", "origin", ref)
            fresh_tree = git(directory, "rev-parse", "HEAD^{tree}")
            if git(directory, "rev-parse", "FETCH_HEAD^{tree}") != fresh_tree:
                raise ValueError("A previous attempt left a different change on the repair branch")
        else:
            git(directory, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential",
                "push", "origin", f"HEAD:{ref}")
    body = result["evidence"].strip()
    body += f"\n\nFrom {BARN}#{number}." if repository == SUITE else f"\n\nFixes #{number}."
    if tested_head:
        body += f"\n\n<!-- barn-tested-head: {tested_head} -->"
    args = ["-X", "POST", "-f", f"head={branch}", "-f", f"base={base}",
            "-f", f"title={result['title']}", "-f", f"body={body}"]
    pr = api(f"repos/{repository}/pulls", *args)
    if repository == BARN and branch.startswith("codex/"):
        label(pr["number"], "automatic-fix", True)
    return pr["number"]


def managed_proof(number, head, suite_head, destination):
    response = api(f"repos/{BARN}/actions/workflows/repair-proof.yml/dispatches", "-X", "POST",
                   "-f", "ref=master", "-F", "return_run_details=true",
                   "-f", f"inputs[pr]={number}", "-f", f"inputs[barn_head]={head}",
                   "-f", f"inputs[suite_head]={suite_head}")
    run_id = (response or {}).get("workflow_run_id")
    if not isinstance(run_id, int):
        raise ValueError("GitHub did not return a managed proof run id")
    print(f"Repair #{number}: managed proof https://github.com/{BARN}/actions/runs/{run_id}", flush=True)
    # Each request can renew its installation token while the accepted run
    # waits for a runner or executes. The receipt remains the same.
    deadline = time.monotonic() + 7200
    while True:
        result = api(f"repos/{BARN}/actions/runs/{run_id}")
        status = result["status"]
        if status == "completed":
            conclusion = result["conclusion"]
            if conclusion != "success":
                raise ValueError(f"Managed proof run {run_id} completed with {conclusion}")
            break
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ValueError(f"Managed proof run {run_id} was still {status} after two hours")
        time.sleep(min(30, remaining))
    run("gh", "run", "download", str(run_id), "--repo", BARN, "--name", "conformance-evidence",
        "--dir", str(destination))
    evidence = destination / "conformance-evidence.json"
    if not evidence.is_file():
        raise ValueError("The managed proof run did not publish conformance evidence")
    return evidence


def queue_repair(root, pr, suite_pr=None):
    number = pr["number"]
    head = pr["head"]["sha"]
    suite_head = None
    with tempfile.TemporaryDirectory(prefix="barn-scope-") as temp:
        workspace = Path(temp)
        directory = workspace / "barn"
        clone(directory, BARN, "master")
        git(directory, "fetch", "origin", f"refs/pull/{number}/head")
        if git(directory, "rev-parse", "FETCH_HEAD").strip() != head:
            print(f"Repair #{number}: the Barn head moved before validation", flush=True)
            return False
        base = git(directory, "merge-base", "HEAD", head).strip()
        scope_args = ["--base", base, "--head", head]
        try:
            run("python3", str(root / "scripts" / "automatic_fix_scope.py"),
                *scope_args, cwd=directory)
        except subprocess.CalledProcessError as error:
            if error.returncode != 1:
                raise
            if suite_pr is None:
                return False
            suite_head = api(f"repos/{SUITE}/pulls/{suite_pr}")["head"]["sha"]
            try:
                evidence = managed_proof(number, head, suite_head, workspace)
                run("python3", str(root / "scripts" / "automatic_fix_scope.py"), *scope_args,
                    "--suite-head", suite_head, "--conformance-evidence", str(evidence), cwd=directory)
            except ValueError as error:
                print(f"Repair #{number}: {error}", flush=True)
                return False
            except subprocess.SubprocessError:
                print(f"Repair #{number}: no accepted managed proof", flush=True)
                return False
    try:
        verdict = wait_for_repair_checks(api, number, head, suite_head)
    except ValueError as error:
        print(str(error), flush=True)
        return False
    if verdict != "ready":
        print(f"Repair #{number}: already {verdict}", flush=True)
        return True
    response = api("graphql", "-f", "query=mutation($id:ID!,$head:GitObjectID!){enqueuePullRequest(input:{pullRequestId:$id,expectedHeadOid:$head}){mergeQueueEntry{id}}}",
                   "-f", f"id={pr['node_id']}", "-f", f"head={head}")
    print(f"Repair #{number}: {response['data']['enqueuePullRequest']['mergeQueueEntry']['id']}", flush=True)
    return True


def finish_closed_companion(number):
    try:
        print(finish_companion(api, number), flush=True)
    except (ValueError, subprocess.SubprocessError) as error:
        print(f"Repair #{number}: {error}", flush=True)
        label(number, "needs-decision", True)


def process(args, stage, item):
    number = item["number"]
    label(number, "codex-running", True)
    report = {"item": item, "comments": api(f"repos/{BARN}/issues/{number}/comments")}
    if stage == "conformance":
        report["pull"] = api(f"repos/{BARN}/pulls/{number}")
        # A merged repair with a published companion only needs completion.
        # Include closed companions so a concurrent merge cannot restart
        # test generation when the existing handoff reaches the worker.
        if report["pull"]["merged"] and companion_pull(api, number, state="all"):
            finish_closed_companion(number)
            return True
    elif stage == "repair":
        for branch in (f"codex/issue-{number}", f"claude/issue-{number}"):
            existing = api(f"repos/{BARN}/pulls?state=open&head=MongooseMoo:{branch}")
            if existing:
                label(existing[0]["number"], "codex-conformance", True)
                return True
    result, patch, head = proposal(args.root, stage, number, report, args.user, args.codex)
    outcome = result["outcome"]
    print(f"{stage} #{number}: {outcome}", flush=True)
    attention = "needs-decision" if stage == "conformance" else "repair-needs-attention"
    if outcome == "unknown":
        label(number, attention, True)
        return True
    # A Codex policy block goes to Claude, as an outage does, unless Claude
    # already failed on this item (#1124 sat two days on a Codex cyber-policy
    # refusal with no repair).
    if outcome in {"policy-block", "usage-limit", "authentication", "provider-unavailable"}:
        if outcome != "policy-block":
            print(f"Codex availability: {outcome}, reset {result['retry_at'] or 'unknown'}", flush=True)
            if result["retry_at"].isdigit():
                args.retry_at = int(result["retry_at"])
        labels = {item["name"] for item in api(f"repos/{BARN}/issues/{number}/labels")}
        if "claude-unavailable" in labels and outcome == "policy-block":
            label(number, attention, True)
            return True
        if "claude-unavailable" not in labels:
            label(number, "codex-unavailable", True)
            label(number, f"codex-{stage}", False)
            workflow = "alarm-investigate.yml" if stage == "investigation" else "claude-fix.yml"
            fields = ["-f", "ref=master", "-f", f"inputs[issue_number]={number}", "-f", "inputs[provider]=claude"]
            if stage == "conformance":
                fields.extend(("-f", "inputs[stage]=conformance"))
            try:
                api(f"repos/{BARN}/actions/workflows/{workflow}/dispatches", "-X", "POST", *fields)
            except subprocess.CalledProcessError:
                label(number, f"codex-{stage}", True)
                raise
            return True
        return False
    label(number, "codex-unavailable", False)
    label(number, "claude-unavailable", False)
    if stage == "conformance" and outcome in {"test", "not-observable", "already-covered", "toast-disagrees"}:
        pr = api(f"repos/{BARN}/pulls/{number}")
        current = pr["merge_commit_sha"] if pr["merged"] else pr["head"]["sha"]
        if current != head:
            raise ValueError("Repair head changed during conformance; rerun for the current head")
        if outcome == "test":
            suite_pr = open_pr(SUITE, f"barn-fix/pr-{number}", "main", result, patch, number, tested_head=head)
            if pr["merged"]:
                finish_closed_companion(number)
        else:
            suite_pr = None
            api(f"repos/{BARN}/issues/{number}/comments", "-X", "POST", "-f", f"body={result['evidence']}")
        if not pr["merged"]:
            may_queue = outcome != "toast-disagrees" and queue_repair(args.root, pr, suite_pr)
            label(number, "needs-decision", not may_queue)
        return True
    if stage in {"repair", "investigation"} and outcome == "patch":
        branch = f"codex/issue-{number}" if stage == "repair" else f"alarm/issue-{number}"
        pr = open_pr(BARN, branch, "master", result, patch, number, expected=head)
        if stage == "repair":
            label(pr, "codex-conformance", True)
        return True
    if outcome not in {"completed", "not-planned", "duplicate", "unresolved"}:
        raise ValueError(f"Unexpected outcome {outcome} for {stage}")
    api(f"repos/{BARN}/issues/{number}/comments", "-X", "POST", "-f", f"body={result['evidence']}")
    if outcome in {"completed", "not-planned"} and stage != "conformance":
        reason = "completed" if outcome == "completed" else "not_planned"
        api(f"repos/{BARN}/issues/{number}", "-X", "PATCH", "-f", "state=closed", "-f", f"state_reason={reason}")
    else:
        label(number, "repair-needs-attention", True)
    return True


def cycle(args):
    for stage in STAGES:
        queue = api(f"repos/{BARN}/issues?state=all&labels=codex-{stage}&sort=created&direction=asc&per_page=100")
        for item in queue:
            number = item["number"]
            if args.number and number != args.number:
                continue
            if stage != "conformance" and item["state"] != "open":
                label(number, f"codex-{stage}", False)
                continue
            try:
                done = process(args, stage, item)
                if done:
                    label(number, f"codex-{stage}", False)
            finally:
                label(number, "codex-running", False)
            return done
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--number", type=int, help="Only take this queued issue or PR")
    parser.add_argument("--interval", type=int, help="Poll indefinitely at this interval in seconds")
    parser.add_argument("--user", default="barn-codex")
    parser.add_argument("--codex", default="/usr/local/libexec/barn-codex/codex")
    parser.add_argument("--publisher", type=Path, default=Path("/etc/barn-repair/github-app.json"))
    parser.add_argument("--initialize-labels", action="store_true")
    args = parser.parse_args()
    args.root = Path(__file__).resolve().parents[1]
    args.retry_at = 0
    if os.geteuid() != 0:
        parser.error("Run as the trusted publisher account (root), with Codex in --user")
    global PUBLISHER_TOKEN
    if not args.publisher.is_file():
        parser.error(f"Missing GitHub App publisher configuration at {args.publisher}")
    PUBLISHER_TOKEN = PublisherToken(json.loads(args.publisher.read_text(encoding="utf-8")))
    if args.initialize_labels:
        labels = {
            "codex-repair": "Queued for a Barn patch from Codex",
            "codex-conformance": "Queued for managed conformance from Codex",
            "codex-investigation": "Queued for an alarm investigation from Codex",
            "codex-running": "The Codex worker is taking this item",
            "codex-unavailable": "Codex reported a provider error for this item",
            "claude-unavailable": "Claude reported a provider error for this item",
            "repair-needs-attention": "The repair needs a maintainer decision",
        }
        existing = api(f"repos/{BARN}/labels?per_page=100")
        names = {item["name"] for item in existing}
        for name, description in labels.items():
            if name not in names:
                api(f"repos/{BARN}/labels", "-X", "POST", "-f", f"name={name}",
                    "-f", f"description={description}", "-f", "color=17575b")
        return 0
    while True:
        try:
            ok = cycle(args)
        except (subprocess.SubprocessError, ValueError, URLError) as error:
            if isinstance(error, subprocess.CalledProcessError):
                message = f"{error.cmd[0]} exited {error.returncode}"
            else:
                message = f"{type(error).__name__}: {error}"
            print(f"Repair worker could not complete its stage: {message}", flush=True)
            ok = False
        if not args.interval:
            return int(not ok)
        time.sleep(max(args.interval, args.retry_at - time.time(), 0 if ok else 900))
        args.retry_at = 0


if __name__ == "__main__":
    raise SystemExit(main())
