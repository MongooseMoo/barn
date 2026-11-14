# A verb call can commit on a dispatch a concurrent chparent changed

## Goal
Make a transaction whose verb lookup walked the parent chain conflict when a
concurrent `chparent` (or other change to that chain) changes what the lookup
would find. Found by the first scheduled extended "Jepsen checkpoint" run
(43426360560, 2026-11-13), job "Checkpoint stage schedule and negative
controls": Elle rejected two ordinary (no control, no fault) histories with
G2-item.

## State
Reproduced by Go tests, fixed, committed locally on
`fix/verb-walk-misses-chparent`. Not pushed. The Jepsen schedule has not been
run against the fix; CI runs it on the PR.

- The cycles (authority family; key 40000003 is the value of `VC:probe()`,
  key 40000004 is `VA.log`; `scripts/jepsen_checkpoint.py`, `AUTHORITY_ROLES`
  and the `v_observe`/`v_mutate` verbs): in schedule-seed-4103 generation 5 and
  schedule-seed-4104 generation 6, a v-observe read the dispatch value without
  the mutator's append and appended to the log, while a kind-2 v-mutate
  (`set_verb_code(dest, "probe", code); chparent(this.vc, dest)`, VC holding no
  `probe` of its own) read the log without the observer's append.
- The fault, before the fix: `Store.ChangeParents` moved the dispatch shape
  clock and the relationship and property versions, but no verb version. A
  walk (`walkVerb`) leaves a verb-scan mark (the object's `verbVersion`) on
  each object it visits and a read mark on the verb found, and follows
  `obj.parents` with no further mark. The shape clock is checked only for a
  transaction that took a mark-less memo hit (`usedVerbMemo`). So a
  transaction holding scan marks, and no memo dependency, committed after a
  reparent of an object on its walk.
- Lookup kinds that held only scan marks, each shown red by a test in
  `db/store/verb_walk_chparent_test.go` before the fix:
  - a walk on a cold memo, from the reparented object and from a descendant
    (`TestVerbWalkConflictsWithConcurrentChparent`);
  - a memo hit taken by a transaction with staged writes
    (`TestVerbMemoHitWithStagedWritesConflictsWithConcurrentChparent`);
  - the marks `PrepareLiveMutation` materializes
    (`TestVerbMemoMaterializedMarksConflictWithConcurrentChparent`);
  - `FindParentVerb` (pass()), which read the definer's parents with no mark
    on the definer at all
    (`TestParentVerbLookupConflictsWithConcurrentChparent`).
- The fix:
  - `ChangeParents` stamps the reparented object's `verbVersion`
    (`stampObjectVerbs`), so a scan mark on an object covers its verb list and
    its parents. Only transactions whose walk visited that object conflict;
    `TestVerbWalkIgnoresChparentOffItsChain` pins that.
  - `FindParentVerb` marks a verb scan on the definer it starts from.
  - The chparent and chparents builtins share `changeParentsLive`
    (`builtins/objects_hierarchy.go`), which also calls `AdoptLiveVerbs` on
    the reparented object, so the task's own scan mark and private view take
    the version its own chparent stamped.
- Other writers, by source reading (no test written, none shown red):
  recycle and renumber stamp every version of the recycled object or the old
  id's tombstone, which a walk through it scanned; verb add, delete, rename,
  permission, argument and code changes stamp the object's `verbVersion` or
  the verb's version.
- Property walks: `TestPropertyWalkConflictsWithConcurrentChparent`
  (`db/store/prop_walk_chparent_test.go`) passed before the fix, from the
  reparented object and from a descendant. Kept as coverage; nothing changed
  for properties.

## Decisions and why
- No conformance test: the fault needs two concurrent tasks, which Toast
  never has. The proof is the Go tests in `db/store` plus the Jepsen schedule.
- Stamp `verbVersion` in `ChangeParents` rather than apply the shape clock to
  walked resolutions: the clock moves on every verb-code commit, chparent and
  recycle anywhere, and the scan-mark path for writers exists to avoid
  failing them on unrelated shape changes. The stamp conflicts only walks
  through the reparented object. `ChangeParents` already did the same for
  property walks (`stampObjectProperties` on the reparented object).
- Not a relationship read in the walk: a parent's `relationshipVersion` moves
  whenever a child is added to or removed from it, so every dispatch through
  a common ancestor would conflict with every create and recycle under it.
- `AdoptLiveVerbs` in the builtin rather than new store code: it is the call
  add_verb and the other verb-definition builtins already use to take the
  version of their own live change. Without it the chparenting task
  conflicts with itself; `TestVerbCallSiteSeesOwnChangesInOneTask` and
  `TestVerbCallFollowsMidTaskShapeChanges` in `engine` went red on the store
  change alone and are the guard for this half.
- `BenchmarkTxnFindParentVerb` added to `db/store/resolve_bench_test.go`: no
  benchmark timed pass(), the one lookup that gained a mark.

## Surprises
- pass() held no dependency on the parents of the verb's definer. A task
  resumed after a suspend calls pass() in a fresh transaction that never
  dispatched to that verb, so nothing else covered it.
- The store change alone made the task running chparent fail its own commit
  with E_INVARG: it holds a scan mark from its earlier call of the verb.

## Measured cost
Windows workstation, other sessions running (total CPU 13-68% across legs),
base and change alternated, six samples a side, medians. All store lookups
0 allocs/op on both sides.
- `BenchmarkTxnFindVerbAncestry` 191.0 -> 192.2 ns/op,
  `BenchmarkDirectFindCallableVerb` 352.6 -> 350.9, memo-hit variants within
  spread (code unchanged).
- `BenchmarkTxnFindParentVerb` 303.7 -> 306.8 ns/op, ranges overlap
  (298.6-310.1 against 302.3-315.1).
- `engine` `BenchmarkVerbDispatch/verb_call_5k` 2.139 -> 2.089 ms, 96
  allocs/op both; `protected_builtin_5k` 3.930 -> 3.993 ms, 10101 both.
- Not measured: the chparent builtin's added `AdoptLiveVerbs` (no benchmark
  times the builtin), and conflict rate or goodput on the real workload.

## Unverified
- A walk leaves no mark on a parent id that names no valid object, so an
  object later created, recreated or renumbered at that id is not seen by
  the walker's validation, and a staged numbered create does not move the
  dispatch clock. No producer found: recycle rewrites its children's parent
  lists, and startup repair was not read. Not tested, not changed.
- The multi-parent recycle and renumber cases were reasoned from source only.

## Next step
Open the PR and read the "Jepsen checkpoint" stage schedule job on it: the
ordinary histories must pass Elle and the `verb-memo`, `hot-memo-guard` and
`relationship-reads` controls must still produce their witnesses.
