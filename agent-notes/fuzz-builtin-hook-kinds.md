# fuzz-builtin-hook-kinds (issue #974)

## Goal

Extend the suspended-stack fuzz generator (`internal/fuzzseed/suspended_stack.go`)
with tasks suspended inside the hook verbs of `create()` (initialize),
`recreate()` (initialize), `recycle()` (the object's `recycle` verb and a
location's `exitfunc`), and `call_function(...)` variants for `move`, `create`,
`recreate` and `recycle`; make `FuzzSuspendedStackCheckpoint` run them and state
each kind's result.

## State

Done, committed locally on `fuzz/builtin-hook-kinds` (not pushed). The lead
chose B for create/recreate/recycle (see "The choice"). The lane is green.

Final runs on the branch:
- `go test ./vm -run 'TestSuspendedStackLayersMarkResult$|TestSuspendedStackKindsReachCheckpoint$|TestWorldHistoryLaneSkipsSecondsBudget$|TestExportRefusesObjectContinuation$' -count=1 -v`:
  all four `--- PASS`, `ok github.com/MongooseMoo/barn/vm 4.632s`. Skips are
  only fork/0, fork/1, the fork kind and mixed/0,1,3,4 (fork line numbering);
  69 PASS lines in the first two.
- `go test ./vm -run 'FuzzSuspendedStackCheckpoint$' -count=1 -v` (seed corpus
  only): `--- PASS: FuzzSuspendedStackCheckpoint (2.46s)`, 50 seeds PASS, 6 SKIP.
- `go test ./engine -run 'TestToastSuspendedMove|TestToastSuspendedCallFunctionMoveImportFinishesMove$|TestBarnMoveHookMiddleFrameCheckpointsAsToastContinuation$|TestExportRefusesPathDependentPushClear$' -count=1 -v`:
  5 PASS, `ok github.com/MongooseMoo/barn/engine 0.709s`.
- `go test ./internal/fuzzseed -count=1`: ok. `gofmt -l` on the touched .go
  files: no output. `go vet ./vm ./engine ./internal/fuzzseed`: clean.
- Not run: whole suites, `-race`, a `-fuzz` run, CI.

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

Lead's answer (2026-11-20): B, with the refusal named at the frame holding the
continuation, before the stack check, its text pinned per kind, and its doc
comment citing the Toast lines.

Built:
- `vm/persistence.go` `ErrToastLosesObjectContinuation`; ExportNativeSnapshot
  scans every frame for `BuiltinCaller` create/recreate/recycle before
  exporting any frame and returns `frame N (#O:verb): <builtin>: <error>`.
  Doc comment cites ToastStunt 03af4fa2 `src/objects.cc` lines read with
  `git show 03af4fa2...:src/objects.cc`: 427-428, 517-518 (Var kept), 454, 538
  (var_ref(*data)), 546-548 (bf_create_write prints an Objid), 552-554
  (bf_create_read allocates an Objid), 833-834, 850, 962-964, 970-972
  (recycle), 957 (panic_moo).
- `vm/checkpoint_resume_test.go` checkpointSuspendedVM accepts that error as it
  accepts ErrPathDependentClear (Barn-form reload, same resume required).
- `TestExportRefusesObjectContinuation` pins, for the direct and the
  call_function seed of each kind, exactly one refusal with text
  `frame 1 (#4:initialize): create: ...`, `frame 1 (#4:initialize): recreate: ...`,
  `frame 1 (#4:recycle): recycle: ...`, `frame 1 (#5:exitfunc): recycle: ...`,
  and the Barn-form resume equal to the uninterrupted run: 8/8 PASS.

## callers() under call_function("move", ...) (for a separate issue)

Not fixed on this branch (lead's decision).

Toast: inside a verb that move() called through call_function, callers() names
the builtin frame `call_function`; Barn names it `move`.

MOO source (run as a wizard; Barn via `./bin/moo <file>` built from this
branch, Minimal.db):

```
a = create(#-1); b = create(#-1); w = create(#-1);
add_verb(w, {player, "rxd", "accept"}, {"this", "none", "this"});
set_verb_code(w, "accept", {"#0.cs = callers(); return 1;"});
add_property(#0, "cs", 0, {player, "rw"});
move(a, w); d = #0.cs;
call_function("move", b, w); i = #0.cs;
return {d, i};
```

Barn output (both callers() lists name `move`):

```
=> {{{#-1, "move", #-1, #-1, #3}, {#-1, "", #3, #-1, #3}, {#-1, "eval", #-1, #-1, #3}, {#2, "eval", #3, #2, #3}}, {{#-1, "move", #-1, #-1, #3}, {#-1, "", #3, #-1, #3}, {#-1, "eval", #-1, #-1, #3}, {#2, "eval", #3, #2, #3}}}
```

Toast evidence: managed WSL Toast (03af4fa2, binary SHA256 61dd1799...) ran
`tests/mongoose-conformance/server/suspended_call_function_move_accept_checkpoint.yaml`
(2 passed). Its accept verb stores `c[2]` of each callers() entry in `stack`
before suspending; the checkpoint Toast wrote
(`engine/testdata/toast-suspended-call-function-move-accept.db`) holds
`stack = {"call_function", "start", ""}` (read with `barn eval --db` on the
fixture: `=> {#10, {"accept"}, {"call_function", "start", ""}, ...}`). The
direct-move producer gives `{"move", "start", ""}`
(`engine/testdata/toast-suspended-move.md`).

Toast source (`git show origin/master:src/execute.cc`): OP_BI_FUNC_CALL's
BI_CALL case stores the called builtin's id, call_function's, on the new
activation (2286-2289); unwind_stack keeps that id for the next hook (362-364);
callers() names a builtin frame by `name_func_by_num(stack[i].bi_func_id)`
(494-499). Barn: `builtins/signatures.go:159-194` calls the inner builtin
directly, and `vm/move_lifecycle.go:60` marks the frame `move`. Barn's export
writes a plain `move` mark for that frame, so stock Toast resumed from Barn's
checkpoint would also name it `move`. The same may apply to create, recreate,
recycle and eval under call_function (not tested).

## Next step

None on this branch: the lead publishes. Open the callers() issue from the
section above.
