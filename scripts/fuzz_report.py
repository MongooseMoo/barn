"""Summarize Go fuzz output and report it as GitHub issues, without
interpreting or executing corpus contents."""

import argparse
import base64
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import sys
import urllib.error
import urllib.request


INLINE_LIMIT = 8000
TAIL_LIMIT = 12000
# The exact record travels base64-encoded in the issue (4/3 growth), beside the
# inline text and log tail, under GitHub's 65536-character body limit.
RECORD_LIMIT = 30000
# Exit status when a record is too large to report before its artifact exists.
DEFERRED = 3
MODULE = "github.com/MongooseMoo/barn/"


def check_message(line):
    """A failed check's file:line and message, without the input values it embeds."""
    file_line, message = line.split(": ", 1)
    message = re.sub(r'"(?:[^"\\]|\\.)*"', "S", message)
    return f"{file_line}: {re.sub(r'[0-9]+', 'N', message)}"


def report(log, package, target, commit, campaign_outcome="failure"):
    # gh --log-failed fixtures contain multiple jobs; a live tee log has no prefix.
    lines = []
    for line in log.splitlines():
        match = re.match(r"([^\t]+)\t[^\t]+\t\d{4}-\d\d-\d\dT\S+ (.*)", line)
        if match:
            if match[1] != target:
                continue
            line = match[2]
        lines.append(line)
    text = "\n".join(lines)
    signal = re.search(r"^.*(?:signal:|terminated by unexpected signal|Killed\s+).*?$", text, re.M)
    panic = re.search(r"^.*(?:panic:|fatal error:).*?$", text, re.M)
    hang = re.search(r"^.*fuzzing process hung or terminated unexpectedly:.*$", text, re.M)
    check = re.search(r"^\s*(\S+\.go:\d+): .+$", text, re.M)
    saved = re.search(r"Failing input written to (testdata/fuzz/[^\s]+)", text)
    setup = re.search(r"^.*\[(?:build|setup) failed\].*$", text, re.M)
    # scripts/fuzz_differential.py's report: the first diverging program.
    divergence = re.search(r"^(## divergence \(\d+\))\n\n### \S+ `([^`]+)`\n\n```moo\n(.*?)\n```\n\n"
                           r"(- toast: [^\n]*)\n(- barn: [^\n]*)", text, re.M | re.S)
    # A count mismatch prevents source attribution, but the differential
    # producer still preserves explicit recovered Barn panics as JSON.
    recovered_panics = []
    if target == "ToastDifferential":
        recovered = re.search(r"^- recovered Barn panics: (.*)$", text, re.M)
        if recovered:
            try:
                records = json.loads(recovered[1])
            except json.JSONDecodeError:
                records = None
            if isinstance(records, list):
                recovered_panics = [record for record in records if isinstance(record, str)
                                    and record.startswith("Error: internal error")]
    # A recorded input is a finding, even when its worker died from a signal.
    # Program sources in a differential report are not Go output, so it is
    # classified before anything that could match their text.
    if divergence and campaign_outcome == "failure":
        kind, error = "divergence", None
    elif recovered_panics and campaign_outcome == "failure":
        kind, error = "panic", None
    elif not saved and (campaign_outcome != "failure" or signal or setup):
        kind, error = "infrastructure", signal
    elif panic:
        kind, error = "panic", panic
    elif saved and signal:
        kind, error = "crash signal", signal
    elif hang:
        kind, error = "hang", hang
    elif saved or check:
        kind, error = "failed check", check
    else:
        kind, error = "infrastructure", None
    if error:
        first_error = error[0].strip()
    elif kind == "divergence":
        # The oracle is built in the job, so the issue names that binary.
        toast = re.search(r"^- toast binary: .*$", text, re.M)
        first_error = "\n".join([divergence[1], divergence[4], divergence[5]] + ([toast[0]] if toast else []))
    elif recovered_panics and kind == "panic":
        first_error = "\n".join(recovered_panics)
    else:
        first_error = check[0].strip() if setup and check else next((line.strip() for line in lines if re.search(
            r"(?:error:|Error:|FAIL|build failed|setup failed|cannot |no required module)", line)),
            "The job failed outside the fuzz command; inspect the failed step.")

    frames = []
    for index, line in enumerate(lines):
        if line.strip().startswith(MODULE):
            # Strip argument values (addresses change between runs), keeping receivers.
            function = re.sub(r"\([^()]*\)$", "", line.strip())
            location = lines[index + 1].strip() if index + 1 < len(lines) else ""
            location = re.sub(r"^.*?/barn/barn/", "", location).split(" +0x")[0]
            frames.append({"function": function, "location": location})
            if len(frames) == 5:
                break
    top = frames[0]["function"] if frames else "unavailable"
    input_path, input_text, repro = None, None, None
    if kind == "divergence":
        # The first diverging corpus file stands in for the top frame.
        top, input_path, input_text = divergence[2], divergence[2], divergence[3][:INLINE_LIMIT]
    elif recovered_panics and kind == "panic" and not frames:
        # Deduplicate by observed native error, without assigning a source/frame.
        top = recovered_panics[0]
    elif kind == "failed check" and check and not frames:
        # A failed check prints no stack, so its file:line and message tell
        # findings in one target apart: one check line can report many bugs.
        top = check_message(check[0].strip())
    signature = hashlib.sha256(f"{target}|{kind}|{top}".encode()).hexdigest()
    if saved:
        relative = Path(saved[1])
        # Only read the corpus file named by Go, inside this target's directory.
        if relative.parent == Path("testdata/fuzz") / target:
            input_path = (Path(package) / relative).as_posix()
            try:
                with Path(input_path).open("rb") as stream:
                    data = stream.read(INLINE_LIMIT + 1)
                input_text = data[:INLINE_LIMIT].decode("utf-8", errors="replace")
                if len(data) > INLINE_LIMIT:
                    input_text += "\n[truncated]"
            except OSError as exc:
                input_text = f"Input unavailable: {exc.strerror}"
            repro = (f"timeout --kill-after=5s 60s go test {shlex.quote(package)} "
                     f"-run '{target}/{relative.name}'")
    version = re.search(r"^go version .+$", text, re.M)
    return {
        "kind": kind, "first_error": first_error, "barn_frames": frames,
        "signature": signature, "input_path": input_path, "input_text": input_text,
        "repro": repro, "commit": commit,
        "go_version": version[0] if version else "unavailable",
        "log_tail": "\n".join(lines[-80:])[-TAIL_LIMIT:],
    }


def summarize(logs, package, target, commit, campaign_outcome="failure"):
    """One report per distinct signature, in first-seen order, with its count."""
    distinct = {}
    for log in logs or [""]:
        result = report(log, package, target, commit, campaign_outcome)
        first = distinct.setdefault(result["signature"], {**result, "count": 0})
        first["count"] += 1
    return list(distinct.values())


def saved_record(result):
    """The exact bytes of the record Go saved for this report, or None."""
    # report() sets repro only for a record Go wrote in this target's testdata.
    if not result.get("repro") or not result.get("input_path"):
        return None
    try:
        return Path(result["input_path"]).read_bytes()
    except OSError:
        return None


def block(value, language="text"):
    # A corpus can contain Markdown fences; keep its contents inside the block.
    text = str("Unavailable" if value is None else value)
    fence = "`" * max([3] + [len(run) + 1 for run in re.findall(r"`+", text)])
    return f"{fence}{language}\n{text}\n{fence}"


def issue_body(result, package, target, run_url, artifact_url):
    record = saved_record(result)
    if record is None:
        exact = None
    elif len(record) > RECORD_LIMIT:
        exact = (f"Exact failing record: {len(record)} bytes, too large for an issue; "
                 "use the failing-input artifact.")
    else:
        encoded = base64.b64encode(record).decode()
        exact = (f"Exact failing record {Path(result['input_path']).name}: {len(record)} bytes, "
                 f"sha256 {hashlib.sha256(record).hexdigest()}. Decode this block with base64 -d "
                 f"into {result['input_path']} and check its sha256:\n"
                 + block("\n".join(encoded[i:i + 76] for i in range(0, len(encoded), 76)), "base64"))
    frames = "\n".join(f"{f['function']}\n  {f['location']}" for f in result["barn_frames"])
    return "\n\n".join(part for part in [
        f"<!-- fuzz-signature: {result['signature']} -->",
        f"Target: {package}/{target}",
        f"Run: {run_url}",
        f"Kind: {result['kind']}",
        f"Campaigns in this report that found it: {result['count']} (the input below is from the first).",
        f"Commit: {result['commit']}\nGo: {result['go_version']}",
        f"First error:\n{block(result['first_error'])}",
        f"Top Barn frames:\n{block(frames or 'Unavailable in Go output')}",
        f"Failing input ({result['input_path'] or 'none recorded'}):\n{block(result['input_text'])}",
        exact,
        f"Bounded reproduction:\n{block(result['repro'] or 'No failing input recorded; inspect the log.')}",
        f"Log tail:\n{block(result['log_tail'])}",
        f"Failing inputs: {artifact_url} (retained for 30 days)." if artifact_url
        else f"Saved inputs are uploaded as artifact fuzz-failure-{target} when this job ends.",
        "Preserve confirmed failing inputs with their regression fixes.",
    ] if part)


def utc(stamp):
    return datetime.fromisoformat(stamp.replace("Z", "+00:00"))


def publish(reports, api, package, target, run_url, artifact_url=None, immediate=False):
    """Open an issue (and dispatch its repair) or comment, per signature.

    A signature whose issue was closed after the fuzzed commit was made is
    commented on that issue: the fix that closed it is not in the commit. A
    signature whose issue closed before the commit is a regression and gets a
    new issue.

    With immediate, nothing is posted when a record is too large to travel in
    the issue: it is reported when the job ends, with its uploaded artifact.
    """
    if immediate and any(len(record) > RECORD_LIMIT for record in map(saved_record, reports)
                         if record is not None):
        return DEFERRED
    issues, page = [], 1
    while True:
        batch = api("GET", f"/issues?state=all&per_page=100&page={page}")
        issues += [issue for issue in batch if "pull_request" not in issue]
        if len(batch) < 100:
            break
        page += 1
    commit_times = {}
    for result in reports:
        infrastructure = result["kind"] == "infrastructure"
        body = issue_body(result, package, target, run_url, artifact_url)
        marker = body.split("\n", 1)[0]
        matching = [issue for issue in issues if marker in (issue.get("body") or "")]
        existing = next((issue for issue in matching if issue.get("state") != "closed"), None)
        if existing:
            api("POST", f"/issues/{existing['number']}/comments", {"body": body})
            continue
        closed = [issue for issue in matching if issue.get("closed_at")]
        if closed:
            commit = result["commit"]
            if commit not in commit_times:
                commit_times[commit] = utc(api("GET", f"/commits/{commit}")["commit"]["committer"]["date"])
            latest = max(closed, key=lambda issue: utc(issue["closed_at"]))
            if utc(latest["closed_at"]) > commit_times[commit]:
                note = (f"Commit {commit} predates this issue's closing, so it does not have the fix; "
                        "no new issue or repair was started.")
                api("POST", f"/issues/{latest['number']}/comments", {"body": f"{note}\n\n{body}"})
                continue
        title = (f"Nightly fuzz infrastructure failure: {target}" if infrastructure
                 else f"Nightly fuzz failure: {target}")
        issue = api("POST", "/issues", {"title": title, "body": body,
                                        "labels": [] if infrastructure else ["claude-fix"]})
        issues.append(issue)
        if not infrastructure:
            # GITHUB_TOKEN events start no other workflows, except
            # workflow_dispatch, so dispatch the fix run explicitly.
            api("POST", "/actions/workflows/claude-fix.yml/dispatches",
                {"ref": "master", "inputs": {"issue_number": str(issue["number"])}})
    return 0


def github_api():
    """REST calls on this repository with the job's token."""
    base = f"{os.environ['GITHUB_API_URL']}/repos/{os.environ['GITHUB_REPOSITORY']}"
    headers = {"Authorization": f"Bearer {os.environ['GITHUB_TOKEN']}",
               "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}

    def call(method, path, payload=None):
        data = None if payload is None else json.dumps(payload).encode()
        request = urllib.request.Request(base + path, data=data, method=method,
                                         headers={**headers, "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                text = response.read()
        except urllib.error.HTTPError as error:
            raise RuntimeError(f"{method} {path}: HTTP {error.code}: "
                               f"{error.read().decode(errors='replace')}") from error
        return json.loads(text) if text else None
    return call


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    # One log per failed campaign not yet reported, in run order.
    parser.add_argument("--log", action="append", default=[], type=Path)
    parser.add_argument("--package", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--campaign-outcome", required=True)
    parser.add_argument("--publish", action="store_true", help="open issues and dispatch repairs")
    parser.add_argument("--immediate", action="store_true",
                        help=f"exit {DEFERRED} without posting if a record cannot travel in the issue")
    # Logs this job already reported: their signatures are not posted again.
    parser.add_argument("--reported-log", action="append", default=[], type=Path)
    parser.add_argument("--artifact-url", default="")
    args = parser.parse_args()
    read = lambda paths: [log.read_text(errors="replace") for log in paths if log.exists()]
    logs, reported = read(args.log), read(args.reported_log)
    if reported and not logs:
        # Every failed campaign was already reported while the lane ran.
        print("[]")
        return 0
    reports = summarize(logs, args.package, args.target, args.commit, args.campaign_outcome)
    seen = {r["signature"] for r in summarize(reported, args.package, args.target, args.commit)} if reported else set()
    reports = [r for r in reports if r["signature"] not in seen]
    print(json.dumps(reports))
    if not args.publish or not reports:
        return 0
    run_url = (f"{os.environ['GITHUB_SERVER_URL']}/{os.environ['GITHUB_REPOSITORY']}/actions/runs/"
               f"{os.environ['GITHUB_RUN_ID']}/attempts/{os.environ['GITHUB_RUN_ATTEMPT']}")
    return publish(reports, github_api(), args.package, args.target, run_url,
                   args.artifact_url or None, args.immediate)


if __name__ == "__main__":
    sys.exit(main())
