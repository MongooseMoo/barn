"""Finish the suite PR for a closed Barn repair under GitHub's branch rules."""

import argparse
import json
import subprocess
import time


BARN = "MongooseMoo/barn"
SUITE = "MongooseMoo/moo-conformance-tests"
REJECTED = {"failure", "timed_out", "cancelled", "action_required"}


def required_check_state(api, repository, branch, head):
    branch_info = api(f"repos/{repository}/branches/{branch}")
    required = branch_info.get("protection", {}).get("required_status_checks", {}).get("checks", [])
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


def wait_for_repair_checks(api, number, head, suite_head=None, timeout=10800):
    endpoint = f"repos/{BARN}/pulls/{number}"
    deadline = time.monotonic() + timeout
    while True:
        pr = api(endpoint)
        if pr["head"]["sha"] != head:
            raise ValueError(f"Repair #{number}: the Barn head moved after validation")
        if pr["merged"]:
            return "merged"
        if pr["state"] != "open":
            raise ValueError(f"Repair #{number} closed without merging")
        failures, pending = required_check_state(api, BARN, "master", head)
        if failures:
            raise ValueError(f"Repair #{number}: required checks failed on {head}: " + ", ".join(failures))
        if not pending:
            if suite_head:
                companion = companion_pull(api, number)
                if companion is None or companion["head"]["sha"] != suite_head:
                    raise ValueError(f"Repair #{number}: the companion head moved after validation")
            response = api("graphql", "-f", f"id={pr['node_id']}", "-f",
                           "query=query($id:ID!){node(id:$id){... on PullRequest{headRefOid state isInMergeQueue}}}")
            current = response["data"]["node"]
            if current["headRefOid"] != head:
                raise ValueError(f"Repair #{number}: the Barn head moved after validation")
            if current["state"] == "MERGED":
                return "merged"
            if current["state"] != "OPEN":
                raise ValueError(f"Repair #{number} closed without merging")
            if current["isInMergeQueue"]:
                return "queued"
            return "ready"
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ValueError(f"Repair #{number}: required checks on {head} are still pending")
        time.sleep(min(30, remaining))


def check_companion_diff(api, endpoint, pr):
    allow = "src/moo_conformance/_db/startup/startup-fixtures.sha256"
    files = api(f"{endpoint}/files?per_page=100")
    if len(files) != pr["changed_files"] or any(
        not (change["filename"] == allow and change["status"] == "modified") and (
            change["status"] != "added" or
            not change["filename"].startswith(("src/moo_conformance/_tests/", "src/moo_conformance/_db/"))
        ) for change in files
    ):
        raise ValueError(f"Suite #{pr['number']} has changes outside the allowed test additions")
    if api(endpoint)["head"]["sha"] != pr["head"]["sha"]:
        raise ValueError(f"Suite #{pr['number']} changed during the file check")


def merge_checked_companion(api, endpoint, pr, deadline):
    number = pr["number"]
    head = pr["head"]["sha"]
    response = api("graphql", "-f", f"id={pr['node_id']}", "-f",
                   "query=query($id:ID!){node(id:$id){... on PullRequest{headRefOid viewerCanMergeAsAdmin}}}")
    access = response["data"]["node"]
    if access["headRefOid"] != head:
        raise ValueError(f"Suite #{number} changed before merge")
    if access["viewerCanMergeAsAdmin"] is not False:
        raise ValueError(f"Suite #{number} is current and green, but requires a non-bypassing merge credential")

    version = "X-GitHub-Api-Version: 2026-03-10"
    result = api(f"{endpoint}/merge-async", "-X", "PUT", "-H", version,
                 "-f", "merge_method=merge", "-f", f"sha={head}", "-F", "bypass_rules=false")
    while result["status"] == "pending":
        request = result["details"]
        if request["expected_head_sha"] != head or request.get("bypass_rules") is not False:
            raise ValueError(f"Suite #{number} merge receipt does not bind the validated head and protections")
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ValueError(f"Suite #{number} merge request {request['uuid']} is still pending")
        time.sleep(min(30, remaining))
        result = api(f"{endpoint}/merge-async/{request['uuid']}", "-H", version)
    if result["status"] != "merged":
        raise ValueError(f"Suite #{number} merge result {result['status']}: {result['details']['message']}")
    landed = api(endpoint)
    if not landed["merged"] or landed["head"]["sha"] != head or landed["merge_commit_sha"] != result["details"]["sha"]:
        raise ValueError(f"Suite #{number} merge result does not match the validated head")
    return f"Suite #{number} merged at {landed['merge_commit_sha']}."


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
    if companion["head"]["repo"]["full_name"] != SUITE:
        raise ValueError(f"Suite #{number} is not a branch of {SUITE}")
    if not barn["merged"]:
        if companion["state"] != "open":
            raise ValueError(f"Suite #{number} is already closed")
        api(endpoint, "-X", "PATCH", "-f", "state=closed")
        return f"Barn #{repair} closed unmerged. Closed companion #{number}."

    head = companion["head"]["sha"]
    check_companion_diff(api, endpoint, companion)
    requested = None
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        current = api(endpoint)
        current_head = current["head"]["sha"]
        if current_head != head:
            commit = api(f"repos/{SUITE}/commits/{current_head}")
            parents = [parent["sha"] for parent in commit["parents"]]
            if requested is None or len(parents) != 2 or parents[0] != head:
                raise ValueError(f"Suite #{number} changed outside the requested branch refresh")
            if parents[1] != requested:
                # Main can advance between our read and GitHub's update.
                if api(f"repos/{SUITE}/compare/{requested}...{parents[1]}")["behind_by"] != 0:
                    raise ValueError(f"Suite #{number} refreshed against an unexpected base")
            check_companion_diff(api, endpoint, current)
            head, requested = current_head, None
        if current["state"] != "open" and not current["merged"]:
            raise ValueError(f"Suite #{number} closed without merging")
        if current["merged"]:
            failures, pending = required_check_state(api, SUITE, "main", head)
            if failures or pending:
                raise ValueError(f"Suite #{number} merged without the expected check verdict on {head}")
            return f"Suite #{number} merged at {current['merge_commit_sha']}."
        base = api(f"repos/{SUITE}/git/ref/heads/main")["object"]["sha"]
        stale = api(f"repos/{SUITE}/compare/{base}...{head}")["behind_by"] != 0
        if stale and requested is None:
            if current.get("auto_merge"):
                raise ValueError(f"Suite #{number} has a pre-existing auto-merge request. Its owner must handle refresh.")
            api(f"{endpoint}/update-branch", "-X", "PUT", "-f", f"expected_head_sha={head}")
            requested = base
        elif not stale:
            failures, pending = required_check_state(api, SUITE, "main", head)
            if failures:
                raise ValueError(f"Suite #{number} failed required checks on {head}: " + ", ".join(failures))
            if not pending and not current.get("auto_merge") and current["mergeable_state"] in {"clean", "unstable"}:
                return merge_checked_companion(api, endpoint, current, deadline)
        time.sleep(min(30, max(0, deadline - time.monotonic())))
    raise ValueError(f"Suite #{number} did not reach a merge verdict on {head}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("repair", type=int)
    parser.add_argument("--barn-head")
    parser.add_argument("--suite-head")
    args = parser.parse_args()

    def api(endpoint, *flags):
        output = subprocess.check_output(["gh", "api", endpoint, *flags], text=True, timeout=120)
        return json.loads(output) if output else None

    try:
        if args.barn_head:
            print(wait_for_repair_checks(api, args.repair, args.barn_head, args.suite_head))
        else:
            print(finish_companion(api, args.repair))
    except (ValueError, subprocess.SubprocessError) as error:
        raise SystemExit(str(error)) from error


if __name__ == "__main__":
    main()
