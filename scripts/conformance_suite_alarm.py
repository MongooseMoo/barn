"""Alarm when the conformance suite loses tests or its run time moves 10% in a week.

For a runtime alarm, compare the sampled JUnit reports to show how much of the
weekly wall-time change came from new cases, previously seen cases and time
outside case timers. These jobs do not measure workstation load.

Samples are merge queue runs of ci.yml, the runs whose tree lands on master: the
seconds its "Run conformance suite" step took, and for the two newest the number of
tests in conformance-results.xml. The newest week is compared with the week before.
Runs from before the newest closed alarm issue was closed are left out: closing the
issue says its change is explained, so a lasting step (such as the harness change of
2026-11-12 that halved the suite's time) is reported once, not again for two weeks.
For a week after a close there is therefore no run-time verdict; the test count is
still checked.

An alarm only reports. With --open-issue it opens one issue labelled
conformance-suite-alarm (none while one is already open) and exits 0; nothing is
stopped or switched off. Without it, the reasons are printed and the exit code is 1.
"""

import argparse
from collections import Counter
from dataclasses import dataclass
import datetime
import io
import json
import math
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


@dataclass(frozen=True)
class Sample:
    run_id: int
    job_url: str
    runner: str
    seconds: float


def timed_sample(jobs):
    """The job whose suite step ran and passed."""
    for job in jobs:
        if job["name"] != JOB:
            continue
        for step in job.get("steps", []):
            if step["name"] == STEP and step["conclusion"] == "success":
                seconds = (when(step["completed_at"]) - when(step["started_at"])).total_seconds()
                return Sample(job["run_id"], job["html_url"], job["runner_name"], seconds)
    return None


@dataclass
class Report:
    count: int | None
    times: Counter | None
    problem: str = ""


def parse_report(junit_xml):
    root = ET.fromstring(junit_xml)
    if root.tag not in ("testsuite", "testsuites"):
        return Report(None, None, "unreadable JUnit report")
    cases = list(root.iter("testcase"))
    times = Counter()
    for case in cases:
        try:
            seconds = float(case.attrib["time"])
            if not math.isfinite(seconds) or seconds < 0:
                raise ValueError
            times[case.get("classname", ""), case.attrib["name"]] += seconds
        except (KeyError, ValueError):
            return Report(len(cases), None, "missing or invalid case name or duration")
    return Report(len(cases), times)


def verdict(recent, earlier, newest_tests, previous_tests):
    """Reasons to alarm, from two weeks of step seconds and the two newest test counts."""
    reasons = []
    if newest_tests is not None and previous_tests is not None and newest_tests < previous_tests:
        reasons.append(f"The suite ran {newest_tests} tests; the run before it ran {previous_tests}.")
    return reasons + runtime_verdict(recent, earlier)


def runtime_verdict(recent, earlier):
    if len(recent) >= ENOUGH and len(earlier) >= ENOUGH:
        now, before = statistics.mean(recent), statistics.mean(earlier)
        change = (now - before) / before
        if abs(change) > LIMIT:
            return [f"The suite's average run time moved {change:+.1%} in a week: {before:.0f} s over "
                    f"{len(earlier)} runs, then {now:.0f} s over {len(recent)} runs."]
    return []


def issue(reasons, run_url, report=""):
    """The issue an alarm opens."""
    lines = ["The conformance suite's test count or run time changed:", ""]
    lines += [f"- {reason}" for reason in reasons]
    if report:
        lines += ["", report]
    lines += ["", f"Measured by {run_url} from merge queue runs of ci.yml.",
              "Nothing has been stopped. Close this issue when the change is explained; "
              "no new one is opened while it is open."]
    return {"title": "Conformance suite test count or run time changed", "body": "\n".join(lines), "labels": [LABEL]}


def api(path, raw=False, post=None):
    command = ["gh", "api", path] + (["--method", "POST", "--input", "-"] if post is not None else [])
    out = subprocess.run(command, check=True, capture_output=True,
                         input=None if post is None else json.dumps(post).encode()).stdout
    return out if raw else json.loads(out)


def investigate(repo, number):
    """Start alarm-investigate.yml on a newly opened alarm issue.

    An issue opened with GITHUB_TOKEN starts no `issues` workflows; workflow_dispatch
    is exempt, as for the fuzz repairs (scripts/fuzz_report.py).
    """
    api(f"repos/{repo}/actions/workflows/alarm-investigate.yml/dispatches", raw=True,
        post={"ref": "master", "inputs": {"issue_number": str(number)}})


def last_closed(repo, label):
    """When the newest closed issue with this label was closed, or None.

    Closing an alarm issue says its change is explained, so the comparison starts
    again from that moment instead of re-reporting the same change.
    """
    closed = [issue["closed_at"] for issue in
              api(f"repos/{repo}/issues?state=closed&labels={label}&per_page=100")
              if issue.get("closed_at")]
    return max(map(when, closed)) if closed else None


def weeks(now, closed):
    """(start, end) of the newest week and the week before, neither starting before closed."""
    spans = ((now - WEEK, now), (now - 2 * WEEK, now - WEEK))
    return [(max(start, closed) if closed else start, end) for start, end in spans]


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


def run_report(repo, run_id):
    try:
        artifacts = api(f"repos/{repo}/actions/runs/{run_id}/artifacts?name=conformance-results")["artifacts"]
        for artifact in artifacts:
            if artifact["name"] == "conformance-results" and not artifact["expired"]:
                data = api(f"repos/{repo}/actions/artifacts/{artifact['id']}/zip", raw=True)
                with zipfile.ZipFile(io.BytesIO(data)) as archive:
                    return parse_report(archive.read("conformance-results.xml"))
        return Report(None, None, "artifact expired" if artifacts else "artifact missing")
    except subprocess.CalledProcessError as exc:
        print(f"Run {run_id} artifact request failed: {exc.stderr.decode(errors='replace')}", file=sys.stderr)
        return Report(None, None, "artifact request failed")
    except (zipfile.BadZipFile, KeyError, ET.ParseError) as exc:
        print(f"Run {run_id} has unreadable JUnit data: {exc}", file=sys.stderr)
        return Report(None, None, "unreadable JUnit report")


def runtime_report(earlier, recent, reports):
    rows = ["| Week | Sampled job | Runner | Wall time | JUnit |",
            "| --- | --- | --- | ---: | --- |"]
    for week, samples in (("Earlier", earlier), ("Newest", recent)):
        for s in samples:
            status = reports[s.run_id].problem or "available"
            rows.append(f"| {week} | [{s.run_id}]({s.job_url}) | `{s.runner}` | {s.seconds:.0f} s | {status} |")

    samples = earlier + recent
    available = sum(reports[s.run_id].times is not None for s in samples)
    lines = [f"JUnit timings available for {available}/{len(samples)} sampled jobs."]
    if available == len(samples):
        means = []
        for week in (earlier, recent):
            total = Counter()
            for s in week:
                total.update(reports[s.run_id].times)
            means.append({key: value / len(week) for key, value in total.items()})
        before, after = means
        added = sum(value for key, value in after.items() if key not in before)
        seen = sum(after.get(key, 0) - value for key, value in before.items())
        wall = statistics.mean(s.seconds for s in recent) - statistics.mean(s.seconds for s in earlier)
        share = f" ({added / wall:.0%} of the increase)" if wall > 0 else ""
        lines += ["", f"Average wall-time change per run: **{wall:+.2f} s**.",
                  f"- Cases absent from the earlier week: {added:+.2f} s{share}.",
                  f"- Previously seen cases, including removals: {seen:+.2f} s.",
                  f"- Time outside JUnit case timers: {wall - added - seen:+.2f} s."]
        files = Counter()
        for key in before.keys() | after.keys():
            classname, name = key
            # The conformance suite embeds the YAML path in its pytest parameter ID.
            path, sep, _ = name.partition("[")[2].partition(".yaml::")
            group = path + ".yaml" if sep else classname or name
            files[group] += after.get(key, 0) - before.get(key, 0)
        lines += ["", "Largest changes in mean case time per run:"]
        for group, delta in sorted(files.items(), key=lambda item: (-abs(item[1]), item[0]))[:5]:
            lines.append(f"- `{group}`: {delta:+.2f} s")
        lines += ["", "Workstation load was not measured."]
    else:
        lines.append("The full weekly change cannot be attributed with incomplete JUnit timings.")
    return "\n".join(lines + [""] + rows)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, help="owner/name")
    parser.add_argument("--open-issue", metavar="RUN_URL", help="report an alarm as an issue naming this run")
    args = parser.parse_args()
    now = datetime.datetime.now(datetime.timezone.utc)

    closed = last_closed(args.repo, LABEL)
    if closed:
        print(f"comparing only runs after the last alarm issue was closed at {closed:%Y-%m-%dT%H:%M:%SZ}")
    sampled = []
    for start, end in weeks(now, closed):
        timed = []
        runs = merged_runs(args.repo, start, end) if start < end else []
        for run in spread(runs, PER_WEEK):
            sample = timed_sample(api(f"repos/{args.repo}/actions/runs/{run['id']}/jobs?per_page=100")["jobs"])
            if sample:  # the suite is skipped when the same tree already passed
                timed.append(sample)
        sampled.append(timed)
    recent, earlier = sampled
    recent_seconds = [s.seconds for s in recent]
    earlier_seconds = [s.seconds for s in earlier]
    print(f"newest week: {[round(s) for s in recent_seconds]}")
    print(f"week before: {[round(s) for s in earlier_seconds]}")

    latest = (earlier + recent)[-2:]
    reports = {s.run_id: run_report(args.repo, s.run_id) for s in latest}
    counts = [reports[s.run_id].count for s in latest]
    previous_tests, newest_tests = ([None, None] + counts)[-2:]
    print(f"test counts, previous then newest: {previous_tests}, {newest_tests}")

    reasons = verdict(recent_seconds, earlier_seconds, newest_tests, previous_tests)
    for reason in reasons:
        print(reason)
    if not reasons:
        return 0
    if args.open_issue and api(f"repos/{args.repo}/issues?state=open&labels={LABEL}&per_page=1"):
        print("An alarm issue is already open; not opening another.")
        return 0
    report = ""
    if runtime_verdict(recent_seconds, earlier_seconds):
        for sample in earlier + recent:
            if sample.run_id not in reports:
                reports[sample.run_id] = run_report(args.repo, sample.run_id)
        report = runtime_report(earlier, recent, reports)
        print(report)
    if not args.open_issue:
        return 1
    opened = api(f"repos/{args.repo}/issues", post=issue(reasons, args.open_issue, report))
    print("Opened", opened["html_url"])
    investigate(args.repo, opened["number"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
