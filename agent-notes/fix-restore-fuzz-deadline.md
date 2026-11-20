# fix/restore-fuzz-deadline

## Goal

Make FuzzRestoreVMSnapshotDoesNotPanic (vm/persistence_verify_test.go) bound
execution time the way production does (an execution deadline), so saved inputs
a67f1c049e52d97e (unbounded vm.Stack growth, OOM) and 021b3e3abaa49095 (CPU
hang, issue #695) stop. Harness-only change.

## State

- Harness now sets the execution deadline from the task's budget
  (`ExecutionBudget` + `SetExecutionDeadline`, as engine/task_runtime.go:448-455
  does). Committed with input a67f1c049e52d97e in
  vm/testdata/fuzz/FuzzRestoreVMSnapshotDoesNotPanic/.
- a67f1c049e52d97e: without the fix, `panic: test timed out after 30s` with live
  heap 1378 MB under GOMEMLIMIT=1GiB; with it, PASS in 1.00 s, peak live heap
  212 MB (gctrace `gc 27 @0.945s ... 417->417->212 MB`).
- 021b3e3abaa49095 is NOT fixed by the deadline and is not checked in (it would
  hang `go test ./vm`). Issue #695 stays open.
- `timeout 120 go test ./vm -run FuzzRestoreVMSnapshotDoesNotPanic -count=1`
  (GOMEMLIMIT=1GiB): before, with both inputs, timeout (exit 124,
  `FAIL github.com/MongooseMoo/barn/vm 118.322s`); after, with a67f1c only,
  `ok github.com/MongooseMoo/barn/vm 2.321s`. `go vet ./vm` clean,
  `gofmt -l vm` empty.

## Decisions and why

- Deadline from `ExecutionBudget()` rather than a literal second: same
  computation as production, and it follows the `NewState(..., 1)` seconds
  limit.
- 021b3e left out of testdata: the only fixes are in production code
  (bytecode verifier or vm), which this task excluded.

## Surprises

- 021b3e3abaa49095 runs a zero-tick cycle at run time, so the deadline is never
  consulted. Disassembly: ip 14 `UNWIND_JUMP target=33 depth=0xb3ac`; ip 21-25
  `IMM 20, POP, IMM 35, IMM 35, FOR_RANGE_NEXT var 33 end 175 offset 16` -> ip 14.
  At run time unwindJump (vm/control.go:423-424) returns an internal error
  (depth 45996 > 0 handlers); with VerbDebug false, vm/vm.go:975-988 pushes
  E_EXEC and continues at ip 21 (operands already consumed). Every instruction
  in 14..25 is tick-free, so chargeTicks never reaches tickCheckpoint. The
  verifier (bytecode/verify.go:478, branchTarget at :507) models UNWIND_JUMP's
  only successor as the target (33), so it never sees the fall-through cycle.
  Probe with the deadline set: ticks=2, ip=21, secondsLeft=0 at 1-4 s while SP
  grew 18657 -> 35913. Memory also grows, slowly, because each UNWIND_JUMP
  copies the whole stack (O(n^2) CPU).
- The diagnosis note called 021b3e "live heap flat"; the stack does grow, but
  the per-iteration stack copy keeps the growth rate low.

## Next step

- Lead decides the production fix for 021b3e / #695. Candidates: the verifier
  rejects an UNWIND_JUMP whose depth exceeds the handler depth it can prove at
  that ip, or the verifier models the error fall-through edge (then the
  zero-tick cycle check rejects it). Then add 021b3e3abaa49095 to testdata
  (sha256 021b3e3abaa4909511092a3ec84a58337ed806cb4cdbc957e009d54515c5c008,
  record in artifact fuzz-corpus-FuzzRestoreVMSnapshotDoesNotPanic of run
  42750434626).
