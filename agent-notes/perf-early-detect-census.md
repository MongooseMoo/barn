# perf/early-detect-census

## Goal

Count, in the conflict census (CONFLICT_CENSUS tracking only), how many MOO
ticks an early abort at a superseded hot read would save. Count-only: execution
never changes. Design approved by Q, briefed by the team lead.

## State

Built and committed locally on branch perf/early-detect-census; not pushed, no
PR (the lead pushes).

- Store: `db/store/store_early_detect.go` (EarlyDetection counters, the check
  `StoreTxn.NoteSupersededHotRead`, loss and release accounting). Hooks in
  `store_conflict_diag.go` (txnConflictDiag fields, ConflictDiag
  DetectedLosses/SavableTicks, carryConflictDiag carries hotReads),
  `store_conflict_census.go` (ConflictCensus.EarlyDetection, lostValidation,
  snapshot, reset), `store_hot_read.go` (findPropertyRenewing records hot
  reads; renewAtCurrentClock carries them), `store_history_gc.go` (release
  counts DetectedNotLost).
- Engine: the preempt closure in `engine/task_runtime.go` calls the check after
  the yield, also when the scope is borrowed; skipped while the gate is held.
- Harness: `engine/mongoose_real_bench_test.go` prints a `diag early_detection`
  line after `attempts_per_slice`, and detected_losses/savable_ticks per key.
- Docs: CONFLICT_CENSUS paragraph in CLAUDE.md, docs/operator-http.md.
- `/debug/conflict-census` serves EarlyDetection through the embedded
  ConflictCensus (internal/app/census.go unchanged).
- Targeted results: `go test ./db/store ./engine -count=1` ok; with `-race` ok.

## Tests and the mutation each catches

All mutations were applied, seen failing, and undone.

- TestEarlyDetectionCountsSavableTicksOfALoss: savable = lossTicks (no
  subtraction of detectTicks) fails (100 vs 70).
- TestEarlyDetectionDoesNotDetect/unchanged_clock: dropping the clock fast exit
  fails (Checks 2, want 1).
- TestEarlyDetectionDoesNotDetect/read_only: dropping the staged-writes fast exit
  fails (detects on a read-only txn).
- TestEarlyDetectionDoesNotDetect/unrelated_commit and /created_object: no
  mutation found that fails them (see Surprises for created_object).
- TestEarlyDetectionRenewalCarriesHotReads/commit_and_renew_carrying_reads:
  removing the hotReads carry in carryConflictDiag fails.
- .../renewal_at_stale_hot_read: removing the hotReads carry in
  renewAtCurrentClock fails.
- .../detection_not_carried: carrying `detected` in carryConflictDiag fails.
- TestEarlyDetectionOffRecordsNothing: calling noteEarlyLossLocked without the
  diag guard panics.
- TestEarlyDetectionDetectedNotLost: removing the release hook fails.
- TestEarlyDetectionUndetectedTrackedAndUntracked: removing the tracked count
  fails.
- engine TestEarlyDetectionCountsSavableTicksInATask: making the preempt call
  unreachable fails (Detected 0). Observed: wasted 3006, savable 1982 (detection
  at the 1024-tick checkpoint), Checks 2.

## Decisions and why

- Census counters are atomics in `earlyDetection`, not under census.mu: the
  check runs every 1024 ticks on every tracking task and must not serialize on
  the census mutex. Losses still update the per-key rows under census.mu.
- Fast-exit order: nil diag, detected, no hot reads, clock unchanged, nothing
  staged. lastCheckClock is recorded only when the slow path runs, so a
  transaction that first stages a write at an unchanged clock still gets checked.
- Recording a new hot read resets lastCheckClock: a hot slot read at an old
  snapshot can already be superseded (renewal declined or refused) without the
  clock moving since the last check.
- renewAtCurrentClock carries diag by hand, not through carryConflictDiag, so
  hotReads is carried there too (cloned). findPropertyRenewing notes the
  lookup's new reads on the transaction it returns, so after a renewal the
  re-read slots are recorded on the renewed transaction.
- DetectedNotLost is counted in `release()`, which both Release and the
  finalizer backstop call exactly once (released.Swap).

## Surprises

- The "skip objects the txn created" clause in the check cannot be exercised:
  object ids come from the store-wide allocateID
  (`db/store/store_txn_objects.go:189`), so no published object shares an
  uncommitted created id, and the live lookup already finds no valid object.
  Kept as briefed; its test passes with or without the clause.
- CheckNanos read 0 in the Windows test runs (2 checks); the monotonic clock
  resolution there is coarse. Not verified on Linux.

## Next step

Lead pushes and opens the PR; then run TestMongooseRealWorkload at 16 players
with BARN_MONGOOSE_STALE_READS and tracking on to read the savable share.
