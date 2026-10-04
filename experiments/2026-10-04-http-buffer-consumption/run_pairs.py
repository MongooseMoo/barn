"""Run the preregistered paired HTTP buffer-consumption comparison in WSL."""

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
        if words and words[0].startswith("BenchmarkHTTPBufferDrain/"):
            rows[words[0].rsplit("-", 1)[0]] = {
                words[index + 1]: float(words[index])
                for index in range(2, len(words), 2)
            }
    if len(rows) != 16:
        raise ValueError(f"expected sixteen workloads, got {sorted(rows)}")
    return rows


def paired_interval(ratios, seed):
    logs = [math.log(ratio) for ratio in ratios]
    rng = random.Random(seed)
    samples = sorted(
        math.exp(statistics.mean(rng.choices(logs, k=len(logs))))
        for _ in range(10000)
    )
    return {"percent": 100 * (math.exp(statistics.mean(logs)) - 1),
            "ci95_percent": [100 * (samples[index] - 1) for index in (249, 9749)]}


def save_text(path, content):
    path.write_bytes(("\n".join(line.rstrip() for line in content.splitlines()) + "\n").encode())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True, type=Path)
    parser.add_argument("--candidate", required=True, type=Path)
    parser.add_argument("--benchstat", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    binaries = {"baseline": args.baseline.resolve(), "candidate": args.candidate.resolve()}
    hashes = {key: hashlib.sha256(path.read_bytes()).hexdigest() for key, path in binaries.items()}
    version = subprocess.run(["go", "version", *map(str, binaries.values())],
                             check=True, capture_output=True, text=True).stdout.strip()
    env = dict(os.environ, GOMAXPROCS="4")
    rng = random.Random(351)
    records = []
    raw = {(key, phase): [] for key in binaries for phase in ("main", "holdout")}
    for index in range(14):
        phase = "main" if index < 12 else "holdout"
        order = ["baseline", "candidate"]
        rng.shuffle(order)
        pair = {"pair": index + 1, "phase": phase, "order": order}
        for key in order:
            completed = subprocess.run(
                ["taskset", "-c", "8-11", str(binaries[key]), "-test.run=^$",
                 "-test.bench=^BenchmarkHTTPBufferDrain$", "-test.benchmem",
                 "-test.benchtime=200ms", "-test.count=1"],
                env=env, check=True, capture_output=True, text=True, timeout=180,
            )
            raw[key, phase].append(completed.stdout)
            pair[key] = parse_benchmarks(completed.stdout)
        if pair["baseline"].keys() != pair["candidate"].keys():
            raise ValueError("workload sets differ")
        records.append(pair)
        print(f"pair {index + 1}/14 {phase} order={','.join(order)}", flush=True)
    for key, path in binaries.items():
        if hashlib.sha256(path.read_bytes()).hexdigest() != hashes[key]:
            raise ValueError(f"{key} binary changed during measurement")
    summary = {}
    for index, name in enumerate(sorted(records[0]["baseline"])):
        ratios = [row["candidate"][name]["ns/op"] / row["baseline"][name]["ns/op"] for row in records]
        metrics = paired_interval(ratios[:12], 351 + index)
        metrics["holdout_percent"] = 100 * (math.sqrt(ratios[12] * ratios[13]) - 1)
        for metric in ("B/op", "allocs/op"):
            metrics[metric] = {
                key: sorted({row[key][name][metric] for row in records}) for key in binaries
            }
        summary[name] = metrics
    for (key, phase), output in raw.items():
        suffix = "" if phase == "main" else "-holdout"
        save_text(args.output / f"{key}{suffix}.txt", "\n".join(output))
    compared = subprocess.run(
        [str(args.benchstat.resolve()), str(args.output / "baseline.txt"), str(args.output / "candidate.txt")],
        check=True, capture_output=True, text=True,
    ).stdout
    save_text(args.output / "benchstat.txt", compared)
    result = {"toolchain": version, "binary_sha256": hashes, "seed": 351,
              "gomaxprocs": 4, "affinity": "8-11", "duration": "200ms",
              "main_pairs": 12, "holdout_pairs": 2, "summary": summary, "pairs": records}
    save_text(args.output / "results.json", json.dumps(result, indent=2))
    print(json.dumps(summary, indent=2), flush=True)


if __name__ == "__main__":
    main()
