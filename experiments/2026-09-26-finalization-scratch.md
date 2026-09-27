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

## Results

Evidence commits: contracts/evaluator d0c04c3; preregistration d171cfe;
source delta 6b72d0a. Worker recommendation: recommend promotion, subject to
independent verification and reserved frame holdouts. No integration/push action
was performed by the experiment worker.

Ten alternating paired runs completed, every process exit 0. Frozen evaluator:

```text
bytes_reduction_pct median 12.946036838841245
95% interval [12.944697685041117, 12.948421204009573]
runtime_change_pct median -16.495310676634254
95% interval [-20.24448898453518, -13.796964237515258]
gate PASS
gate_exit=0
```

| Metric | Baseline median [min,max] | Candidate median [min,max] |
| --- | --- | --- |
| ns/op | 3089807 [2832229,3230228] | 2502690 [2427405,2716468] |
| B/op | 1497474 [1497460,1497488] | 1303602.5 [1303523,1303652] |
| allocs/op | 8318 [8318,8318] | 129 [128,129] |

Raw paired output and every per-pair percentage are committed beside the frozen
evaluator as baseline-paired.txt, candidate-paired.txt and result.json. Profile
runs are separate from the gate. Candidate profile command is the baseline
command with candidate binary/profile paths. Candidate profile raw row:

```text
BenchmarkPendingWaifLocals-32 1053 2274323 ns/op 1302761 B/op 128 allocs/op
PASS
profile_exit=0
```

Candidate alloc_space now attributes 50.36% to appendPendingFinalizationRoots
and 48.68% to collectDirectWaifsForGC. The temporary collector allocation has
shrunk below the displayed profile threshold; remaining dominant cost is the
VM-owned pending lists/maps, which this experiment does not change. Total profile
bytes cannot be compared directly because the faster candidate did more work.
In-use samples total 3596.64kB baseline versus 4107.34kB candidate, all attributed
to runtime/time initialization. This sampling does not establish a live-heap
reduction or exclude a small pooled allocation. Structural bounds, reference
clearing contracts and bounded per-entry retention are the stronger evidence.

Fast contract output, captured before measurement with the exact source delta:

```text
go test ./vm -count=1
ok  github.com/MongooseMoo/barn/vm 8.021s
vm_test_exit=0
go test -c -o .tmp/finalization-candidate.test.exe ./vm
compile_exit=0
```

Seal check: `git diff --exit-code d171cfe HEAD --` the five preregistered evaluator
paths emitted no diff and returned evaluator_diff_exit=0. No source changes after
6b72d0a. Existing full VM suite covers anonymous cycles and root/drain behavior;
new contracts cover scratch miss construction, reset, oversize rejection, nested
aliases, batch order and VM isolation. Ref maps only grow during collection, so
their pre-clear length bounds their largest borrowed size. Value slices are
cleared before reuse, with capacity capped at 256; oversized scratch is discarded.
No scratch references escape into VM-owned root maps/slices. Pool eviction or
GC simply invokes New on a later miss and affects performance only.

Limitations: development row is WAIF-heavy releaseLocal bookkeeping, not an
end-to-end throughput estimate. Frame holdouts and independent adversarial
review remain the promotion actor's responsibility. The worker did not execute
either reserved frame benchmark. No retained-heap improvement is claimed.
