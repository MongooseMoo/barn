"""Refinement's frozen, fail-closed, paired evaluator."""
import json
import random
import re
import statistics
import sys
from pathlib import Path

root = Path(sys.argv[1])
pattern = re.compile(r'^(Benchmark(?:PendingWaifLocals|PendingFinalizationDiagnostic/\S+))-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$', re.M)
rows = {}
for side in ['baseline', 'candidate']:
    text = (root / f'{side}.txt').read_text(encoding='utf-8-sig')
    assert len(re.findall(r'^PASS$', text, re.M)) == 10
    table = {}
    for name, ns, byte, alloc in pattern.findall(text):
        table.setdefault(name, []).append(tuple(map(float, [ns, byte, alloc])))
    expected = {'BenchmarkPendingWaifLocals', 'BenchmarkPendingFinalizationDiagnostic/ParallelMixed'}
    for kind, counts in [('Anonymous', [1, 8, 256, 257]), ('Waif', [1, 8])]:
        expected.update(f'BenchmarkPendingFinalizationDiagnostic/{kind}/{n}/{path}' for n in counts for path in ['Value', 'Frame'])
    assert set(table) == expected and all(len(v) == 10 for v in table.values())
    rows[side] = table


def metric(baseline, candidate, column, reduction=False):
    pairs = [(c[column] / b[column] - 1) * (-100 if reduction else 100) for b, c in zip(baseline, candidate)]
    rng = random.Random(20260926)
    boot = sorted(statistics.median(rng.choices(pairs, k=10)) for _ in range(20000))
    return {'pairs': pairs, 'median': statistics.median(pairs), 'ci95': [boot[499], boot[19499]]}


results = {}
passed = True
for name in rows['baseline']:
    baseline, candidate = rows['baseline'][name], rows['candidate'][name]
    runtime = metric(baseline, candidate, 0)
    row_pass = runtime['ci95'][0] <= 5
    result = {
        'runtime_change_pct': runtime,
        'baseline_medians_ns_bytes_allocs': [statistics.median(r[i] for r in baseline) for i in range(3)],
        'candidate_medians_ns_bytes_allocs': [statistics.median(r[i] for r in candidate) for i in range(3)],
    }
    if name == 'BenchmarkPendingWaifLocals':
        result['bytes_reduction_pct'] = metric(baseline, candidate, 1, True)
        row_pass &= result['bytes_reduction_pct']['ci95'][0] >= 10 and runtime['median'] <= 5
    result['gate'] = 'PASS' if row_pass else 'FAIL'
    results[name] = result
    passed &= row_pass
results['gate'] = 'PASS' if passed else 'FAIL'
print(json.dumps(results, indent=2))
sys.exit(0 if passed else 1)
