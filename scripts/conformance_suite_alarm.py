"""Alarm when the conformance suite loses tests or its run time moves 10% in a week.

The suite's wall time on the CI runners is steady (47 runs on 2026-11-11: 662-724 s,
1.9% spread), because fixed waits in the tests dominate it. A 10% move in the weekly
average therefore means a hang, a new timeout, or ordinary commands several times
slower; it does not see smaller engine slowdowns or anything about many players.

Samples are merge queue runs of ci.yml, the runs whose tree lands on master: the
seconds its "Run conformance suite" step took, and for the two newest the number of
tests in conformance-results.xml. The newest week is compared with the week before.

An alarm only reports. With --open-issue it opens one issue labelled
conformance-suite-alarm (none while one is already open) and exits 0; nothing is
stopped or switched off. Without it, the reasons are printed and the exit code is 1.
"""

import argparse
import datetime
import io
import json
import statistics
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

JOB = "MOO conformance suite"
STEP = "Run conformance suite"
WEEK = datetime.timedelta(days=7)
PER_WEEK = 12   # runs sampled from each week
ENOUGH = 5      # a week with fewer samples gives no verdict
LIMIT = 0.10
LABEL = "conformance-suite-alarm"


def when(stamp):
    return datetime.datetime.fromisoformat(stamp.replace("Z", "+00:00"))


def spread(runs, count):
    """Up to count runs, evenly spaced through the list."""
    if len(runs) <= count:
        return list(runs)
    return [runs[round(i * (len(runs) - 1) / (count - 1))] for i in range(count)]


def step_seconds(jobs):
    """Seconds the suite step took in a run's jobs, or None when it did not run and pass."""
    for job in jobs:
        if job["name"] != JOB:
            continue
        for step in job.get("steps", []):
            if step["name"] == STEP and step["conclusion"] == "success":
                return (when(step["completed_at"]) - when(step["started_at"])).total_seconds()
    return None


def test_count(junit_xml):
    """Number of test cases a junit report holds."""
    return sum(1 for _ in ET.fromstring(junit_xml).iter("testcase"))


def verdict(recent, earlier, newest_tests, previous_tests):
    """Reasons to alarm, from two weeks of step seconds and the two newest test counts."""
    reasons = []
    if newest_tests is not None and previous_tests is not None and newest_tests < previous_tests:
        reasons.append(f"The suite ran {newest_tests} tests; the run before it ran {previous_tests}.")
    if len(recent) >= ENOUGH and len(earlier) >= ENOUGH:
        now, before = statistics.mean(recent), statistics.mean(earlier)
        change = (now - before) / before
        if abs(change) > LIMIT:
            reasons.append(f"The suite's average run time moved {change:+.1%} in a week: {before:.0f} s over "
                           f"{len(earlier)} runs, then {now:.0f} s over {len(recent)} runs.")
    return reasons


def issue(reasons, run_url):
    """The issue an alarm opens."""
    lines = ["The conformance suite's test count or run time changed:", ""]
    lines += [f"- {reason}" for reason in reasons]
    lines += ["", f"Measured by {run_url} from merge queue runs of ci.yml.",
              "Nothing has been stopped. Close this issue when the change is explained; "
              "no new one is opened while it is open."]
    return {"title": "Conformance suite test count or run time changed", "body": "\n".join(lines), "labels": [LABEL]}


def api(path, raw=False, post=None):
    command = ["gh", "api", path] + (["--method", "POST", "--input", "-"] if post is not None else [])
    out = subprocess.run(command, check=True, capture_output=True,
                         input=None if post is None else json.dumps(post).encode()).stdout
    return out if raw else json.loads(out)


def merged_runs(repo, since, until):
    """Successful merge queue runs of ci.yml created in [since, until), oldest first."""
    window = f"{since:%Y-%m-%dT%H:%M:%SZ}..{until:%Y-%m-%dT%H:%M:%SZ}"
    runs, page = [], 1
    while True:
        batch = api(f"repos/{repo}/actions/workflows/ci.yml/runs?event=merge_group&status=success"
                    f"&per_page=100&page={page}&created={window}")["workflow_runs"]
        runs += batch
        if len(batch) < 100:
            return sorted(runs, key=lambda run: run["created_at"])
        page += 1


def run_tests(repo, run_id):
    """Test count from a run's conformance-results artifact, or None when it has none."""
    for artifact in api(f"repos/{repo}/actions/runs/{run_id}/artifacts")["artifacts"]:
        if artifact["name"] == "conformance-results" and not artifact["expired"]:
            archive = zipfile.ZipFile(io.BytesIO(api(f"repos/{repo}/actions/artifacts/{artifact['id']}/zip", raw=True)))
            return test_count(archive.read("conformance-results.xml"))
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, help="owner/name")
    parser.add_argument("--open-issue", metavar="RUN_URL", help="report an alarm as an issue naming this run")
    args = parser.parse_args()
    now = datetime.datetime.now(datetime.timezone.utc)

    weeks = []
    for start, end in ((now - WEEK, now), (now - 2 * WEEK, now - WEEK)):
        timed = []
        for run in spread(merged_runs(args.repo, start, end), PER_WEEK):
            seconds = step_seconds(api(f"repos/{args.repo}/actions/runs/{run['id']}/jobs?per_page=100")["jobs"])
            if seconds is not None:  # the suite is skipped when the same tree already passed
                timed.append((run["id"], seconds))
        weeks.append(timed)
    recent, earlier = weeks
    print(f"newest week: {[round(s) for _, s in recent]}")
    print(f"week before: {[round(s) for _, s in earlier]}")

    counts = [run_tests(args.repo, run_id) for run_id, _ in (earlier + recent)[-2:]]
    previous_tests, newest_tests = ([None, None] + counts)[-2:]
    print(f"test counts, previous then newest: {previous_tests}, {newest_tests}")

    reasons = verdict([s for _, s in recent], [s for _, s in earlier], newest_tests, previous_tests)
    for reason in reasons:
        print(reason)
    if not reasons or not args.open_issue:
        return 1 if reasons else 0
    if api(f"repos/{args.repo}/issues?state=open&labels={LABEL}&per_page=1"):
        print("An alarm issue is already open; not opening another.")
    else:
        print("Opened", api(f"repos/{args.repo}/issues", post=issue(reasons, args.open_issue))["html_url"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
