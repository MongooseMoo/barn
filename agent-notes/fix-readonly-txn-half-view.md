# fix/readonly-txn-half-view

## Goal

Q, 2026-11-11: "Fix, PR and merge it." The Jepsen run on PR #763 (job
130636384333, generation 2, ops 1663/1664) showed a read-only task seeing half of
another task: the new parent from that task's `chparent` but not the marker the
same task appended afterwards. Diagnosis: `reports/diagnose-stale-inheritance-763.md`
in the main checkout.

## Cause

- `chparent` and the other coarse builtins mutate the live store mid-task
  (`builtins/objects_hierarchy.go` builtinChparent -> `Store.ChangeParents`,
  which bumps the clock and publishes at once), and a boundary
  (`StoreTxn.CommitAndRenewCarryingReads`) publishes the writes made so far. The
  rest of the attempt's writes wait for its final commit.
- The exclusive commit gate the attempt holds from that boundary stops commits,
  not snapshots. A read-only transaction whose snapshot falls in between saw the
  first half, and `StoreTxn.Commit` returned early for it (no gate, no
  validation); the runtime did not even call Commit, because `HasWrites` was
  false.
- Standalone hooks (`engine/call_verb.go` callVerbWithArgstr: do_login_command,
  CallVerb) never armed `BeforeIrreversibleEffect`, so their coarse builtins
  mutated the live store without the gate at all.

## State

Done in this branch:

- `internal/commitgate/gate.go`: `ExclusiveGeneration`, odd while an exclusive
  grant is active.
- `db/store/store_core.go`: `exclusiveGrantClock`, recorded at each exclusive
  grant (`noteExclusiveGrant`, also for the checkpoint hold in
  `store_snapshot.go`).
- `db/store/store_txn.go`: BeginSnapshot samples generation, clock, grant clock,
  generation. A snapshot taken during a hold whose holder already published is
  marked (`overlapsHold`, `holdClock`). `mustValidateReads` (used by `HasWrites`,
  Commit's early return and `commitTaskRowsOnly`) is true when the transaction
  read a version newer than `holdClock`; Commit then waits on the shared gate for
  the holder's commit and validates every read. A loss is the ordinary conflict
  the runtime re-runs.
- `db/store/store_hot_read.go`: hot-read renewal carries the mark.
- `engine/task_runtime.go`: corrected the comment above BeforeIrreversibleEffect;
  the boundary body moved to `Runtime.renewUnderGate`, shared with hooks.
- `engine/call_verb.go`: vmOwnershipNone hooks take the exclusive gate at their
  first irreversible builtin and hold it to their commit.
- Tests: `db/store/readonly_half_view_test.go`.
  `TestReadOnlyTxnSeesHalfOfLiveMutatingTxn` failed before the fix
  (`reader: mf={} parent=#1 z=-1 commit=E_NONE`) and passes after it
  (`commit=E_INVARG`). `TestReadOnlyTxnBesideExclusiveHoldStaysGateFree` pins
  the gate-free path.

## Decisions and why

- Followed the existing external-read precedent (`store_external_reads.go`): a
  read-only transaction that may have seen an exclusive holder's partial work
  is validated and committed like a writer, and `HasWrites` reports it.
- The mark needs a read of a version published under the hold, not just a
  snapshot taken during one. `agent-notes/perf-gate-hold-builtin.md` measured
  Mongoose at 1p with 161 exclusive holds in 10 s, 1.55 s held in total and the
  longest 672 ms. Under the broad rule, unrelated read-only tasks would have
  waited out those holds. While a hold is active no ordinary commit can move
  the clock, so any version newer than the grant clock is the holder's (or
  comes from a gate-free direct write, which only causes an extra validation).
- The gate is sampled before the clock and again after the grant clock. With
  only one sample after the clock, a holder that published before the snapshot
  and then committed and released before the sample would be missed.
- Hooks: in a standalone hook, an Irreversible builtin now takes the exclusive
  gate, as it already does in a task. Example: Mongoose `#0:do_login_command`
  line 18 calls `server_log` for a redlisted host (mongoose_fresh.db).

## Surprises

- The runtime calls Commit only when `HasWrites` is true
  (`engine/task_runtime.go` final commit), so changing Commit alone would never
  have run for a read-only task.

## Not closed (open)

- Hooks nested in another execution (vmOwnershipExecution,
  `engine/runtime.go` VerbCaller) and sweep hooks (vmOwnershipSweep,
  `engine/runtime.go` RunGC path, `engine/waif_lifecycle.go`) still mutate the
  live store without the gate. Their owner may already hold it, so taking it
  again would deadlock. Not verified whether a read-only snapshot can interleave
  with them; sweeps run under VMStartMu, which stops new VMs from starting.
- Gate-free direct writes (`DirectTxn`, for example the server's
  `SetVerbCode` in `server/input_processor.go`) are single operations committed
  at once. They have no staged second half.

## Next step

PR against master, auto-merge, watch CI. Rebased on master 2026-11-11;
`TestRenewalAccountsForEveryTxnField` needed the two new fields listed (both
carried). db/store, engine, and db/store -race pass locally. After merge:
update #763 from master and rerun its Jepsen jobs.
