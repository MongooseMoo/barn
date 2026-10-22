#!/usr/bin/env python3
"""Sealed component experiment runner; runs prebuilt Go test binaries only."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import random
import re
import statistics
import subprocess

ROWS = [(mode, v, lines) for mode in ("Live", "Adopt")
        for v, lines in ((1, 0), (16, 64), (128, 64), (128, 512))]
ENV = {"GOMAXPROCS": "1", "GOGC": "100"}
PATTERN = re.compile(r"^(BenchmarkVerbArgsPublication/\S+)\s+(\d+)\s+([\d.]+)\s+ns/op\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op$", re.M)


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def write(path, value):
    Path(path).write_text(json.dumps(value, indent=2) + "\n")


def row_name(row):
    mode, verbs, lines = row
    return f"BenchmarkVerbArgsPublication/{mode}/V{verbs}/L{lines}"


def run(binary, row, count, output):
    name = row_name(row)
    selector = "/".join("^" + part + "$" for part in name.split("/"))
    command = [str(Path(binary).resolve()), "-test.run=^$", "-test.bench=" + selector,
               f"-test.benchtime={count}x", "-test.count=1", "-test.benchmem",
               "-test.timeout=5m"]
    env = dict(os.environ)
    env.pop("GOMEMLIMIT", None)
    env.update(ENV)
    try:
        result = subprocess.run(command, env=env, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=330)
    except subprocess.TimeoutExpired as error:
        partial = error.stdout or ""
        if isinstance(partial, bytes):
            partial = partial.decode("utf-8", errors="replace")
        Path(output).write_text(partial)
        raise RuntimeError(f"Benchmark timed out; partial output retained: {output}") from error
    Path(output).write_text(result.stdout)
    matches = PATTERN.findall(result.stdout)
    if result.returncode or len(matches) != 1:
        raise RuntimeError(f"Invalid benchmark: {output}; exit={result.returncode}; rows={len(matches)}")
    found, actual, ns, space, allocs = matches[0]
    if re.sub(r"-\d+$", "", found) != name or int(actual) != count:
        raise RuntimeError(f"Benchmark identity/count mismatch in {output}")
    return {"row": name, "count": count, "ns": float(ns), "bytes": float(space),
            "allocs": float(allocs), "raw": str(output), "raw_sha256": digest(output),
            "command": command, "exit": result.returncode}


def calibrate(args):
    out = Path(args.output)
    out.mkdir(parents=True, exist_ok=False)
    rows = []
    for index, row in enumerate(ROWS):
        value = run(args.baseline, row, 1000, out / f"calibration-{index:02}.txt")
        elapsed = value["ns"] * 1000 / 1e9
        count = min(50000, max(1000, math.ceil(0.75 / elapsed) * 1000))
        rows.append({"dimensions": row, "iterations": count, "calibration": value})
        print(row_name(row), "iterations", count, flush=True)
    write(out / "plan.json", {"baseline_sha256": digest(args.baseline),
                              "runner_sha256": digest(__file__), "environment": ENV,
                              "gomemlimit": "unset", "platform": platform.platform(),
                              "pairs": 10, "rows": rows, "metric_mode": args.metric_mode})


def compare(args):
    plan = json.loads(Path(args.plan).read_text())
    if digest(__file__) != plan["runner_sha256"]:
        raise RuntimeError("Runner changed after calibration")
    if digest(args.baseline) != plan["baseline_sha256"] or plan["pairs"] != 10:
        raise RuntimeError("Baseline or pair count changed")
    out = Path(args.output)
    out.mkdir(parents=True, exist_ok=False)
    record = {"plan_sha256": digest(args.plan), "runner_sha256": digest(__file__),
              "baseline_sha256": digest(args.baseline),
              "candidate_sha256": digest(args.candidate), "samples": []}
    write(out / "results.json", record)
    for pair in range(1, 11):
        sides = ("baseline", "candidate") if pair % 2 else ("candidate", "baseline")
        rows = plan["rows"] if pair % 2 else list(reversed(plan["rows"]))
        for side in sides:
            binary = getattr(args, side)
            for index, row in enumerate(rows):
                value = run(binary, row["dimensions"], row["iterations"],
                            out / f"pair-{pair:02}-{side}-{index:02}.txt")
                value.update(pair=pair, side=side)
                record["samples"].append(value)
                write(out / "results.json", record)
        print("PAIR_COMPLETED", pair, flush=True)
    if digest(args.baseline) != record["baseline_sha256"] or digest(args.candidate) != record["candidate_sha256"]:
        raise RuntimeError("Binary changed during comparison")
    print("COMPARISON_COMPLETED", flush=True)


def percentile(values, fraction):
    values = sorted(values)
    at = (len(values) - 1) * fraction
    lower, upper = math.floor(at), math.ceil(at)
    return values[lower] + (values[upper] - values[lower]) * (at - lower)


def analyze(args):
    plan = json.loads(Path(args.plan).read_text())
    metric_mode = plan.get("metric_mode", "Live")
    if metric_mode not in ("Live", "Adopt"):
        raise RuntimeError("Unknown preregistered metric mode")
    record = json.loads(Path(args.results).read_text())
    if record["plan_sha256"] != digest(args.plan) or record["runner_sha256"] != digest(__file__):
        raise RuntimeError("Sealed plan or runner differs")
    if plan["runner_sha256"] != digest(__file__) or record["baseline_sha256"] != plan["baseline_sha256"]:
        raise RuntimeError("Baseline or calibration runner differs")
    counts = {row_name(row["dimensions"]): row["iterations"] for row in plan["rows"]}
    if set(counts) != {row_name(row) for row in ROWS} or plan["pairs"] != 10:
        raise RuntimeError("Plan rows or pair count differs")
    expected = {(pair, side, row_name(row)) for pair in range(1, 11)
                for side in ("baseline", "candidate") for row in ROWS}
    actual = [(v["pair"], v["side"], v["row"]) for v in record["samples"]]
    if set(actual) != expected or len(actual) != len(expected):
        raise RuntimeError("Missing, extra or duplicate samples")
    for sample in record["samples"]:
        if digest(sample["raw"]) != sample["raw_sha256"] or sample["exit"] != 0:
            raise RuntimeError("Raw sample altered or failed")
        matches = PATTERN.findall(Path(sample["raw"]).read_text())
        if len(matches) != 1:
            raise RuntimeError("Raw sample has missing or extra rows")
        found, count, ns, space, allocs = matches[0]
        parsed = {"ns": float(ns), "bytes": float(space), "allocs": float(allocs)}
        if (re.sub(r"-\d+$", "", found) != sample["row"] or
                int(count) != sample["count"] or int(count) != counts[sample["row"]] or
                any(not math.isfinite(value) or value < 0 for value in parsed.values()) or
                parsed["ns"] <= 0 or any(sample[key] != value for key, value in parsed.items())):
            raise RuntimeError("Raw numeric values or fixed work differ from cached sample")
        if digest(sample["command"][0]) != record[sample["side"] + "_sha256"]:
            raise RuntimeError("Sample binary differs from recorded input")
    rng = random.Random(20261017)
    summary = {"results_sha256": digest(args.results), "rows": {}}
    for row in ROWS:
        name = row_name(row)
        by_pair = {(v["pair"], v["side"]): v for v in record["samples"] if v["row"] == name}
        ratios = [by_pair[p, "candidate"]["ns"] / by_pair[p, "baseline"]["ns"] for p in range(1, 11)]
        boots = [statistics.median(rng.choices(ratios, k=10)) for _ in range(10000)]
        summary["rows"][name] = {"paired_ns_ratios": ratios,
            "median_ns_ratio": statistics.median(ratios),
            "ci95": [percentile(boots, 0.025), percentile(boots, 0.975)],
            "medians": {side: {metric: statistics.median(by_pair[p, side][metric] for p in range(1, 11))
                               for metric in ("ns", "bytes", "allocs")}
                        for side in ("baseline", "candidate")}}
    summary["metric_mode"] = metric_mode
    primary = summary["rows"][row_name((metric_mode, 128, 512))]
    control = summary["rows"][row_name((metric_mode, 1, 0))]
    summary["timing_gate"] = (primary["median_ns_ratio"] <= 0.8 and
                              primary["ci95"][1] < 1 and control["median_ns_ratio"] <= 1.05)
    write(args.output, summary)
    print(json.dumps(summary, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    calibration = commands.add_parser("calibrate")
    calibration.add_argument("--baseline", required=True)
    calibration.add_argument("--output", required=True)
    calibration.add_argument("--metric-mode", choices=("Live", "Adopt"), default="Live")
    comparison = commands.add_parser("compare")
    comparison.add_argument("--plan", required=True)
    comparison.add_argument("--baseline", required=True)
    comparison.add_argument("--candidate", required=True)
    comparison.add_argument("--output", required=True)
    analysis = commands.add_parser("analyze")
    analysis.add_argument("--plan", required=True)
    analysis.add_argument("--results", required=True)
    analysis.add_argument("--output", required=True)
    args = parser.parse_args()
    {"calibrate": calibrate, "compare": compare, "analyze": analyze}[args.command](args)


if __name__ == "__main__":
    main()
