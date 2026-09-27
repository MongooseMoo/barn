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
