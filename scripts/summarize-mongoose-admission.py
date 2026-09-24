"""Summarize completed live trials without pooling runs into false replication."""
import argparse
from collections import defaultdict
import json
from pathlib import Path
import statistics


def summarize(rows):
    groups = defaultdict(list)
    for row in rows:
        capacity = row.get("admission_limit", row["limit"] if row["label"] == "new" else None)
        groups[(row["limit"], row["label"], capacity)].append(row)
    print("| CPUs | Admission | Build | Runs | Median ms, per run | p95 ms, per run | Max ms | Error-marked probes | Clean logins | Checkpoints OK |")
    print("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
    compact = []
    for (limit, label, capacity), trials in sorted(groups.items()):
        measured = [r for r in trials if "latency_ms" in r]
        latency = lambda key: ", ".join(f"{r['latency_ms'][key]:.2f}" for r in measured) or "unmeasured"
        errors = sum(c["error"] for r in trials for c in r["commands"])
        probes = sum(len(r["commands"]) for r in trials)
        clean = sum(r.get("clean_login", False) for r in trials)
        checkpoints = sum(r.get("checkpoint_success", False) for r in trials)
        print(f"| {limit} | {capacity if capacity is not None else '-'} | {label} | {len(trials)} | {latency('median')} | {latency('p95')} | {latency('max')} | {errors}/{probes} | {clean}/{len(trials)} | {checkpoints}/{len(trials)} |")
        compact.append(dict(limit=limit, admission_limit=capacity, label=label, runs=len(trials), measured_runs=len(measured),
            binary_sha256=sorted({r["binary_sha256"] for r in trials}),
            database_sha256=sorted({r["database_sha256"] for r in trials}),
            sqlite_sha256=trials[0].get("sqlite_sha256", {}),
            median_of_run_medians_ms=statistics.median(r["latency_ms"]["median"] for r in measured) if measured else None,
            median_of_run_p95_ms=statistics.median(r["latency_ms"]["p95"] for r in measured) if measured else None,
            probes=probes, error_marked_probes=errors, clean_logins=clean, successful_checkpoints=checkpoints,
            cpu_cores_per_run=[r["cpu_seconds"]/r["window_seconds"] for r in measured],
            observed_s_run_slow_slices=[r["loaded_background"]["s_run_slow_slices"] for r in measured],
            failures=[r["failure"] for r in trials if "failure" in r]))
    return compact


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", type=Path, nargs="+")
    parser.add_argument("--json-out", type=Path)
    args = parser.parse_args()
    rows = [row for path in args.results for row in json.loads(path.read_text(encoding="utf-8"))]
    result = summarize(rows)
    if args.json_out:
        args.json_out.write_bytes((json.dumps(result, indent=2) + "\n").encode("utf-8"))
