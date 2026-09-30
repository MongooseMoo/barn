# Commit scratch experiment preregistration

Hypothesis: reusing the decentralized commit's temporary footprint, locked-slot,
and grouped-write storage reduces allocated bytes per released multi-object
transaction without changing validation, lock order, publication, or ownership.
This is one independent candidate in the parent's scratch-pooling campaign.
User requested all worthwhile candidates; the ownership review identified this
one. No whole transaction or published object image is eligible for pooling.

Prior art: searched tracked experiments for pool/scratch, db/store pooling,
git history for pool/scratch/commit allocation, and the main checkout's untracked
2026-06-24 commit-dominated concurrency ledger. Historical scheduler/VM pooling
and reverted transaction experiments do not establish current commit scratch
benefit. No current local commit scratch experiment found.

Single variable: bounded reusable commit-local scratch, including grouped inner
slices. Preserve existing per-kind application and property-definition insertion
order. Retain at most capacity 128 for IDs, slot pointers and groups, at most 128
map entries at reset, and at most 256 aggregate capacity across every retained
group's six inner slices. Clear all retained references before returning scratch.
The pool borrow ends after all slot locks have been released, including errors.

Baseline: source 3c0f510, benchmark harness commit 04dfc97, compiled before the
candidate-specific red contracts. Baseline binary .tmp/commit-baseline.test.exe.
Candidate binary .tmp/commit-candidate.test.exe. Baseline contains exactly the
same benchmark body; candidate-specific reset tests intentionally do not compile
on baseline. Red contract commit 4b07992 emitted undefined commitScratch and
newCommitScratch, exit 1. Evaluator commit 25d9776 precedes preregistration.

Sealed evaluator paths and git blobs:
- db/store/commit_scratch_bench_test.go ca93bcfe377fcada5017d57a6c5784e001c25a04
- db/store/commit_scratch_test.go f60004d0b227cf19bada061d2815d86d8782480e
- experiments/commit-scratch-evaluate.py c37c7cf01ed013c7784f30d8f494f1a8ffc20326
All other existing db/store tests are unchanged and sealed by the prereg commit.

Primary metric: B/op for BenchmarkCommitScratch/objects8, complete production
begin/stage/commit/release lifecycle. Minimum effect: median paired reduction
at least 10%, with paired 95% bootstrap median interval excluding zero. Secondary
guard: no statistically supported runtime regression over 5% on any development
row (lower bound of paired runtime percent CI must be <=5%). Report runtime
medians and full paired spread; no runtime speedup claim unless CI excludes zero.

Exact invocation per binary:
`<binary> -test.run=^$ -test.bench=^BenchmarkCommitScratch$ -test.benchmem -test.benchtime=300ms -test.count=1 -test.cpu=4 -test.timeout=120s`
Ten pairs, deterministic identical instances objects1/objects8/objects32. Odd
pairs baseline then candidate; even pairs candidate then baseline. Save stdout
and exit status for each invocation under experiments/2026-09-26-commit-scratch/.
Run only with parent measurement slot. No concurrent performance commands.
Analysis: `python experiments/commit-scratch-evaluate.py experiments/2026-09-26-commit-scratch`.
The frozen evaluator uses 10,000 bootstrap median resamples, seed 926, reporting
paired percentage change and percentile interval. Missing rows/PASS or failed
commands fail closed. No peeking/stopping; finish all ten pairs.

Instrumentation: profile baseline before source edits and candidate after using
the same objects8 row, -test.benchtime=2s, -test.cpu=4,
-test.memprofilerate=1 and -test.memprofile=<side>.mem; inspect alloc_space and
alloc_objects with Go pprof. Profiles explain allocation movement, not timing.

Fast contracts: new reset/reference clearing/oversize/conflict tests plus existing
db/store full tests and race suite, including mixed kinds, property insertion
order, stale snapshots and concurrent read/write lock coverage. Failure kills
promotion. Unknown MOO behavior stops work pending the managed oracle.
Holdout BenchmarkCommitObjectOrdering is reserved for the independent verifier;
worker must not run it. Independent reviewer checks ownership and evaluator seal.

Kill/falsification: failing primary effect, runtime guard, contract, retained
capacity bound, source ownership, or changed evaluator means do not promote.
No second tuned candidate under this record. On failure profile to explain
whether scratch allocation shrank and identify remaining costs. Promotion is
the parent's decision after independent recomputation and holdout.

## Baseline profile and disposition (appended after preregistration)

Recommendation: abandon this complex pooled commit-scratch proposal before
implementation. This is profile-based rejection, not a measured A/B loss and
not evidence that every commit optimization is worthless. Parent explicitly
approved stopping after reviewing the baseline ceiling; no candidate source,
paired runs, candidate profile, timing gate result, or holdout result exists.
The preregistered ten-pair plan was not executed. No promotion gate is claimed.

Environment: Go 1.26.0 windows/amd64, Ryzen 9 5950X, benchmark cpu=4. Exact
successful PowerShell invocation (all dotted test flags must be quoted):
`& .tmp/commit-baseline.test.exe '-test.run=^$' '-test.bench=^BenchmarkCommitScratch/objects8$' '-test.benchmem' '-test.benchtime=2s' '-test.cpu=4' '-test.memprofilerate=1' '-test.memprofile=.tmp/commit-base.mem' '-test.timeout=120s'`
Exit 0, raw output:
```
BenchmarkCommitScratch/objects8-4 4599 549803 ns/op 22634 B/op 147 allocs/op
PASS
```
Timing above includes allocation profiling overhead and is NOT a performance
baseline. An earlier wrapper attempt left dotted flags unquoted; PowerShell
passed an invalid -test flag, producing exit 2 and no profile. It was corrected
before any data existed. No failed metric run was silently rerun.

Raw profiles/text: sibling 2026-09-26-commit-scratch directory contains the binary
memory profile, benchmark stdout, pprof alloc_space, alloc_objects, and annotated
commit source. Commands: `go tool pprof -top -alloc_space <baseline-binary> <mem>`;
likewise -alloc_objects, and `-list=commitDecentralized -alloc_space`.

Observed alloc_space totals and line attribution:
- Total 104230.49 KiB across benchmark calibration and measured iterations.
- cloneObjectForReadTxn: 79911.62 KiB, 76.67% (65.43% of allocation count).
- commitDecentralized local flat: 5875 KiB, 5.64%.
- addID closure: 551.12 KiB, 0.53%, growing writeIDs at line 357.
- Grouped property writes at line 475 account for 5287.5 KiB (1152 bytes/commit).
- lockIDs copy at line 398: 293.75 KiB (64 bytes/commit).
- slot-pointer slice at line 432: 293.75 KiB (64 bytes/commit).
- writeIDs growth contributes approximately 120 bytes/commit.
- The small bookkeeping maps at lines 352/399/473 allocate no observed heap bytes
  for this eight-object instance: source-level make calls were misleading.

Even eliminating every local commit scratch byte would save approximately
1400/22634 = 6.19% on the frozen primary workload, below the 10% meaningful gate.
Pooling cannot remove the dominant published clone allocations. A reusable
group structure would also introduce reset scans, map indirection, retained
memory and a lifetime contract after slot unlock. That complexity is not
justified by this primary-workload ceiling.

Ownership: slot pointers are borrowed until unlockSlots runs. Grouped write
slices only feed image builders; returned property values, code, and object
collections belong to immutable published images or transaction snapshots.
Those images can remain visible to concurrent readers/history and cannot be
returned with scratch. The current code deliberately detaches collections
before publication because coarse writers can mutate the current image later.
The 76.67% clone share is a separate ownership-sensitive target, not permission
to pool it. Whole transactions also retain idempotent Release/finalizer semantics.

A simpler future candidate is eliminating temporary grouping for trivially
single-object footprints, but this record establishes neither runtime benefit
nor a reason to implement it. A more consequential follow-up would profile
which transaction-private clones are redundant, with a separate ownership
proof and preregistration. Neither is smuggled into this scratch experiment.

Integration instructions: do NOT integrate candidate-specific red tests from
4b07992 or merge this whole branch; they intentionally fail compilation without
the rejected implementation. The independent benchmark from 04dfc97 is reusable
if desired. Promote only this negative record/evidence and optionally that
benchmark, selected by the parent. No production source was modified. Evaluator
and preregistered fields remain unchanged.
