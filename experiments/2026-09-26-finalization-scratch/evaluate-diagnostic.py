import json
import random
import re
import statistics
from pathlib import Path

root = Path('experiments/2026-09-26-finalization-scratch')
pattern = re.compile(r'^(BenchmarkPendingFinalizationDiagnostic/\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$', re.M)
rows = {}
for side in ['baseline', 'candidate']:
    text = (root / f'diagnostic-{side}.txt').read_text(encoding='utf-8-sig')
    assert len(re.findall(r'^PASS$', text, re.M)) == 10
    table = {}
    for name, ns, byte, alloc in pattern.findall(text):
        table.setdefault(name, []).append(tuple(map(float, [ns, byte, alloc])))
    assert len(table) == 13 and all(len(v) == 10 for v in table.values())
    rows[side] = table
assert rows['baseline'].keys() == rows['candidate'].keys()
results = {}
for name in rows['baseline']:
    baseline, candidate = rows['baseline'][name], rows['candidate'][name]
    changes = [(c[0] / b[0] - 1) * 100 for b, c in zip(baseline, candidate)]
    rng = random.Random(20260926)
    boot = sorted(statistics.median(rng.choices(changes, k=10)) for _ in range(20000))
    ci = [boot[499], boot[19499]]
    results[name] = {
        'runtime_change_pct': {'pairs': changes, 'median': statistics.median(changes), 'ci95': ci},
        'baseline_medians_ns_bytes_allocs': [statistics.median(r[i] for r in baseline) for i in range(3)],
        'candidate_medians_ns_bytes_allocs': [statistics.median(r[i] for r in candidate) for i in range(3)],
        'credible_regression': ci[0] > 5,
    }
    print(name.removeprefix('BenchmarkPendingFinalizationDiagnostic/'), results[name])
(root / 'diagnostic-results.json').write_text(json.dumps(results, indent=2) + '\n', encoding='utf-8')
