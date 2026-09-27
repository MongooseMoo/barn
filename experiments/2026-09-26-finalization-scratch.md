# Pending finalization scratch reuse

Date: 2026-09-26

Experiment branch: perf/pool-finalization

## Preregistration (frozen)

Hypothesis: bounded reuse of temporary refs/waifs collection reduces allocated
bytes in repeated finalizable local release by at least 10%, without a meaningful
runtime regression. Single variable: replace the temporary collections used by
collectPendingFinalizationsFromValue and collectPendingFinalizationsFromFrame
with a shared sync.Pool of pointer-owned scratch, preserving traversal and
appendPendingFinalizationRoots ordering. Plain values never acquire scratch.
Cap reusable refs length and waifs capacity at 256; clear roots before return.

Origin: user-requested audit of all four scratch candidates. Prior-art search:
`rg -n 'pending|finalization|pool' experiments --glob '*.md'` found frame/waif
workload context, but no previous scratch-pool experiment. Direct accumulation
would change anonymous-before-waif batching; VM-owned scratch would retain a
buffer per VM. Pooling preserves current operations with bounded transient storage.

Baseline: 3c0f5105e30dd557734deac200ff0fec51faae8f. Baseline binary compiled before
candidate-specific red tests, using `go test -c -o .tmp/finalization-baseline.test.exe ./vm`.
Benchmark sources unchanged between baseline and sealed evaluator commit d0c04c3.
Tests intentionally compile-fail on baseline (undefined scratch API); no baseline
benchmark or measurement evaluator is changed by those tests.

Primary metric: B/op in BenchmarkPendingWaifLocals (4096 distinct waifs, each
released twice through production releaseLocal, fresh VM per iteration).
Exact paired command: `./experiments/2026-09-26-finalization-scratch/measure.ps1`.
Each process uses `-test.run=^$ -test.bench=^BenchmarkPendingWaifLocals$ -test.benchtime=1s -test.benchmem -test.timeout=30s`.
Ten complete paired samples, alternating AB/BA; no early stop or tuning sweep.
Default GOMAXPROCS/GOGC; Go 1.26.0 windows/amd64, Ryzen 9 5950X, 32 logical CPUs.
Parent serializes all performance workloads during measurements.

Analysis: frozen evaluate.py computes per-pair percent bytes reduction, median,
and percentile bootstrap 95% interval (20,000 resamples, seed 20260926).
Pass requires allocation reduction interval lower bound >=10%; runtime change
median <=5% and no significant >5% regression (runtime interval lower bound <=5%).
Report runtime median and min/max separately; all malformed/missing rows and
nonzero process exits fail closed. Command:
`python experiments/2026-09-26-finalization-scratch/evaluate.py experiments/2026-09-26-finalization-scratch/baseline-paired.txt experiments/2026-09-26-finalization-scratch/candidate-paired.txt`.

Sealed evaluator paths and Git blob hashes at d0c04c3:

- vm/pending_roots_bench_test.go: b506a5ebed626134162179b8083a040ce4c2cdc0
- vm/frame_scan_bench_test.go: 30a7f0194c9a39f2610cc1643f0ceaafd8808493
- vm/finalization_scratch_test.go: 13cd3f43f6907223e6d37e55a4aa37d80b42c030
- experiments/2026-09-26-finalization-scratch/evaluate.py: ecc724f90efd2f39e9bd8e26be140d63ea9cd03a
- experiments/2026-09-26-finalization-scratch/measure.ps1: fd9cffc33e3f37f9df3ce5b09da158a0c303f0f0

Fast contracts: `go test ./vm -count=1`; new tests preserve nested list/map
identity deduplication, ordering, frame collection, preseeded roots and VM
isolation. Scratch reset tests call Pool.New directly (miss path), assert every
backing slot is cleared, and reject excess map length or slice capacity.
Existing anonymous cycle and pending waif drain tests remain unchanged.
No new MOO semantic assertion; this is internal ownership reuse only.

Kill criteria: correctness failure, changed evaluator, missing measurements,
allocation interval below gate, runtime failure, retained Value references,
unbounded scratch retention, or source changes beyond the single variable.
Falsification: scratch allocation is too small to clear 10% or pool overhead
exceeds the runtime gate. Profile candidate before recommending rejection.

Holdout: BenchmarkPlainFrameFinalization and BenchmarkTemporaryFrameLifecycle,
reserved for separate verifier, not run by experiment worker. Require no
significant >5% regression and no allocation growth in plain-frame path.

## Baseline profile

Command: baseline binary with same row, 2s benchtime, memprofile and cpuprofile.
Raw artifacts under experiments/2026-09-26-finalization-scratch/. Baseline
profile row: 2503241 ns/op, 1497448 B/op, 8318 allocs/op. alloc_space profile:
collectDirectFinalizationRoots 147MB / 12.46%; appendPendingFinalizationRoots
43.55%; collectDirectWaifsForGC 43.28%. The proposed scratch reuse addresses
the 12.46% slice allocation, not the persistent pending collections.
Initial command was rejected because PowerShell split unquoted dotted flags;
quoted correction passed (exit 0). Rejected invocation produced no benchmark.
Profile binaries and raw pprof files remain uncommitted under .tmp; text profiles
are committed. Baseline retained profile did not attribute sampled live bytes
to finalization (sampling is not proof of zero live bytes).
