# Direct-reference collection experiment

## Goal

Find whether one directly owned container traversal and one collection per
released value simplify the store/VM without changing finalization behavior.

## State

Q said make it so. Isolated branch experiment/direct-roots is based on the
latest fetched origin/master; worktree C:/Users/Q/code/barn-direct-roots.
Worker direct_roots_implementation owns the bounded source edits; parent owns
notes, measurements and publication. Baseline worktree is barn-direct-roots-base.
Existing benchmarks are being measured as experiment evidence; correctness is
left to PR CI. No merge is requested for this experiment yet.

## Smallest implementation plan

Add a types-owned visitor for direct anonymous and WAIF references, approximately
25-35 lines. Descend lists and maps, map keys before values, preserve the
MayHoldFinalizable fast paths, and stop at reference leaves: never walk object
properties or retained images. It reports occurrences; callers own deduplication.

Replace recursive descent in store/collectTopLevelWaifsInto and both VM direct
collectors with this visitor. Keep store WAIF filtering and WaifSet ownership.
Keep VM scratch maps/slices, lazy membership indexes, logical WAIF equality,
first-seen ordering and independent pending-list drains.

Collect released values once for both pending lists. Production pairs occur in
vm.go popFrame and releaseLocal, plus anonymous_gc.go frame collection. The
value-only pending-finalization collector has no unpaired production callers.
Use the existing scratch results to record WAIF candidates and pending roots;
retain a small leaf recorder for the existing PendingWaifs index. Remove old
collectors when unused, rather than retaining forwarding compatibility helpers.

Expected replacement scale: about 45-70 added production lines against roughly
80-110 removed/rewritten lines. These are planning estimates, not measured savings.
If callback/adapter cost defeats clarity or performance, keep only the smaller
successful portion or reject the experiment. No universal graph abstraction,
new storage representation, cache removal, covering-root reducer or lifecycle
change is part of this experiment.

## Evidence and constraints

- types/waif.go:80 compares logical identities; types/waifset.go uses those same
  identities and preserves insertion order.
- vm/anonymous_gc.go:32 directly collects refs without entering WAIF properties.
- vm/waif_gc.go:7 owns a separate lazy PendingWaifs membership index.
- db/store/store_reachability.go:239 collects top-level WAIFs for the existing cache.
- vm/vm.go:88 and :560 pair both collections; anonymous_gc.go:249 does so for frames.
- Preserve anonymous-map iteration behavior rather than introducing sorting.
- Preserve no-finalizable-value fast paths and scratch pooling thresholds.

## Validation

Existing PR CI owns correctness; no local CI reruns to gate pushing. Existing
identity/drain, scratch ownership, selection-boundary and shutdown-root tests
cover relevant contracts. Adapt direct-helper references only if removed; no
new speculative tests or guards.

Existing benchmarks read: BenchmarkPendingWaifLocals,
BenchmarkPendingFinalizationDiagnostic, BenchmarkPlainFrameFinalization. Compare
baseline and candidate interleaved with identical settings, recording ns/op,
B/op and allocs/op. These are targeted experiment measurements, not full-workload
parity evidence. Frame benchmark rows mostly use scalar locals, so do not claim
they establish a nested-frame speedup. No new benchmark harness is proposed.

## Next step

Implementation complete: five production files, 46 added and 78 removed lines,
including the 26-line new visitor (net minus 32). No test, workflow or benchmark
changes. Store calls the visitor directly; the forwarding wrapper is gone.
Frame, stack and local release collect once; leaf recording populates both
independently indexed pending lists. Parent inspected the complete source diff.

Three paired benchmark runs completed using unchanged source throughout each
compilation/run, in order base/candidate/base/candidate/candidate/base. Raw rows
are in direct-roots-benchmarks.txt. Production WAIF-local timings were
2.46/2.52/3.06 ms base and 2.13/2.07/2.61 ms candidate; this is directional,
not a statistically established or full-workload speedup. Plain frame kept zero
allocations, with timings 88.63/89.23/91.93 ns base and 87.91/104.3/88.32 ns
candidate. Frame diagnostic rows showed no consistent slowdown. Value helper
rows are not equivalent WAIF work: candidate now records both pending lists;
baseline recorded only PendingFinalizations. Small anonymous Value rows slowed
several ns; they do not measure the complete releaseLocal path. Preserve these
limitations rather than presenting every benchmark as a win.

Next: publish the experiment PR, use its existing CI, review the production
change and report keep/reject. No merge is authorized yet.
