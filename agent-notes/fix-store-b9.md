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
- Fix: when the object is live, both methods call `markExistenceRead`, which
  calls `markObjectScalarRead`. `validateObjectReadsLocked` (`store_txn_validation.go`) records a Missing
  conflict (E_INVIND) once live is no longer a valid object, so the checker
  retries. Both subtests pass; `go test ./db/store -count=1` passes.
- Elle fault `valid-reads` (Q's decision, 2026-10-25): `markExistenceRead` skips the
  read under it. Workload: the valid/recycle held pair in
  `engine/elle_workload_test.go` (world object #50, builtin `elle_valid`), after the
  torn-snapshot pair, plus one read of the `valid` row afterwards. Listed in
  `elleFaults`, `FAULTS` (`scripts/check_elle.py`) and the table in
  `docs/elle-isolation.md`.
- Evidence (WSL, seed 412, pinned jar under /root/.cache/barn-elle-412):
  fault history `/tmp/b9-valid-fault/list-append.edn` -> Elle `false`, exit 1,
  G2-item witness on the pair (checker read `valid` = [] yet the recycler did not
  see its append of 197 to `property`). Healthy Go run: list-append `true`,
  rw-register `true`, exit 0; the checker's committed attempt read `valid` = [50].
  The full `check_elle.py` matrix was not run locally; the PR's Elle CI job is the gate.
- Two commits on `fix/store-b9`; not pushed, no PR.

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
- The two-real-task reproduction exists only under the Elle fault (the
  valid/recycle pair); with the fix in place, the Go store test is the regression.
- The false direction (valid(#N) = false, then a concurrent commit makes #N valid)
  is not covered and not shown either way.
- The valid/recycle pair adds #50 to the Elle world for healthy runs too. Seeds
  395/396 were not run locally.

## Next step
Lead pushes and opens the PR; read its Elle CI result.
