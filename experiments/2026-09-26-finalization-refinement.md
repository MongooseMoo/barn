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
