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

## Results and diagnosis

Recommendation: do not promote the finalization pool unchanged. This diagnostic
supersedes the original development-row recommendation: two anonymous-only Value
rows have credible regressions above the declared 5% threshold. The original
WAIF result remains valid for its workload but did not generalize to these rows.

Build commands: `go test -overlay .tmp/diagnostic-baseline-overlay.json -c -o
.tmp/diagnostic-baseline.test.exe ./vm` and corresponding candidate paths.
Both build exits were 0. All ten pairs, thirteen rows per process, completed with
exit 0. Raw output: diagnostic-baseline.txt and diagnostic-candidate.txt in
2026-09-26-finalization-scratch. Diagnostic evaluator SHA256 after measurement
remained 18096FC819941DC37040D91130603A7209996F03F6A2383BF48E420ACF1EE460.
Production source did not change during these measurements.

Reproduction analysis command:
`python experiments/2026-09-26-finalization-scratch/evaluate-diagnostic.py`.
The script implements the declared paired-bootstrap plan, validates all 260 rows
and twenty PASS markers, and writes every paired percentage and median to
diagnostic-results.json. Analysis exit was 0 (successfully analyzed, not a
performance pass). Per-row runtime changes below are paired percentages; median
runtime quotients need not equal the median paired percentage.

| Row | Runtime change median, 95% interval | Baseline ns/B/alloc medians | Candidate ns/B/alloc medians |
| --- | --- | --- | --- |
| Anonymous/1/Value | +53.00% [41.68,56.20] | 98.52 / 0 / 0 | 148 / 0 / 0 |
| Anonymous/1/Frame | -46.46% [-71.32,-43.63] | 335.7 / 192 / 2 | 171.7 / 0 / 0 |
| Anonymous/8/Value | +14.94% [9.83,19.22] | 365.95 / 0 / 0 | 403.9 / 0 / 0 |
| Anonymous/8/Frame | -25.70% [-40.16,-21.70] | 708.3 / 192 / 2 | 483.25 / 0 / 0 |
| Anonymous/256/Value | -65.68% [-66.62,-63.69] | 39957.5 / 18856 / 13 | 13713 / 0 / 0 |
| Anonymous/256/Frame | -64.55% [-68.16,-60.63] | 44554 / 19048 / 15 | 15703.5 / 0 / 0 |
| Anonymous/257/Value | +4.26% [-1.97,68.74] | 41482 / 18856 / 13 | 43426 / 19114 / 16 |
| Anonymous/257/Frame | +4.36% [-5.30,45.61] | 43279.5 / 19048 / 15 | 44771 / 19115 / 16 |
| Waif/1/Value | -29.93% [-34.19,-23.79] | 115.7 / 24 / 1 | 80.865 / 0 / 0 |
| Waif/1/Frame | -49.09% [-52.60,-42.91] | 236.2 / 72 / 2 | 124.05 / 0 / 0 |
| Waif/8/Value | -51.11% [-54.50,-45.98] | 1006.4 / 360 / 4 | 503.35 / 0 / 0 |
| Waif/8/Frame | -43.01% [-59.10,-37.23] | 1259 / 408 / 5 | 727.7 / 0 / 0 |
| ParallelMixed | -79.66% [-89.56,-77.35] | 1072 / 912 / 10 | 223.45 / 0 / 0 |

Operational diagnosis: small anonymous-only Value collection already allocates
zero bytes in the baseline. Pool access/reset adds work without removing an
allocation there. The frame path differs: baseline allocates 192B/two objects,
and pooling removes those allocations. WAIF paths also remove actual slice
allocation. At 257 anonymous IDs scratch exceeds the 256-entry retention cap,
so each collection discards its scratch; candidate allocations increase instead
of falling. That row's runtime interval is too wide to claim a credible slowdown,
but the allocation penalty and absence of reuse follow directly from the reset
bound and measured counts. No broad concurrency regression is evident in the
mixed parallel row. These microbenchmarks do not establish which path dominates
the separate application regression; that still requires application profiling.

No refinement implemented. A follow-up should evaluate keeping small anonymous
Value collection on its allocation-free path while preserving proven frame/WAIF
reuse, or reject this pool if avoiding the regressions adds unjustified complexity.
Any follow-up requires a new frozen experiment and all diagnostic rows as gates.
