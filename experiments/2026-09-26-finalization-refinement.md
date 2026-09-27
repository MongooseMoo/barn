# Selective finalization scratch reuse preregistration

Frozen before source or measurements. Parent authorized exactly one refinement
after diagnostics at 13c3c6e found small anonymous Value regressions.

Hypothesis: retaining the original temporary map/slice algorithm for every
non-WAIF Value preserves its allocation-free small-anonymous behavior, while
pooling direct WAIF Values and all eligible Frames retains worthwhile savings.
Single variable: collectPendingFinalizationsFromValue uses the original
algorithm for Type()!=TYPE_WAIF and existing pooled algorithm otherwise.
Pooled frame path, reset bound, all traversals and deduplication stay unchanged.
No new introspection or extra traversal; no further refinement authorized.

Baseline: 3c0f510 source; initial candidate: 6b72d0a; diagnostic result: 13c3c6e.
Boundary correctness tests and evaluator committed first at be0abb8.
Boundary contract output before source change:
`ok github.com/MongooseMoo/barn/vm 0.205s`, boundary_test_exit=0.
Contracts cover WAIF, ANON, LIST, MAP and plain OBJ against existing collector
semantics, including repeated identities. No MOO-semantic changes.

Primary metric: B/op reduction in BenchmarkPendingWaifLocals, 95% bootstrap
interval lower bound >=10%, runtime median increase <=5%. Safety gate: all
thirteen diagnostic rows from 000d0fe must have no credible >5% regression
(95% paired runtime-change interval lower bound <=5%). Missing or failed rows
fail closed. Oversize rows are included and cannot be dropped.

Plan: ten complete alternating AB/BA process pairs using identical evaluators
in original/refined source binaries. Each process:
`-test.run=^$ -test.bench=^(BenchmarkPendingWaifLocals|BenchmarkPendingFinalizationDiagnostic)$ -test.benchtime=200ms -test.benchmem -test.timeout=60s`.
One full process contains the primary plus thirteen diagnostic rows.
Default GOGC/GOMAXPROCS, Go1.26 Windows/amd64 Ryzen5950X, parent-exclusive CPU
slot. No early stopping or selective reruns. Relative to the initial primary
experiment the duration is now 200ms, declared in advance to keep fourteen-row
diagnostic pairs bounded; allocation metric is not timing-based.

Analysis: paired percent runtime change per row and primary bytes reduction;
median and percentile 95% bootstrap intervals, 20,000 draws, seed20260926 reset
per metric. Frozen evaluator evaluate-refinement.py writes all pairs/medians and
a fail-closed overall gate. Correctness: full `go test ./vm -count=1` must pass.
Any credible >5% regression means rejection, not another tuning iteration.

Evaluator paths (Git blob hashes):

- vm/pending_roots_bench_test.go: b506a5ebed626134162179b8083a040ce4c2cdc0
- vm/pending_roots_diagnostic_bench_test.go: 794d3e3d1f72f6b7f79e6cd4d0602e5e7501a3dc
- vm/frame_scan_bench_test.go: 30a7f0194c9a39f2610cc1643f0ceaafd8808493
- vm/finalization_scratch_test.go: 13cd3f43f6907223e6d37e55a4aa37d80b42c030
- vm/pending_roots_selection_test.go: 53933f1e1af0da4dde7188733c7bf5d2caa76bfd
- experiments/2026-09-26-finalization-scratch/evaluate-refinement.py: 7cfe41de84374dd475a309f6b8135a85b32d3d78

Same diagnostic overlays as b6f0b16: both binaries omit only the candidate-API
scratch-reset test via identical package-only replacement; baseline additionally
substitutes original anonymous_gc.go. The new selection-boundary test exists
identically in both builds. Prior raw evidence remains untouched; refinement
outputs go under experiments/2026-09-26-finalization-refinement/.
Reserved frame holdouts remain the independent verifier's responsibility.

## Results

Source commit: e764dc4; preregistration: 71639c9; boundary contracts/evaluator:
be0abb8. Ten complete alternating pairs, all fourteen rows per process, all
process exits 0. Frozen evaluator returned gate_exit=0 and gate=PASS. Seal diff
against 71639c9 for all six pinned paths was empty, seal_diff_exit=0.

Primary B/op reduction median 12.94195%, 95% interval [12.93678,12.94885].
Baseline median 1497561B/8318alloc; refined candidate 1303749B/129alloc.
Runtime median paired change -7.50994%, interval [-12.55894,-6.68807]. Raw primary
pair 8 had +148.86% runtime change; it is retained in the analysis. Timing remains
noisy and these microbenchmarks cannot establish application throughput.

| Row | Runtime change median, 95% interval | Baseline ns/B/alloc medians | Refined ns/B/alloc medians |
| --- | --- | --- | --- |
| PendingWaifLocals | -7.51% [-12.56,-6.69] | 3167138.5 / 1497561 / 8318 | 2925232.5 / 1303749 / 129 |
| Anonymous/1/Value | -3.85% [-5.55,-0.63] | 94.6 / 0 / 0 | 92.285 / 0 / 0 |
| Anonymous/1/Frame | -51.25% [-56.43,-47.90] | 326.85 / 192 / 2 | 158.25 / 0 / 0 |
| Anonymous/8/Value | -1.48% [-3.27,0.87] | 355.15 / 0 / 0 | 349.5 / 0 / 0 |
| Anonymous/8/Frame | -22.99% [-26.69,-20.00] | 616.95 / 192 / 2 | 464.2 / 0 / 0 |
| Anonymous/256/Value | -0.18% [-7.27,6.09] | 36632 / 18856 / 13 | 36817.5 / 18856 / 13 |
| Anonymous/256/Frame | -60.76% [-63.51,-58.91] | 39334.5 / 19048 / 15 | 15336.5 / 0 / 0 |
| Anonymous/257/Value | +2.93% [0.11,7.87] | 35535 / 18856 / 13 | 37130.5 / 18856 / 13 |
| Anonymous/257/Frame | +1.69% [-1.44,6.10] | 38413.5 / 19048 / 15 | 39183.5 / 19112.5 / 16 |
| Waif/1/Value | -31.19% [-36.30,-28.04] | 112.3 / 24 / 1 | 77.915 / 0 / 0 |
| Waif/1/Frame | -43.22% [-45.72,-40.57] | 217.95 / 72 / 2 | 123.3 / 0 / 0 |
| Waif/8/Value | -3.64% [-16.80,1.24] | 867.05 / 360 / 4 | 860.15 / 360 / 4 |
| Waif/8/Frame | -40.00% [-41.52,-34.87] | 1157 / 408 / 5 | 706.85 / 0 / 0 |
| ParallelMixed | -39.31% [-49.73,-31.50] | 857.05 / 912 / 10 | 544.25 / 362 / 4 |

The previously regressing small anonymous Value paths now retain zero allocations
and pass their runtime guards. Container Value paths retain original allocation
costs, intentionally foregoing the initial pool's wins on large anonymous lists.
Frame257 still discards oversized scratch and adds about 64.5B/one allocation;
its interval does not show a credible >5% runtime regression. This measured
tradeoff remains visible rather than being removed from the gate.

Correctness/build output before measurements:

```text
go test ./vm -count=1
ok github.com/MongooseMoo/barn/vm 7.827s
vm_test_exit=0
baseline_build_exit=0
candidate_build_exit=0
```

Both binaries used the documented overlays and identical new selection-boundary
test. Command paths: .tmp/refinement-baseline.test.exe and
.tmp/refinement-candidate.test.exe. Full raw output and frozen-evaluator output
are committed in the adjacent refinement directory as baseline.txt, candidate.txt,
and results.json. No evaluator/source edits after source commit e764dc4.

Worker recommendation: recommend promotion of the refined source, subject to
independent review, reserved plain-frame holdouts and application confirmation.
This supersedes the diagnostic rejection only for the newly narrowed source;
the initial all-Value pool remains rejected. No further tuning was performed.
