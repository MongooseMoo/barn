"""Keep a series of player goodput and latency on master and report a lasting change for the worse.

Each point is the median of several runs of the real-workload harness
(engine/mongoose_real_bench_test.go, TestMongooseRealWorkload) at 1 and 16 players
on one commit. One run cannot judge a commit: two 16-player runs of the same commit
have differed by 26% (892 and 707 commands/s on 2026-11-12). So nothing is decided
before a merge. After merges, the newest points are compared with the points before
them, as MongoDB does for its performance tests (Daly et al. 2020), and a change is
reported only once at least three points sit on the far side of it.

Three series are watched: commands/s at 16 players (lower is worse), p99 latency at
16 players and p50 latency for one player (higher is worse). A change is reported
when the newest 3 to 10 points differ from the 8 to 20 before them by at least 10%
in the worse direction and a one-sided rank test puts that at less than 1 in 100
by chance. These limits were chosen before any series existed; no real series has
tested them yet.

An alarm only reports. `check --open-issue` opens one issue labelled goodput-alarm
(none while one is already open); nothing is stopped or switched off.
"""

import argparse
import io
import itertools
import json
import math
import random
import re
import statistics
import sys
import zipfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_suite_alarm import api  # noqa: E402

RESULT = re.compile(r"players=(\d+) goodput=(\d+)/s failed=(\d+) uncaught=(\d+) .*? p50=(\S+) p99=(\S+) ")
WORKFLOW = "goodput-series.yml"
ARTIFACT = "goodput-point"
LABEL = "goodput-alarm"
KEEP = 40            # newest points read
AFTER = range(3, 11)  # points that may sit after a change
BEFORE = (8, 20)     # fewest and most points compared before it
SHIFT = 0.10
CHANCE = 0.01
EXACT = 20000        # enumerate every split up to this many, sample this many beyond
# (title, players, field, +1 when higher is worse / -1 when lower is worse, unit)
WATCHED = (
    ("Commands per second at 16 players", "16", "goodput", -1, "/s"),
    ("p99 latency at 16 players", "16", "p99_ms", +1, " ms"),
    ("p50 latency for one player", "1", "p50_ms", +1, " ms"),
)


def milliseconds(text):
    """The harness prints a latency as 12.34us or 12.34ms."""
    number, unit = float(text[:-2]), text[-2:]
    if unit not in ("us", "ms"):
        raise ValueError(f"unknown latency unit in {text!r}")
    return number / 1000 if unit == "us" else number


def run_results(output):
    """{players: {goodput, p50_ms, p99_ms, failed, uncaught}} from one harness run's output."""
    return {players: {"goodput": float(goodput), "p50_ms": milliseconds(p50), "p99_ms": milliseconds(p99),
                      "failed": int(failed), "uncaught": int(uncaught)}
            for players, goodput, failed, uncaught, p50, p99 in RESULT.findall(output)}


def point(outputs, levels):
    """Median of each number over the runs; raises when a run lacks a level or lost commands."""
    runs = [run_results(output) for output in outputs]
    medians = {}
    for level in levels:
        for number, results in enumerate(runs, 1):
            if level not in results:
                raise ValueError(f"run {number} has no result for {level} players")
            if results[level]["failed"] or results[level]["uncaught"]:
                raise ValueError(f"run {number} at {level} players: {results[level]['failed']} failed, "
                                 f"{results[level]['uncaught']} uncaught")
        medians[level] = {field: statistics.median(results[level][field] for results in runs)
                          for field in ("goodput", "p50_ms", "p99_ms")}
        medians[level]["goodput_runs"] = [results[level]["goodput"] for results in runs]
    return medians


def chance(before, after):
    """One-sided chance that points drawn alike rank `after` at least this far above `before`."""
    pooled = before + after
    order = sorted(pooled)
    rank = {value: (order.index(value) + len(order) - order[::-1].index(value) + 1) / 2 for value in set(pooled)}
    seen = sum(rank[value] for value in after)
    count = len(after)
    if math.comb(len(pooled), count) <= EXACT:
        sums = [sum(rank[value] for value in chosen) for chosen in itertools.combinations(pooled, count)]
    else:
        draw = random.Random(0)
        sums = [sum(rank[value] for value in draw.sample(pooled, count)) for _ in range(EXACT)]
    return sum(1 for total in sums if total >= seen) / len(sums)


def change(values, worse):
    """The newest points that are lastingly worse than the points before them, or None.

    Of the splits that qualify, the one least likely by chance is where the change is.
    Returns (points after the change, median before, median after, shift as a fraction).
    """
    found, least = None, CHANCE
    for after_count in AFTER:
        before = values[max(0, len(values) - after_count - BEFORE[1]):len(values) - after_count]
        after = values[len(values) - after_count:]
        if len(before) < BEFORE[0]:
            break
        was, now = statistics.median(before), statistics.median(after)
        shift = (now - was) / was
        if shift * worse < SHIFT:
            continue
        signed = (lambda xs: xs) if worse > 0 else (lambda xs: [-x for x in xs])
        odds = chance(signed(before), signed(after))
        if odds < least:
            found, least = (after_count, was, now, shift), odds
    return found


def findings(points):
    """One sentence per watched series that changed for the worse; points are oldest first."""
    reasons = []
    for title, level, field, worse, unit in WATCHED:
        series = [(p["commit"], p["levels"][level][field]) for p in points if level in p["levels"]]
        hit = change([value for _, value in series], worse)
        if hit:
            after_count, was, now, shift = hit
            last_good, first_bad = series[-after_count - 1][0], series[-after_count][0]
            reasons.append(f"{title} moved {shift:+.0%}: median {was:.4g}{unit} before, {now:.4g}{unit} over the "
                           f"newest {after_count} points. The change is in {last_good[:12]}...{first_bad[:12]}.")
    return reasons


def issue(reasons, repo, run_url):
    lines = ["Player goodput or latency on master changed for the worse and stayed there:", ""]
    lines += [f"- {reason}" for reason in reasons]
    lines += ["", f"Measured by {run_url}; each point is the median of several harness runs "
              f"(https://github.com/{repo}/actions/workflows/{WORKFLOW}).",
              "Nothing has been stopped. Close this issue when the change is fixed or explained; "
              "no new one is opened while it is open."]
    return {"title": "Player goodput or latency got worse on master", "body": "\n".join(lines), "labels": [LABEL]}


def past_points(repo):
    """Points from the newest successful runs on master, oldest first."""
    runs = api(f"repos/{repo}/actions/workflows/{WORKFLOW}/runs?branch=master&status=success&per_page={KEEP}")
    points = []
    for run in runs["workflow_runs"]:
        for artifact in api(f"repos/{repo}/actions/runs/{run['id']}/artifacts")["artifacts"]:
            if artifact["name"] == ARTIFACT and not artifact["expired"]:
                archive = zipfile.ZipFile(io.BytesIO(api(f"repos/{repo}/actions/artifacts/{artifact['id']}/zip", raw=True)))
                points.append(json.loads(archive.read("point.json")))
    return sorted(points, key=lambda p: p["measured_at"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    make = commands.add_parser("point", help="print one point from harness run outputs")
    make.add_argument("--commit", required=True)
    make.add_argument("--measured-at", required=True, help="UTC time, ISO 8601")
    make.add_argument("--fixture", required=True, help="sha256 of the database the runs loaded")
    make.add_argument("--load", required=True, help="1-minute load average before the runs")
    make.add_argument("--levels", default="1,16")
    make.add_argument("outputs", nargs="+", type=Path)
    check = commands.add_parser("check", help="compare the newest points with the ones before them")
    check.add_argument("--repo", required=True, help="owner/name")
    check.add_argument("--open-issue", metavar="RUN_URL", help="report an alarm as an issue naming this run")
    check.add_argument("--newest", type=Path, help="this run's point, which no finished run holds yet")
    args = parser.parse_args()

    if args.command == "point":
        levels = point([path.read_text(encoding="utf-8", errors="replace") for path in args.outputs],
                       args.levels.split(","))
        print(json.dumps({"commit": args.commit, "measured_at": args.measured_at, "fixture": args.fixture,
                          "load": float(args.load), "runs": len(args.outputs), "levels": levels}, indent=1))
        return 0

    points = past_points(args.repo)
    if args.newest and args.newest.exists():  # absent when the machine was too busy to measure
        points.append(json.loads(args.newest.read_text(encoding="utf-8")))
    # A different database gives different numbers; compare only points on the newest one.
    points = [p for p in points if points and p["fixture"] == points[-1]["fixture"]]
    print(f"{len(points)} points on the newest fixture")
    for title, level, field, _, unit in WATCHED:
        print(f"{title} ({unit.strip()}): {[round(p['levels'][level][field], 2) for p in points if level in p['levels']]}")
    reasons = findings(points)
    for reason in reasons:
        print(reason)
    if not reasons or not args.open_issue:
        return 1 if reasons else 0
    if api(f"repos/{args.repo}/issues?state=open&labels={LABEL}&per_page=1"):
        print("An alarm issue is already open; not opening another.")
    else:
        print("Opened", api(f"repos/{args.repo}/issues", post=issue(reasons, args.repo, args.open_issue))["html_url"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
