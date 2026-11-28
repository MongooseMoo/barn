"""Finish the suite PR for a closed Barn repair under GitHub's branch rules."""

import argparse
import json
import subprocess
import time


BARN = "MongooseMoo/barn"
SUITE = "MongooseMoo/moo-conformance-tests"
REJECTED = {"failure", "timed_out", "cancelled", "action_required"}


def required_check_state(api, repository, branch, head):
    protection = api(f"repos/{repository}/branches/{branch}/protection/required_status_checks")
    required = protection["checks"]
    if not required:
        raise ValueError(f"{repository} has no required checks to verify")
    checks = api(f"repos/{repository}/commits/{head}/check-runs?per_page=100&filter=latest")["check_runs"]
    pending = False
    failed = []
    for requirement in required:
        matching = [run for run in checks if run["name"] == requirement["context"] and
                    requirement["app_id"] in (None, -1, run["app"]["id"])]
        failed.extend(run["html_url"] for run in matching if run["conclusion"] in REJECTED)
        if not any(run["conclusion"] == "success" for run in matching):
            pending = True
    return failed, pending


def companion_pull(api, number):
    pulls = api(f"repos/{SUITE}/pulls?state=open&head=MongooseMoo:barn-fix/pr-{number}&base=main")
    return pulls[0] if pulls else None


def auto_merge(api, pr):
    return api("graphql", "-f",
               "query=mutation($id:ID!,$head:GitObjectID!){enablePullRequestAutoMerge(input:"
               "{pullRequestId:$id,expectedHeadOid:$head,mergeMethod:MERGE}){clientMutationId}}",
               "-f", f"id={pr['node_id']}", "-f", f"head={pr['head']['sha']}")


def finish_companion(api, repair, timeout=4200):
    barn = api(f"repos/{BARN}/pulls/{repair}")
    if barn["state"] != "closed":
        raise ValueError(f"Barn #{repair} is still open")
    companion = companion_pull(api, repair)
    if companion is None:
        return f"No open conformance pull request for Barn #{repair}."
    number = companion["number"]
    endpoint = f"repos/{SUITE}/pulls/{number}"
    companion = api(endpoint)
    if companion["merged"]:
        return f"Suite #{number} merged at {companion['merge_commit_sha']}."
    if companion["state"] != "open" or companion["head"]["repo"]["full_name"] != SUITE:
        raise ValueError(f"Suite #{number} is not a branch of {SUITE}")
    if not barn["merged"]:
        api(endpoint, "-X", "PATCH", "-f", "state=closed")
        return f"Barn #{repair} closed unmerged. Closed companion #{number}."

    allow = "src/moo_conformance/_db/startup/startup-fixtures.sha256"
    # Bound the request and verify we saw every changed path, including on a
    # surprisingly large companion. The compare performed later binds its head.
    files = api(f"{endpoint}/files?per_page=100")
    if len(files) != companion["changed_files"] or any(
        change["filename"] != allow and (
            change["status"] != "added" or
            not change["filename"].startswith(("src/moo_conformance/_tests/", "src/moo_conformance/_db/"))
        ) for change in files
    ):
        raise ValueError(f"Suite #{number} has changes outside the allowed test additions")
    if api(endpoint)["head"]["sha"] != companion["head"]["sha"]:
        raise ValueError(f"Suite #{number} changed during the file check")

    head = companion["head"]["sha"]
    requested = None
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        current = api(endpoint)
        current_head = current["head"]["sha"]
        if current_head != head:
            # A normal update-branch merge must keep the reviewed tree as one
            # parent and use suite main as the other. Recheck the changed files
            # before accepting any other head in a new invocation.
            commit = api(f"repos/{SUITE}/commits/{current_head}")
            parents = [parent["sha"] for parent in commit["parents"]]
            if requested is None or len(parents) != 2 or parents[0] != head:
                raise ValueError(f"Suite #{number} changed outside the requested branch refresh")
            if parents[1] != requested:
                # Main can advance between our read and GitHub's update.
                if api(f"repos/{SUITE}/compare/{requested}...{parents[1]}")["behind_by"] != 0:
                    raise ValueError(f"Suite #{number} refreshed against an unexpected base")
            head, requested = current_head, None
        if current["merged"]:
            return f"Suite #{number} merged at {current['merge_commit_sha']}."
        if current["state"] != "open":
            raise ValueError(f"Suite #{number} closed without merging")
        base = api(f"repos/{SUITE}/git/ref/heads/main")["object"]["sha"]
        comparison = api(f"repos/{SUITE}/compare/{base}...{head}")
        stale = comparison["behind_by"] != 0
        if stale and requested is None:
            if not current.get("auto_merge"):
                auto_merge(api, current)
            api(f"{endpoint}/update-branch", "-X", "PUT", "-f", f"expected_head_sha={head}")
            requested = base
        elif not stale:
            failures, pending = required_check_state(api, SUITE, "main", head)
            if failures:
                raise ValueError(f"Suite #{number} failed required checks on {head}: " + ", ".join(failures))
            if not current.get("auto_merge"):
                if not pending and current["mergeable_state"] in {"clean", "unstable"}:
                    # GitHub declines auto-merge when the PR is immediately
                    # mergeable. Use the normal endpoint only with protection
                    # enforced for this credential, even if main moves now.
                    enforced = api(f"repos/{SUITE}/branches/main/protection/enforce_admins")["enabled"]
                    admin = (api(f"repos/{SUITE}").get("permissions") or {}).get("admin")
                    if not enforced and admin is not False:
                        raise ValueError(f"Suite #{number} is current and green, but requires a non-bypassing merge credential")
                    result = api(f"{endpoint}/merge", "-X", "PUT", "-f", "merge_method=merge",
                                 "-f", f"sha={head}")
                    if result["merged"]:
                        return f"Suite #{number} merged at {result['sha']}."
                    raise ValueError(f"Suite #{number} was not merged: {result['message']}")
                else:
                    auto_merge(api, current)
        time.sleep(30)
    raise ValueError(f"Suite #{number} did not reach a merge verdict on {head}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("repair", type=int)
    args = parser.parse_args()

    def api(endpoint, *flags):
        output = subprocess.check_output(["gh", "api", endpoint, *flags], text=True, timeout=120)
        return json.loads(output) if output else None

    try:
        print(finish_companion(api, args.repair))
    except (ValueError, subprocess.SubprocessError) as error:
        raise SystemExit(str(error)) from error


if __name__ == "__main__":
    main()
