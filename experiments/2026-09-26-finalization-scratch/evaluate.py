"""Frozen paired benchmark evaluator; stdlib only."""
import json
import random
import re
import statistics
import sys
from pathlib import Path

ROW = re.compile(r"^BenchmarkPendingWaifLocals-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$", re.M)


def read(path):
    text = Path(path).read_text(encoding="utf-8-sig")
    rows = [tuple(map(float, row)) for row in ROW.findall(text)]
    if len(rows) != 10 or len(re.findall(r"^PASS$", text, re.M)) != 10:
        raise ValueError("expected exactly ten passing rows per side")
    return rows


baseline, candidate = map(read, sys.argv[1:3])
rng = random.Random(20260926)
results = {}
for name, column in [("bytes_reduction_pct", 1), ("runtime_change_pct", 0)]:
    pairs = [(c[column] / b[column] - 1) * 100 for b, c in zip(baseline, candidate)]
    if column == 1:
        pairs = [-x for x in pairs]
    samples = sorted(statistics.median(rng.choices(pairs, k=10)) for _ in range(20000))
    results[name] = {"pairs": pairs, "median": statistics.median(pairs), "ci95": [samples[499], samples[19499]]}
for name, rows in [("baseline", baseline), ("candidate", candidate)]:
    results[name] = {metric: {"median": statistics.median(r[i] for r in rows), "min": min(r[i] for r in rows), "max": max(r[i] for r in rows)} for i, metric in enumerate(["ns/op", "B/op", "allocs/op"])}
allocation = results["bytes_reduction_pct"]
runtime = results["runtime_change_pct"]
passed = allocation["ci95"][0] >= 10 and runtime["median"] <= 5 and runtime["ci95"][0] <= 5
results["gate"] = "PASS" if passed else "FAIL"
print(json.dumps(results, indent=2))
sys.exit(0 if passed else 1)
