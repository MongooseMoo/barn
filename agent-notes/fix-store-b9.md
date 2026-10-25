# fix/store-b9: valid()/ObjectExists record no read

## Goal
B9 from `agent-notes/db-store-review.md` (that note is on the main checkout, not
this branch): `StoreTxn.Valid` and `StoreTxn.ObjectExists`
(`db/store/store_txn_objects.go`) answered from `tx.object` without recording any
read, so a concurrent recycle of the checked object never fails the checker's
validation. The task was to show a non-serializable outcome with two snapshot
transactions; if shown, make the smallest fix.

## State
- Shown. `db/store/valid_read_test.go` `TestValidCheckConflictsWithConcurrentRecycle`
  (subtests `Valid`, `ObjectExists`): the checker sees x valid and sets y.flag = 1;
  the recycler reads y.flag = 0 and recycles x; recycler commits, then the checker
  commits too. On origin/master 3cb7e64c both subtests fail with
  `x recycled=true, y.flag=1; no serial order gives both`.
- Fix: when the object is live, both methods call `markObjectScalarRead`.
  `validateObjectReadsLocked` (`store_txn_validation.go`) records a Missing
  conflict (E_INVIND) once live is no longer a valid object, so the checker
  retries. Both subtests pass; `go test ./db/store -count=1` passes.
- Committed on `fix/store-b9`; not pushed, no PR.

## Decisions and why
- Mark only when the answer is true. Marking a recycled tombstone's scalar read
  would always fail validation (live is not valid), so `valid()` of a recycled
  object would never commit.
- Scalar read, not a new existence-read map: it reuses existing validation and
  renewal handling. Cost: `valid(x)` now also conflicts with a concurrent
  rename/chown/flag change of x (scalarVersion moves); that is a retry, not wrong
  results.
- No other read covers x on the real path: `builtinValid` -> `validForRead`
  (`builtins/store_reads.go:104`) -> `tx.Valid`, and a task such as
  `if (valid(x)) y.flag = 1; endif` reads only y and the verb's own object.
  `RecycleObject` (`store_txn_objects.go`) bumps x's versions and those of x's
  location and parents only.

## Not done / unverified
- No engine-level (two real MOO tasks) reproduction; the store test calls the same
  StoreTxn methods the builtins call.
- The false direction (valid(#N) = false, then a concurrent commit makes #N valid)
  is not covered and not shown either way.
- Elle: not run locally. `docs/elle-isolation.md:142-144` asks for a fault plus
  workload for each new validation protection; not added (outside smallest fix).

## Next step
Lead decides on push/PR and whether the Elle fault/workload is wanted.
