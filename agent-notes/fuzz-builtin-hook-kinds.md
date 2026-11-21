# fuzz-builtin-hook-kinds (issue #974)

## Goal

Extend the suspended-stack fuzz generator (`internal/fuzzseed/suspended_stack.go`)
with tasks suspended inside the hook verbs of `create()` (initialize),
`recreate()` (initialize), `recycle()` (the object's `recycle` verb and a
location's `exitfunc`), and `call_function(...)` variants for `move`, `create`,
`recreate` and `recycle`; make `FuzzSuspendedStackCheckpoint` run them and state
each kind's result.

## State

Generator, lane seeds/pins and the call_function("move") import are written and
committed locally on `fuzz/builtin-hook-kinds` (not pushed). The lane is red for
create-initialize, recreate-initialize, recycle-recycle and recycle-exitfunc:
that is the real choice below, sent to the lead and waiting on the answer.

Facts read (file:line on branch `fuzz/builtin-hook-kinds`):
- `builtins/signatures.go:159-194` builtinCallFunction calls the inner builtin
  function directly (`fn(ctx, args[1:])`), so under `call_function("move", ...)`
  Barn's hook frame carries the same `move` mark as a direct move()
  (`vm/move_lifecycle.go:60`). Export (`vm/persistence.go` ExportNativeSnapshot)
  then writes a plain `move` mark, which Toast resumes to the same result
  (Toast's bf_call_function at pc>1 only calls the inner builtin with the saved
  data: `git show origin/master:src/execute.cc` lines 3486-3491).
- Toast writes call_function's frame as builtin `call_function`, line
  `bf_call_function data: fname = move`, then move's data line
  (`execute.cc:3502-3510`). Barn's reader already parsed that into
  `Builtin{Name: call_function, Inner: {Name: move}}`
  (`db/format/reader_task.go:296-310`), but withoutMoveContinuation refused
  `Inner != nil`.
- `internal/fuzzseed/structured.go:183` choose is `byte % n`: adding kinds
  changes which kind a byte >= 15 selects.

## Decisions and why

- New kinds are appended after the 15 existing ones so explicit small-byte
  seeds keep their kinds.
- The call_function variant is variant 2 for the move and new hook kinds, the
  same value that means call_function("eval", ...) for eval/scatter/expression.
- The three raw-byte seeds in `vm/suspended_stack_test.go` (two "found by the
  lane", issue-902) were rewritten with kind indexes (and issue-902's one
  move-exitfunc variant 2 -> 0). Checked: origin/master's `moo-fuzz-source
  --generator suspended-stack` on the old bytes and the branch's on the new
  bytes print byte-identical programs (`cmp`: "a identical (8 lines)", "b
  identical (15 lines)", "c identical"); a direct move-accept/exitfunc/enterfunc
  seed and mixed seed 2 also render identically ("d/e/f/g identical").
- Toast fixture for call_function("move"): one producer (accept, pc 2) is
  enough to show the wrapper; the unwrap is pc-agnostic.

## Lane results

`go test ./vm -run 'TestSuspendedStackLayersMarkResult$|TestSuspendedStackKindsReachCheckpoint$|TestWorldHistoryLaneSkipsSecondsBudget$' -count=1 -v`:
- move-accept/exitfunc/enterfunc through call_function (seed 2 of each): PASS
  (`--- PASS: TestSuspendedStackLayersMarkResult/move-accept/2` etc.;
  `--- PASS: TestSuspendedStackKindsReachCheckpoint/move-*`). Exported in
  Toast's form (plain `move` mark) and re-read.
- create-initialize, recreate-initialize (direct and call_function): FAIL
  `ExportNativeSnapshot: frame 0 (#0:): operand stack of 1 does not match its
  call site`. Cause read: `vm/op_verb.go:325-327` pushInitialize pushes the
  created object on the caller's stack; Toast keeps it in the builtin's data
  (`bf_create data: oid = N`).
- recycle-recycle: FAIL `ExportNativeSnapshot: frame 1 (#4:recycle): builtin
  continuation has no Toast form` (`compiler/native_export.go:39-40`).
- recycle-exitfunc: FAIL `ExportNativeSnapshot: frame 1 (#5:exitfunc): builtin
  continuation has no Toast form`.
- Same four kinds through Barn's own frame form (temporary probe test using
  `checkpointSuspendedVMInBarnForm`, not committed): all 12 seeds PASS
  (`--- PASS: TestProbeHookKindsBarnForm (0.47s)`, 12 subtests PASS).
- TestWorldHistoryLaneSkipsSecondsBudget/issue-902: PASS.

## call_function("move") import

- Producer `tests/mongoose-conformance/server/suspended_call_function_move_accept_checkpoint.yaml`.
  Managed WSL Toast (binary SHA256 61dd1799..., HEAD 03af4fa2..., Test.db
  1a3f23eb...): `2 passed, 1 warning in 29.78s`. Fixture
  `engine/testdata/toast-suspended-call-function-move-accept.db` SHA256
  363d5639...; accept frame `47 2 45` / `call_function` /
  `bf_call_function data: fname = move` / `bf_move data: what = 11, where = 13, position = 0`.
- Fix: `vm/persistence.go` withoutMoveContinuation unwraps a call_function mark
  whose line names move.
- `TestToastSuspendedCallFunctionMoveImportFinishesMove`: unfixed
  `native frame 2: native builtin continuation requires conversion`; fixed PASS.
- Managed Barn (linux build of this branch, SHA256 1a1fb163...) on the new and
  the move-accept producers: `3 passed, 1 warning in 53.85s`; its logs hold 0
  "Toast could not load" records and 0 ERROR records.

## Surprises

- Pre-existing on origin/master: `TestSuspendedStackLayersMarkResult` ran all
  seeds in one test, and the fork seed's `t.Skipf` in checkpointSuspendedVM
  (`vm/checkpoint_resume_test.go:432`) skipped the whole test at the third
  kind, so the pinned results of every kind after `fork` were never checked
  (`--- SKIP: TestSuspendedStackLayersMarkResult (0.23s)`). Same in
  `TestSuspendedStackKindsReachCheckpoint`'s mixed seed loop: mixed seed 0
  skips, so seeds 1-4 never ran. Fixed by a subtest per seed; now mixed/2
  PASSes and mixed/0,1,3,4 SKIP (fork line numbering), and every non-fork
  pinned kind PASSes.
- Divergence, not fixed: Toast's callers() inside a hook reached through
  call_function("move", ...) names `call_function`; Barn names `move`. Toast
  output: the fixture's `stack` (recorded by accept before it suspended) is
  `{"call_function", "start", ""}` (`barn eval` on the fixture). Toast source:
  OP_BI_FUNC_CALL stores the called function's id (`execute.cc:2287`),
  unwind_stack keeps it for the next hook (`:362`), callers() prints it
  (`:494-499`). Barn: `./bin/moo` on a call_function("move") accept that
  stores callers() gave `{#-1, "move", ...}` for both move() and
  call_function("move"). Barn's export writes plain `move`, so a stock Toast
  resumed from Barn's file would also name `move`.
- Corpora outside the repo for FuzzSuspendedStackCheckpoint (if any) render
  different programs now that there are 19 kinds; none are in `vm/testdata/fuzz`.

## The choice: create, recreate, recycle

Evidence: Toast's bf_create/bf_recreate keep a `Var` but write only its first
8 bytes as `bf_create data: oid = N` and read back `sizeof(Objid)`; bf_recycle
the same (`objects.cc:427-428, 454, 548, 551-559, 833-834, 850, 961-985`, per
`C:/Users/Q/code/barn/agent-notes/native-continuation-refusals-inventory.md`).
Recorded run (cross-engine-checkpoint-coverage.md, U1/U2, not re-run here):
stock Toast resumed create()/recreate() returns an integer 18/18 and panics on
recycle() 12/12.

- A. Write Toast's form now (bf_create/bf_recycle data) and import it. Stock
  Toast then returns an integer or panics: ruled out by the brief.
- B. Keep these tasks in Barn's frame form, as the runtime already does
  (`engine/task_factory.go` "Toast could not load this checkpoint"), and make
  the lane treat the refusal like `compiler.ErrPathDependentClear`: a named
  export error for create/recreate/recycle continuations, so the lane checks
  Barn-form resume equivalence (12/12 PASS in the probe) and fails on any other
  refusal. Toast's form waits on a Toast fix of the two read functions.

## Next step

Wait for the lead's answer on A/B; then implement, rerun the lane, commit.
