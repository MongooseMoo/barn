"""Measure the preregistered list-cache overhead against compiled test binaries."""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import random
import statistics
import subprocess


def parse_benchmarks(output):
    rows = {}
    for line in output.splitlines():
        words = line.split()
        if not words or not words[0].startswith("BenchmarkListMetadata/"):
            continue
        name = words[0].rsplit("-", 1)[0]
        rows[name] = {
            words[index + 1]: float(words[index])
            for index in range(2, len(words), 2)
        }
    if len(rows) != 6:
        raise ValueError(f"expected six benchmark rows, got {sorted(rows)}")
    return rows


def paired_interval(ratios, seed):
    logs = [math.log(ratio) for ratio in ratios]
    rng = random.Random(seed)
    samples = sorted(
        math.exp(statistics.mean(rng.choices(logs, k=len(logs))))
        for _ in range(10000)
    )
    return {
        "percent": 100 * (math.exp(statistics.mean(logs)) - 1),
        "ci95_percent": [100 * (samples[index] - 1) for index in (249, 9749)],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True, type=Path)
    parser.add_argument("--candidate", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    binaries = {"baseline": args.baseline.resolve(), "candidate": args.candidate.resolve()}
    hashes = {key: hashlib.sha256(path.read_bytes()).hexdigest() for key, path in binaries.items()}
    version = subprocess.run(
        ["go", "version", *map(str, binaries.values())],
        check=True, capture_output=True, text=True,
    ).stdout.strip()
    env = dict(os.environ, GOMAXPROCS="4")
    rng = random.Random(355)
    records = []
    raw = {key: [] for key in binaries}
    for index in range(14):
        phase = "main" if index < 12 else "holdout"
        order = ["baseline", "candidate"]
        rng.shuffle(order)
        pair = {"pair": index + 1, "phase": phase, "order": order}
        for key in order:
            completed = subprocess.run(
                ["taskset", "-c", "8-11", str(binaries[key]),
                 "-test.run=^$", "-test.bench=^BenchmarkListMetadata$",
                 "-test.benchmem", "-test.benchtime=300ms", "-test.count=1"],
                env=env, check=True, capture_output=True, text=True, timeout=180,
            )
            raw[key].append(f"pair: {index + 1}\nphase: {phase}\n{completed.stdout}")
            pair[key] = parse_benchmarks(completed.stdout)
        if pair["baseline"].keys() != pair["candidate"].keys():
            raise ValueError("baseline and candidate workloads differ")
        records.append(pair)
        print(f"pair {index + 1}/14 {phase} order={','.join(order)}", flush=True)
    for key, path in binaries.items():
        if hashlib.sha256(path.read_bytes()).hexdigest() != hashes[key]:
            raise ValueError(f"{key} binary changed during measurement")
    summary = {}
    for index, name in enumerate(sorted(records[0]["baseline"])):
        ratios = [row["candidate"][name]["ns/op"] / row["baseline"][name]["ns/op"] for row in records]
        metrics = paired_interval(ratios[:12], 355 + index)
        metrics["holdout_percent"] = 100 * (math.sqrt(ratios[12] * ratios[13]) - 1)
        for metric in ("B/op", "allocs/op", "header-B"):
            if metric in records[0]["baseline"][name]:
                metrics[metric] = {
                    key: sorted({row[key][name][metric] for row in records})
                    for key in binaries
                }
        summary[name] = metrics
    result = {"toolchain": version, "binary_sha256": hashes, "seed": 355,
              "gomaxprocs": 4, "affinity": "8-11", "duration": "300ms",
              "main_pairs": 12, "holdout_pairs": 2, "summary": summary, "pairs": records}
    for key in binaries:
        # Trim output padding while retaining every measurement and its order.
        output = "\n".join(line.rstrip() for line in "\n".join(raw[key]).splitlines()) + "\n"
        (args.output / f"{key}.txt").write_text(output)
    (args.output / "results.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(summary, indent=2), flush=True)


if __name__ == "__main__":
    main()
