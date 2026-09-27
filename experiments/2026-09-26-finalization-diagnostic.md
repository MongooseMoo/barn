# Finalization pooling workload-gap diagnostic preregistration

Baseline source: 3c0f510. Candidate source: 6b72d0a. Previous experiment evidence:
17aa576. This diagnostic does not alter the previous frozen evaluator or metric;
it adds independent adversarial workloads before promotion. Production is frozen.

Expectations, declared before measurement: anonymous-only single-value baseline
may have stack-resident scratch and zero heap allocations, leaving no allocation
benefit to compensate Pool.Get/Put. Repeated small WAIF sets should reduce slice
allocations. Anonymous sets above the retention cutoff (257) should show little
reuse benefit and may regress because every scratch object is discarded. Mixed
parallel workloads may expose pool/GC effects hidden by sequential measurements.
These are hypotheses, not established root causes of the application regression.

Evaluator: vm/pending_roots_diagnostic_bench_test.go. Rows: Anonymous 1/8/256/257
and Waif 1/8, each Value and Frame paths; plus ParallelMixed with eight anonymous
and eight WAIF roots and both collection paths per operation. Sequential rows
warm one VM, then repeat the same roots to isolate temporary collection. Final
root counts must remain correct. Shared immutable container cache is resolved
before the parallel row starts. No MOO-semantic changes or assertions are added.

Plan: identical diagnostic evaluator source compiled for baseline/candidate;
ten alternating AB/BA process pairs, each `-test.run=^$`
`-test.bench=^BenchmarkPendingFinalizationDiagnostic$ -test.benchtime=200ms`
`-test.benchmem -test.timeout=60s`. Default GOMAXPROCS/GOGC. Parent authorizes
the serialized CPU slot before any build or measurement. Preserve every row.

Analysis: paired runtime percentage change, median and 95% percentile bootstrap
interval (20,000 resamples, seed 20260926) per row; also report allocated bytes
and allocations per operation. A credible regression means the lower endpoint
exceeds +5%. Any such row requires diagnosis and potentially dropping/refining
the finalization pool; it cannot be dismissed because the original WAIF row won.
Multiple-row comparisons are exploratory and no new source tuning is authorized
by this diagnostic. Missing rows, failed assertions, nonzero exits or mismatched
evaluator sources invalidate the affected measurement. No selective reruns.

No baseline/candidate builds or CPU tests have run as of this preregistration.

## Evaluator preparation (before builds or measurements)

Preregistration/harness commit: 000d0fe. Harness Git blob:
794d3e3d1f72f6b7f79e6cd4d0602e5e7501a3dc; on-disk SHA256:
18096FC819941DC37040D91130603A7209996F03F6A2383BF48E420ACF1EE460.

Go overlays make both binaries share the same diagnostic evaluator and all other
tests. Both replace vm/finalization_scratch_test.go (candidate-only API contracts)
with `.tmp/diagnostic-empty-test.go`, containing only `package vm`, SHA256
557693541E08F709730DB064B251AED9CCC4135295D757E1481DC6BB5F52AE51.
Baseline additionally replaces vm/anonymous_gc.go with output from
`git show 3c0f510:vm/anonymous_gc.go`, SHA256
C438C6134445076D0F2F08856D1B6802B2FCA265AFF37D3870EF340F861E8B87.
No other overlay substitutions. Overlay files reside under .tmp as
diagnostic-baseline-overlay.json and diagnostic-candidate-overlay.json.
Production checkout files are unchanged. The previous contract tests remain
unchanged on disk and committed; only these diagnostic binaries exclude them.

Sequential rows warm PendingFinalizations and identity maps once before b.Loop;
they do not drain/reinitialize those retained maps during measurement. This
isolates repeated-root scratch overhead, not task/new-VM lifecycle cost.
ParallelMixed initializes one VM per benchmark worker inside RunParallel, so
amortized per-worker initialization is included identically on both sides.
