# Regular and anonymous object lifecycles

## Change and scope

Creation can use an existing immutable property base when its names and inherited
owner/permission metadata match. The common ancestry walker still visits every
ancestor and the transaction records its property scans. Added names use normal
union construction; changed metadata uses the existing slot construction.
The live and transaction readers share that walker.

Recycle visits property values directly, retaining shape and individual value
dependencies without materializing a temporary value slice. Lifecycle hooks,
identity allocation, coarse boundaries, COW publication and locking are unchanged.

The baseline is the final benchmark-only commit immediately before the production
change on this branch. Its detached worktree contains the same benchmark sources.
The production change and its semantic regressions follow those baseline commits.

## Measurement

Windows/amd64, AMD Ryzen 9 5950X, Go 1.27.1, GOMAXPROCS=4. This is a shared,
unpinned host. Confidence intervals and all unfavorable observations are retained.
No production throughput or latency claim follows from these synthetic fixtures.

Each store operation handles 32 objects in a fresh store. Creation commits each
object; destruction starts with committed objects; the pair creates/recycles each
object. There are empty and 150-slot classes and a two-parent union. Snapshot
registrations are released explicitly, with a mechanical zero-active-readers
assertion after every cohort. Isolated destruction finishes setup GC before its
timer starts. Time and allocation counters exclude setup and verification.

The VM fixture calls RunServerVerbTask through the production VM, transactions,
and recycle continuations. Batches contain one or 32 objects. Its loaded class
slots are shared, and each object's inherited recycle hook increments an observed
counter. Explicit destruction and drop-reference/run_gc collection verify the
expected counts. Runtime teardown is excluded from the timer. First task
compilation is included; these are bounded cold-runtime samples.

Timing comparisons use ten observations per arm, in two batches of five with
base/candidate/candidate/base order. The final store campaign uses 64 iterations
per observation; VM uses eight. The longer empty-store check uses 512 iterations.
The retained-heap comparison uses 1,024 live created objects, a full GC before
and after creation, and KeepAlive through the second measurement. Its timing is
not a throughput metric; compare only retained-B/object.

Commands, from the respective worktree, with GOMAXPROCS=4:

```text
go test ./db/store -run '^$' -bench '^BenchmarkObjectLifecycle' -benchtime=64x -count=5 -benchmem
go test ./engine -run '^$' -bench '^BenchmarkObjectLifecycle' -benchtime=8x -count=5 -benchmem
go test ./db/store -run '^$' -bench '^BenchmarkObjectLifecycle/anon=.*/slots=0' -benchtime=512x -count=5 -benchmem
go test ./db/store -run '^$' -bench '^BenchmarkRetainedCreatedObjects$' -benchtime=1x -count=10 -benchmem
benchstat final-base-store.txt final-candidate-store.txt
benchstat base-vm.txt candidate-vm.txt
benchstat retained-base.txt retained-candidate.txt
```

## VM results

These rows measure 32 objects inheriting 150 slots, ten observations per arm.
The full tables, including empty and single-object cases, are in
[benchstat-vm.txt](benchstat-vm.txt).

| Operation | Object type | Base time | Candidate time | Time change | Allocated bytes change |
| --- | --- | --- | --- | --- | --- |
| Create | Regular | 2.845 ms | 1.022 ms | -64.08% | -69.61% |
| Create | Anonymous | 2.933 ms | 1.512 ms | -48.46% | -56.70% |
| Recycle | Regular | 1.451 ms | 1.265 ms | -12.81% | -18.08% |
| Recycle | Anonymous | 1.474 ms | 1.313 ms | No significant difference | -18.96% |
| Pair | Regular | 4.084 ms | 2.478 ms | -39.33% | -48.85% |
| Pair | Anonymous | 4.292 ms | 2.359 ms | -45.05% | -50.07% |

Automatic collection of 32 anonymous objects has no significant timing change
for either class. Its hook counts remain correct. Do not describe explicit
recycle improvements as an automatic-GC speedup.

## Store and retained heap results

[benchstat-final-store.txt](benchstat-final-store.txt) is the final store table.
Single-parent, 150-slot creation time decreases 83.02% for regular objects and
87.37% for anonymous objects. Pair time decreases 74.32% and 82.10%. Two-parent
creation time decreases 31.38% and 35.56%; its pairs decrease 25.70% and 29.12%.
The store destruction implementation itself is unchanged. Most isolated recycle
rows have no significant change; the anonymous two-parent row is faster in this
campaign. Make no general store-only recycle speedup claim.

[benchstat-retained.txt](benchstat-retained.txt) reports about 555 retained bytes
per regular object and 631 per anonymous object in both arms, with no significant
difference. The improvement removes transient construction work while preserving
the shared retained representation.

Short empty-store time changes vary in sign across campaigns, including apparent
slowdowns in the final short campaign. The longer empty-store comparison in
[benchstat-empty-store.txt](benchstat-empty-store.txt) does not reproduce those
slowdowns. Allocations are stable. These small-case timing deltas are inconclusive;
retain them and do not use them as evidence of either a speedup or a settled
regression. The wide-class allocation reductions recur in store and VM fixtures.

## Profiles and rejected evidence

The CPU/heap profiles and text reports named `base-store.*.pprof`,
`candidate-store.*.pprof` and `profile-*.txt` cover separate diagnostic runs of
the single-parent 150-slot pair benchmark. They include fixture setup, unlike
benchmark timing counters. Allocation sampling attributes about 617 MB to
propTable.put in the baseline and 39 MB in the candidate; total sampled allocation
falls from about 1,259 MB to 634 MB. Remaining table copies are visible. In-use
profile snapshots occur after the benchmark; use the dedicated live-object
retention comparison for retained bytes per object.

The first `base-store-1.txt`, `candidate-store-1.txt`, `candidate-store-2.txt`
and `base-store-2.txt` belong to an exploratory campaign whose snapshot-release
fixture changed while it ran. Reject them as a comparison. They are retained
alongside the replacement `released-*` and final store samples. The first VM
samples used the corrected shared-slot fixture throughout and remain valid.

`released-*` and `recycle-*` excluded creation time but allowed creation's GC work
to overlap destruction. An apparent two-parent recycle slowdown repeated in that
fixture. The final fixture completes setup GC before destruction, after which
the slowdown does not reproduce. That observation supports setup interference;
it is not proof that every noisy timing difference comes from GC. All original
samples remain available. The focused empty-store follow-up and final fixture
correction were prompted by those unfavorable rows, not silently substituted.

## Correctness verification

Focused new regressions cover inherited metadata, changed owners and c flags,
added definitions, shared sibling isolation, multiple-parent shape union,
ancestor-change conflicts and property-visitation value dependencies. The
existing staged-definition/coarse-create test was migrated to direct visitation.
Full build, lint, race, managed conformance and pinned Elle evidence are required
from the PR's current-head workflows before merge. No local full-suite CI was run.
