# callers() frame name under call_function (issue #1029)

## Goal

Inside a verb that a builtin calls (move's accept/exitfunc/enterfunc, and any
other builtin that returns `make_call_pack`) when that builtin is reached
through `call_function("name", ...)`, Barn must name the builtin frame in
`callers()`, `task_stack()` and tracebacks exactly as ToastStunt does.

Done when: each case has a conformance test passing on Toast unchanged and on
Barn; the checkpoint name is right in both directions for call_function move; a
generated lane compares it; this note holds each run's command and result and
what is not verified.

Not included: push, PR, any GitHub post, issue #1028's eval-continuation import.

## State

Done, committed locally (not pushed). Suite test
`src/moo_conformance/_tests/builtins/call_function_callback_frames.yaml`
committed on `test/callers-call-function-frame` in the suite worktree; Barn
fix, lane change, tests and this note on `fix/callers-call-function-frame`.
All 50 cases pass on Toast unchanged and on Barn; cross-engine move
checkpoint passes in both directions.

## Toast source readings

All from `git show origin/master:src/<file>` in `C:/Users/Q/src/toaststunt`
(origin/master head commit "Fix a stack overflow when freeing deeply nested
lists and maps").

- `execute.cc:2285-2290` (OP_BI_FUNC_CALL, BI_CALL): `RUN_ACTIV.bi_func_id =
  func_id` where `func_id` is the builtin the bytecode called. Under
  `call_function("move", ...)` that is call_function's id; `bi_func_pc =
  p.u.call.pc` (the inner builtin's pc, since `bf_call_function` passes the
  inner package through).
- `execute.cc:3468-3500` `bf_call_function`: calls the inner builtin with
  `call_bi_func(fnum, ...)`; on BI_CALL wraps the inner data in `cf_state
  {fnum, data}` and keeps the inner pc. On resume (`next != 1`) calls the inner
  builtin again with `s->fnum`, `s->data`.
- `execute.cc:310-313, 360-364` `unwind_stack` FIN_RETURN: takes the finished
  hook's caller `bi_func_id` (call_function), calls it, and on another BI_CALL
  stores the same `bi_func_id` again. So accept, exitfunc and enterfunc all see
  `call_function`.
- `execute.cc:452-517` `make_stack_list`: a builtin frame entry is
  `{#-1, name_func_by_num(stack[i].bi_func_id), #-1, #-1, player}` and, with
  line numbers, `stack[i].bi_func_pc` as the line (494-505).
- `execute.cc:3765-3775` `bf_callers` and `3779-3797` `bf_task_stack` both use
  `make_stack_list`; so do the handler error stack (556) and the
  handle_uncaught_error traceback list (593).
- `execute.cc:184-188` traceback text: `... called from built-in function
  %s()` with `name_func_by_num(activ_stack[t].bi_func_id)`, so under
  call_function it prints `call_function()`.
- `execute.cc:3503-3530` checkpoint: activation writes
  `name_func_by_num(a.bi_func_id)` (4024) then `write_bi_func_data`; for
  call_function the data is `bf_call_function data: fname = <inner>` followed
  by the inner builtin's data.
- Builtins returning `make_call_pack` with a non-zero pc (each calls a verb):
  `objects.cc` `bf_move` via `do_move` (141 accept, 178 exitfunc, 195
  enterfunc), `bf_create` (440 initialize), `bf_recreate` (525 initialize),
  `bf_recycle` (841 recycle verb, 868/873 exitfunc through `move_to_nothing`),
  `verbs.cc:628` `bf_eval`. `tail_call_pack` (pc 0: `bf_pass`, protected
  builtin -> `#0:bf_*` in `functions.cc:231`) leaves `bi_func_pc` 0, so no
  builtin frame appears; out of scope.

## Barn source readings

- `builtins/signatures.go` `builtinCallFunction`: looks up the inner builtin
  and calls `fn(ctx, args[1:])` directly; nothing records that the call site
  was call_function.
- The callee activation carries `BuiltinCaller`/`BuiltinPC`
  (`types/activation_frame.go:14-15`, VM `StackFrame` `vm/vm.go:245-246`), set
  by `vm.markBuiltinCaller` (`vm/op_verb.go:317`). `types.ExpandBuiltinFrames`
  (`types/activation_frame.go:30`) turns it into the `{#-1, name, #-1, #-1,
  player}` entry for callers(), task_stack() (`builtins/tasks.go:296, 452`)
  and tracebacks (`vm/traceback.go:25, 190`), like Toast's make_stack_list.
- Marks: `vm/move_lifecycle.go:60` `"move"` (every hook: accept, and exitfunc
  and enterfunc pushed again from `resumeMoveLifecycle`), `vm/op_verb.go:337`
  `pushInitialize(..., builtin)` with `"create"`/`"recreate"` from
  `builtins/objects.go:275`, `builtins/objects_hierarchy.go:706`,
  `vm/recycle_lifecycle.go:33, 80` `"recycle"`. Eval has no mark: eval frames
  are `IsEvalFrame` and callers() substitutes synthetic wrapper frames
  (`builtins/tasks.go:304-324`, `evalWrapperFrames`), and the printed
  traceback appends a fixed `... called from built-in function eval()`
  (`task/traceback.go:63-65`).
- Checkpoint: `ExportNativeSnapshot` (`vm/persistence.go:363-413`) refuses a
  move continuation whose `BuiltinCaller != "move"` and writes a plain `move`
  builtin record; import `withoutMoveContinuation` (`vm/persistence.go:313`)
  already unwraps `call_function` + `bf_call_function data: fname = move`, and
  `convertNativeSnapshot` (`vm/persistence.go:244`) then sets
  `BuiltinCaller = "move"`, losing the name. create/recreate/recycle
  continuations are refused for export (`ErrToastLosesObjectContinuation`).

## Runs

Load before runs: WSL loadavg 14.26 on 8 CPUs (CI jobs running).

1. Toast, first draft of
   `moo-conformance-tests-callers-call-function/src/moo_conformance/_tests/builtins/call_function_callback_frames.yaml`
   (callers / task_stack of a suspended task / printed traceback, kinds
   move accept, exitfunc, enterfunc, create and recreate initialize, recycle
   recycle and exitfunc, eval'd code; direct and through call_function):

   ```
   wsl -d Debian --exec bash -lc 'set -euo pipefail; cd /mnt/c/Users/Q/code/barn-callers-call-function; make conformance-toast CONFORMANCE_SUITE=/mnt/c/Users/Q/code/moo-conformance-tests-callers-call-function CONFORMANCE_ENV=/root/.cache/barn-make-conformance-linux K=call_function_callback_frames'
   ```

   Result: 2 failed, 39 passed (7 min under load). The two failures were
   `task_stack_of_task_suspended_in_recycle_recycle_{direct,call_function}`,
   timeouts caused by the test: its cleanup recycled the child `o` before
   deleting the suspending `recycle` verb on its parent, so the statement
   itself suspended forever. Cleanup now strips every object's verbs before
   recycling any. Every expectation written from the Toast source passed,
   including eval'd code's builtin frame with line 2 (`bi_func_pc`).

2. Toast, after the cleanup fix, the two failing rows' test families:
   same command with `K="call_function_callback_frames and recycle_recycle"`.
   Result: `6 passed, 14126 deselected in 198.37s` (admission, callers x2,
   task_stack x2, traceback x1). So all 41 cases pass on Toast unchanged.

`bin/moo` probe of the issue program extended to all three move hooks, with
the fix in place (Minimal.db): direct move gives `{#-1, "move", #-1, #-1, #3,
2}` and `... 4`; through call_function accept `{..., "call_function", ..., 2}`,
enterfunc `... 4`, exitfunc `... 3`.

`bin/moo` probe, eval inside a verb, before any eval change:
`d:run()` returning `{eval("return callers(1);"), call_function("eval",
"return callers(1);")}` gives, for both, `{{#-1, "eval", #-1, #-1, #3, 1},
{#2, "eval", #3, #2, #3, 1}}`: Barn's synthetic top-level eval wrapper
(`builtins/tasks.go:304` returns `syntheticEvalCallers` whenever the top frame
is an eval frame), line 1 instead of Toast's `bi_func_pc` 2, and without the
`d:run` frame below. That is wrong for direct eval() too, not only under
call_function.

3. Barn baseline, same file, binary built before any source change (its
   mtime precedes every edit):

   ```
   wsl -d Debian --exec bash -lc 'set -euo pipefail; cd /mnt/c/Users/Q/code/barn-callers-call-function; export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin; make conformance BIN=bin-linux CONFORMANCE_SUITE=/mnt/c/Users/Q/code/moo-conformance-tests-callers-call-function CONFORMANCE_ENV=/root/.cache/barn-make-conformance-linux K=call_function_callback_frames'
   ```

   Result: `26 failed, 15 passed`. Every call_function row failed with the
   inner builtin's name (e.g. `['move', 2, 1]`); both eval_code direct rows
   failed too (callers: line 1, not 2; task_stack: no builtin entry at all,
   `['', 4, 0]`). Every traceback row failed, and the output shows a second
   Barn divergence that is not call_function-specific: Barn prints the builtin
   entry as an activation, `... called from #-1:move (this == #-1), line 2`,
   where Toast prints `... called from built-in function move()`
   (`execute.cc:184-188`). Direct traceback rows were added after this run.

4. Toast, the families added after run 3 (`traceback_of_error_in_{kind}_direct`,
   `traceback_of_error_in_{kind}_call_function` (renamed from
   `..._through_call_function`, same content), `callers_below_{via}_eval_activation`):
   run 1's command with `K="call_function_callback_frames and (traceback or callers_below)"`.
   Result: `19 passed, 14123 deselected in 381.55s`. With runs 1-2 every one
   of the file's 50 cases has passed on Toast unchanged.

5. Barn with the fix, the whole file: run 3's command. Result:
   `51 passed, 14091 deselected in 338.56s` (50 cases plus admission).

6. Cross-engine, `tests/mongoose-conformance/server/suspended_call_function_move_accept_checkpoint.yaml`
   with a new final step: every hook appends `{verb, callers()[1][2]}` after
   its work (accept after it resumes), expected
   `[["accept", "call_function"], ["exitfunc", "call_function"], ["enterfunc", "call_function"]]`.
   Run as `scripts/cross-engine-checkpoint.sh`'s "Barn checkpoint tests"
   sessions narrowed with `-k "capability_admission or suspended_call_function_move_accept"`
   (scratch script `xengine.sh`, same uv flags as the script), three
   directions. Command:
   `MSYS_NO_PATHCONV=1 wsl -d Debian --exec bash -lc 'bash <scratchpad>/xengine.sh'`.
   Result: Barn writes/Toast resumes `2 passed`; Toast writes/Barn resumes
   `2 passed`; Barn writes/Barn resumes `2 passed`. So Toast resumed from
   Barn's checkpoint names the accept frame and the later hooks
   call_function, and Barn keeps the name from Toast's.

Go tests run (Windows): `go test ./vm ./builtins ./types ./task ./engine
./internal/fuzzseed -count=1` once after the first fix (failures were the two
pinned tests listed below, since updated); then focused:
`go test ./vm -run 'TestSuspendedStack|TestExportRefusesObjectContinuation$|FuzzSuspendedStackCheckpoint$'`
ok; `go test ./engine -run 'TestToastSuspendedCallFunctionMove|TestToastSuspendedMove|TestBarnMoveHookMiddleFrame'`
ok (5 tests); `go test ./engine -run 'TestEvalNestedVerbSeesEvalFrameInCallers$'`
ok. `gofmt -l` on changed files: none. `go vet` on changed packages: clean.
Mutation checks: reverting the import naming (`vm/persistence.go`, name
`"move"`) fails `TestToastSuspendedCallFunctionMoveKeepsFrameName` and the two
suspended-stack tests.

## Not verified

- staticcheck / golangci-lint, `-race`, a `-fuzz` run, whole `go test ./...`
  after the last edits, CI.
- That the new cross-engine step fails on the pre-fix binary (not rebuilt;
  by source the old export wrote a plain `move` record).
- Checkpoints of a task suspended in eval'd code: the eval() mark lives on
  the task activation only, so after a Barn or Toast checkpoint load the eval
  frame has no builtin entry (task_stack() lacks it; callers() from it falls
  back to the root synthetic wrapper), as before this change. Importing and
  exporting call_function eval is #1028's work.
- create/recreate/recycle callbacks under call_function across a checkpoint:
  still refused for Toast export (now naming `call_function`), kept in Barn
  form, where `BuiltinCaller` persists as written.
- The command-line root eval's synthetic wrapper frames (line 1 on its
  builtin marker) were not compared with Toast.

## Findings

- F1 (issue #1029): every callback a builtin pushes is named by the inner
  builtin; Toast names it by the builtin the bytecode called.
- F2: Barn's printed traceback prints a builtin entry as
  `#-1:<name> (this == #-1), line <pc>`; Toast prints `... called from
  built-in function <name>()`. Holds for direct move/create/recreate/recycle.
- F3: eval() inside a verb: callers() from the eval'd code returned Barn's
  synthetic top-level wrapper (line 1, no caller frames); task_stack() of a
  task suspended in eval'd code had no builtin entry; the eval activation's
  task frame had verb_loc #0 (zero value), Toast's is #-1
  (`execute.cc:3443-3449` setup_activ_for_eval).
- F4: checkpoint export refused a call_function move continuation (`move
  continuation has no Toast form`) only because of the name check; import
  renamed it `move`.

## Decisions and why

- Name at the call site, as Toast does: `executeCallBuiltin`
  (`vm/op_misc.go`) renames the callback a builtin just pushed to the name of
  the builtin the bytecode called (`Registry.NameByID`), and the return path
  (`vm/stack.go`) gives the next callback of a move or recycle continuation
  the finished callback's name (Toast's unwind_stack keeps `bi_func_id`).
  One place for every builtin instead of threading a name through each
  lifecycle.
- Checkpoint (move only, local to the move path): export accepts
  `BuiltinCaller == "call_function"` and writes Toast's call_function record
  wrapping move's; import takes the name from the Toast record's outer builtin.
  Export's create/recreate/recycle refusal now also covers a non-move
  callback named call_function (otherwise it reached the generic BuiltinPC
  refusal and the suspended-stack lane fataled).
- Eval: the eval() activation's task frame carries `BuiltinCaller "eval"`, pc
  2 (call_function's name under call_function), only on the task activation,
  not the VM frame, so `vm/persistence.go`'s eval conversion (#1028's area)
  is untouched. callers() walks an eval() activation like any other frame and
  keeps the synthetic wrapper only for the command-line root eval frame
  (`rootEvalFrame`, no builtin entry below it). Tracebacks read the eval mark
  from the task activation (`vm/traceback.go` snapshotActivationFrames).
- Tracebacks: `ExpandBuiltinFrames` marks inserted entries `IsBuiltinFrame`,
  and `FormatTraceback` prints them Toast's way; the fixed eval() line stays
  only for an eval frame with no builtin entry below.

## Existing Barn tests that pinned the old behavior

- `vm/suspended_stack_test.go` `TestExportRefusesObjectContinuation`: wanted
  the refusal for the call_function seeds to name the inner builtin
  (`create:` etc.); now names `call_function:`. Barn-only message text.
- `engine/eval_callers_test.go` `TestEvalNestedVerbSeesEvalFrameInCallers`:
  wants a verb called from eval()'d code (inside a `;` eval) to see the same
  3 entries as one called straight from the `;` code. Toast's make_stack_list
  lists the eval() activation and its builtin entry above the `;` frames;
  checked by the new `callers_below_{via}_eval_activation` test (Toast run 4).

## Surprises

- The vm checkpoint lane (`reloadCheckpoint`, `vm/checkpoint_resume_test.go`)
  reuses the original task's activations (`restored.Task = machine.Task`), so
  a name lost on import was invisible to a hook's own callers() after resume.
  The generator's move-exitfunc enterfunc now records its builtin entry name,
  which move() takes from the restored exitfunc VM frame; mutation check:
  reverting the import naming fails `TestSuspendedStackLayersMarkResult` and
  `TestSuspendedStackKindsReachCheckpoint` (`checkpointed ... "move"`).

## Next step

The lead publishes: suite PR first (the Barn branch needs the new suite test
to be green in CI only after both land; the suite test fails on Barn master).
The `engine/testdata/toast-suspended-call-function-move-accept.db` producer
yaml gained a step; the fixture was not regenerated (its Go test does not
read the new property).
