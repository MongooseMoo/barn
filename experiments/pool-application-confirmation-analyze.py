"""Independent fixed-ten-pair application confirmation analysis.

Exit 0: noninferiority demonstrated; 1: clear loss/correctness failure;
2: complete measurements but inconclusive. Invalid input raises an error.
"""
from pathlib import Path
import json
import random
import re
import statistics
import sys

root = Path(sys.argv[1])
destination = Path(sys.argv[2])
data = {}
for pair in range(10):
    for side in ("baseline", "candidate"):
        text = (root / f"{pair}-{side}.txt").read_text(encoding="utf-8-sig")
        if not text.rstrip().endswith("exit=0"):
            raise ValueError(f"unfinished or failed process: {pair}-{side}")
        lines = [line for line in text.splitlines() if re.search(r"players=\d+ goodput=", line)]
        if len(lines) != 1:
            raise ValueError(f"expected exactly one player row: {pair}-{side}")
        row = {key: float(value) for key, value in re.findall(r"([\w/]+)=([\d.]+)", lines[0])}
        for latency in ("p50", "p99"):
            duration = re.search(rf"\b{latency}=([\d.]+)(ns|us|µs|μs|ms|s)\b", lines[0])
            if duration is None:
                raise ValueError(f"missing or unsupported duration: {pair}-{side} {latency}")
            scale_ms = {"ns": 0.000001, "us": 0.001, "µs": 0.001, "μs": 0.001, "ms": 1, "s": 1000}
            row[latency] = float(duration[1]) * scale_ms[duration[2]]
        if row["players"] != 16:
            raise ValueError(f"wrong workload: {pair}-{side}")
        data[pair, side] = row

rng = random.Random(20260929)
results = {}
for metric in ("goodput", "bytes/op", "allocs/op", "p50", "p99", "abort"):
    baseline = [data[i, "baseline"][metric] for i in range(10)]
    candidate = [data[i, "candidate"][metric] for i in range(10)]
    # Goodput must be positive. Zero diagnostic denominators are represented
    # as differences without silently dropping an observation.
    if metric == "goodput" and any(value <= 0 for value in baseline + candidate):
        raise ValueError("nonpositive goodput")
    if all(value > 0 for value in baseline):
        paired = [c / b for b, c in zip(baseline, candidate)]
        statistic = "ratio"
    else:
        paired = [c - b for b, c in zip(baseline, candidate)]
        statistic = "difference"
    bootstrap = sorted(statistics.median(rng.choices(paired, k=10)) for _ in range(20000))
    results[metric] = {
        "statistic": statistic,
        "paired": paired,
        "median": statistics.median(paired),
        "ci95": [bootstrap[499], bootstrap[19499]],
        "baseline": {"values": baseline, "median": statistics.median(baseline), "range": [min(baseline), max(baseline)]},
        "candidate": {"values": candidate, "median": statistics.median(candidate), "range": [min(candidate), max(candidate)]},
    }
counts = {
    side: {metric: [data[i, side][metric] for i in range(10)] for metric in ("failed", "uncaught")}
    for side in ("baseline", "candidate")
}
if any(value != 0 for metrics in counts.values() for values in metrics.values() for value in values):
    verdict, exit_code = "CORRECTNESS_FAILURE", 1
elif results["goodput"]["ci95"][1] < 0.95:
    verdict, exit_code = "CLEAR_LOSS", 1
elif results["goodput"]["ci95"][0] >= 0.95:
    verdict, exit_code = "NONINFERIOR", 0
else:
    verdict, exit_code = "INCONCLUSIVE", 2
output = dict(verdict=verdict, pairs=10, seed=20260929, resamples=20000, latency_units="ms", correctness=counts, metrics=results)
encoded = json.dumps(output, indent=2)
if destination.exists():
    raise FileExistsError(f"refusing to overwrite {destination}")
destination.write_text(encoded + "\n", encoding="utf-8")
print(encoded)
sys.exit(exit_code)
