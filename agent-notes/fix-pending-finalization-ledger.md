# fix/pending-finalization-ledger

## Goal

Stop `vm.PendingFinalizations` growing without bound in a task that keeps
suspending (on the live Mongoose DB Barn went from 0.5 GB to 12 GB in 40 s,
and every resume clones the ledger in `copyRetryVM`). Approved plan: "Leak fix
plan" in `agent-notes/mongoose-faceoff-2026-10-28.md` (main checkout).

## State

Implemented and committed locally on this branch; not pushed, no PR.

- `vm/anonymous_gc.go`: `PruneSuspendedPendingFinalizations` replaces the
  ledger with a new slice holding only anonymous objects that still exist,
  and nils both identity maps (rebuilt lazily by
  `appendPendingFinalizationRoots`).
- `engine/task_runtime.go` `handOffSuspended`: calls it when shutdown was not
  requested, after the slice commit and before `SetVM`.
- `engine/waif_lifecycle.go` `flushDeferredGCBarrier`: on mid-sweep shutdown,
  queued WAIFs with `Task == nil` publish their own WAIF (canonicalized)
  instead of being dropped; the entry whose `:recycle` just ran is skipped.
- Tests: `TestSuspendPrunesPendingFinalizationLedger` (new file
  `engine/suspend_finalization_ledger_test.go`; old code: 98 entries, 49
  WAIFs after 50 suspends) and
  `TestDeferredWaifRecycleShutdownHandsOffUntaskedRemainder` (old behavior:
  empty handoff). Shared WAIF-class fixture extracted to
  `newWaifRecycleStore` in `engine/shutdown_finalization_test.go`.
- Ran: `go test ./engine ./vm -run 'Pending|Shutdown|Finaliz|Waif|Suspend'`,
  both ok. Whole suite and CI gates not run (PR CI does that).

## Decisions and why

- Shutdown check for the prune is read after `deferPendingWaifs`, via
  `Runtime.ShutdownRequested()` (same `lifecycle.Mu`). `ShutdownRequested` is
  only ever set to true (`engine/runtime.go:417`), so false afterwards means
  `deferPendingWaifs` also took its non-shutdown branch.
- Prune placed after the slice commit: `Store.Get` reads committed objects,
  so pruning before the commit would drop anonymous objects this slice
  created but had not yet published.
- Anon existence uses the exported `Store.Get` (`ok && view.Anonymous`) rather
  than a new store helper.
- Change 2 skips the current batch entry for `Task == nil`: its `:recycle`
  already ran, and `CanonicalizePendingFinalizationValues` does not filter
  recycled WAIFs, so publishing it would recycle it again after reload.

## Surprises

- `Task == nil` entries also come from `AdoptPendingFinalizations` (loaded
  roots). Before this change a mid-sweep shutdown dropped those too; now they
  are handed off as well. The change-2 test uses adopted WAIFs for that reason.

## Next step

Push, open the PR, read CI. Then the capped Mongoose world rerun must show a
flat heap, then the full faceoff matrix.
