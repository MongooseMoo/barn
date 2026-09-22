#!/usr/bin/env python3
"""Render a matrix JSON without counting incorrect samples as speedups."""
import argparse
from collections import defaultdict
import json
from pathlib import Path
import statistics


def report(data):
    lanes = list(dict.fromkeys(r["lane"] for r in data["runs"]))
    errors = [(r["lane"], r["round"], e) for r in data["runs"] for e in r["errors"]]
    rows = ["# Mongoose performance matrix", "", f"Platform: `{data['platform']}`.",
            f"Fresh worlds: {len(data['runs'])}. Recorded errors: {len(errors)}.", "",
            "Each cell is the median of the per-world medians (milliseconds). "
            "Parentheses give the smallest and largest per-world median. "
            "Warm-ups and worlds with recorded errors are excluded from timing summaries. "
            "Missing/failed cells are not speedups.", "",
            "Accepted worlds by lane: " + ", ".join(f"{lane}={sum(r['lane'] == lane and not r['errors'] for r in data['runs'])}" for lane in lanes) + ".", "",
            "| Workload / clock | " + " | ".join(lanes) + " |",
            "|---|" + "---:|" * len(lanes)]
    groups = defaultdict(lambda: defaultdict(list))
    invalid = set()
    for run in data["runs"]:
        local = defaultdict(list)
        for sample in run["samples"]:
            if "error" in sample:
                invalid.add((sample["case"], run["lane"]))
                continue
            if sample.get("warmup"):
                continue
            for clock in ("server_s", "reported_s", "wall_s"):
                if clock in sample:
                    key = (sample["case"], clock)
                    groups[key]  # Preserve INVALID rows even without a valid world.
                    if not run["errors"]:
                        local[key].append(sample[clock] * 1000)
        for key, vals in local.items():
            groups[key][run["lane"]].append(statistics.median(vals))
    for (case, clock), by_lane in groups.items():
        cells = []
        for lane in lanes:
            values = by_lane[lane]
            cells.append("INVALID" if (case, lane) in invalid else "—" if not values else
                         f"{statistics.median(values):.3f} ({min(values):.3f}–{max(values):.3f})")
        rows.append(f"| {case} / {clock.removesuffix('_s')} | " + " | ".join(cells) + " |")
    rows += ["", "Concurrent transactions: operations/s, median across worlds. "
             "Every accepted run validates final committed counters. "
             "One operation includes the statement-mode eval and its reply.", "",
             "| Workload | Clients | " + " | ".join(lanes) + " |",
             "|---|---:|" + "---:|" * len(lanes)]
    parallel = defaultdict(lambda: defaultdict(list))
    failed_parallel = set()
    for run in data["runs"]:
        for error in run["errors"]:
            if "shape" in error:
                failed_parallel.add((error["shape"], error["clients"], run["lane"]))
        for sample in ([] if run["errors"] else run["parallel"]):
            parallel[(sample["shape"], sample["clients"])][run["lane"]].append(sample["ops_s"])
    for (shape, clients), by_lane in sorted(parallel.items()):
        cells = []
        for lane in lanes:
            values = by_lane[lane]
            cells.append("INVALID" if (shape, clients, lane) in failed_parallel else "—" if not values else
                         f"{statistics.median(values):.1f} ({min(values):.1f}–{max(values):.1f})")
        rows.append(f"| {shape} | {clients} | " + " | ".join(cells) + " |")
    rows += ["", "Concurrent p95 request latency (milliseconds), median across worlds.", "",
             "| Workload | Clients | " + " | ".join(lanes) + " |",
             "|---|---:|" + "---:|" * len(lanes)]
    tails = defaultdict(lambda: defaultdict(list))
    for run in data["runs"]:
        for sample in ([] if run["errors"] else run["parallel"]):
            tails[(sample["shape"], sample["clients"])][run["lane"]].append(sample["p95_s"] * 1000)
    for (shape, clients), by_lane in sorted(tails.items()):
        cells = []
        for lane in lanes:
            values = by_lane[lane]
            cells.append("INVALID" if (shape, clients, lane) in failed_parallel else "—" if not values else
                         f"{statistics.median(values):.3f} ({min(values):.3f}–{max(values):.3f})")
        rows.append(f"| {shape} | {clients} | " + " | ".join(cells) + " |")
    rows += ["", "## Log audit", "", "```json", json.dumps([
        {"lane": r["lane"], "round": r["round"], **r.get("log_audit", {})} for r in data["runs"]
    ], indent=2), "```", "",
             "## Provenance", "", "```json", json.dumps(data["artifacts"], indent=2), "```", "",
             "## Errors", "", "```json", json.dumps(errors, indent=2), "```", ""]
    return "\n".join(rows)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("matrix", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    rendered = report(json.loads(args.matrix.read_text()))
    if args.output:
        args.output.write_text(rendered, encoding="utf-8")
    else:
        print(rendered)
