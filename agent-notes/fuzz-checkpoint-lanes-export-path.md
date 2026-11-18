# Fuzz checkpoint lanes through the production export path

## Goal

Make `checkpointSuspendedVM` (`vm/checkpoint_resume_test.go`) write the
checkpoint the way production does: the suspended task's snapshot goes through
`vm.ExportNativeSnapshot` before it is written, so the reload goes through the
Toast-frame import (`convertNativeSnapshot`, `vm/persistence.go`). Then measure
what `FuzzCheckpointResumeEquivalence` and `FuzzWorldCheckpointEffects` find,
and add one plain test for the PR 886 shape (task suspended inside `eval()`).

## State

- Done and committed locally on `fuzz/checkpoint-lanes-export-path`; not
  pushed. Both lanes now write the suspended task through
  `vm.ExportNativeSnapshot` and reload it through the Toast-frame import. An
  export error fails the case.
- Deterministic tests are green. Fuzzing the resume lane will fail when it
  reaches either of the two export-error classes under Findings; neither is
  fixed here.

### What production does (read, not run)

- `engine/task_factory.go:472-498` `TaskSnapshots`: `snapshot := t.PersistenceSnapshot()`;
  when `snapshot.VM != nil` it calls
  `vm.ExportNativeSnapshot(snapshot.VM, snapshot.CallStack, s.registry.Compiler())`
  and on error keeps the Barn frame form and logs a warning (`:489`).
- `engine/task.go:65-73` `Task.PersistenceSnapshot`: `task.State.PersistenceSnapshotLocked()`
  plus `machine.PersistenceVMSnapshot()`.
- `task/snapshot.go:313-321`: `CallStack` is `cloneActivationFrames(t.CallStack)`,
  the task's activation stack. The engine pushes the root activation
  (`engine/call_verb.go:401`, `engine/eval.go:180`, `engine/task_factory.go:296`,
  `engine/task_load.go:87,207`, `engine/task_runtime.go:791`); the VM pushes one
  per verb call (`vm/op_verb.go:308,557`) and per eval frame (`vm/registry.go:146`).
- `vm/persistence.go:267-297` `ExportNativeSnapshot` uses `callStack` only for
  `frame.Native.Programmer = callStack[index].Programmer` (`:282-284`), by frame
  index, so `CallStack[i]` must be VM frame `i`'s activation.
- `db/format/writer_vm.go:45-60` also pairs `snapshot.CallStack[i]` with frame
  `i`; with no entry it uses `snapshot.Programmer`.
- Reload: `db/format/reader_vm.go:246-262` reads a frame with no Barn marker as
  `frame.Native`; `vm/persistence.go:105` `RestoreVMSnapshot` runs
  `convertNativeSnapshot` (`:195`) on it.
- `compiler/native_export.go:28-38` refuses, before anything else: a frame with
  `BuiltinPC != 0`, a move or recycle continuation, pending finally reasons, a
  WAIF setter `ReturnLocal`.

### What the lane does today

- `vm/checkpoint_resume_test.go:376-386` builds the suspended `task.Snapshot`
  by hand with no `CallStack` and `VM: machine.PersistenceVMSnapshot()`.

### Change made

- `vm/checkpoint_resume_test.go` `checkpointSuspendedVM`: builds the snapshot,
  sets `CallStack` from `machine.Task.GetCallStack()`, requires one activation
  per VM frame, calls `ExportNativeSnapshot` (an error is `t.Fatalf`), writes
  the exported snapshot, and requires every reloaded frame to be `Native`, so
  `RestoreVMSnapshot` runs `convertNativeSnapshot`. The write/reload/restore
  body moved unchanged into `reloadCheckpoint`.
- `checkpointSuspendedVMInBarnForm` keeps the old behaviour (Barn frame form)
  for the two tests whose subject is that form:
  `vm/rebuild_persistence_test.go:35` and `vm/persistence_temporaries_test.go:100`.
- `vm/fuzz_test.go` `runAll`: pushes the root activation on each task, as the
  engine does (`engine/eval.go:180` for the root, `engine/task_factory.go:296`
  for a fork child).
- New test `TestCheckpointExportsSuspendedEvalWithToastContinuation`
  (`vm/checkpoint_resume_test.go`): `return eval("suspend(); return 73;");`
  exports as 2 frames, the top with builtin `eval` at pc 2, reloads through
  the import, and the task returns `{1, 73}`.
- `vm/world_history_test.go` `TestWorldCheckpointDetectsChangedVerbCode`: its
  helper took the object in `ancestor` from the restored frame; it now takes
  it from the suspended frame. See Surprises. The test's expectation (the
  observer reports changed verb code) is unchanged.

### Results so far

- `go test ./vm -run 'TestCheckpointExportsSuspendedEvalWithToastContinuation$|TestCheckpointResumeSeedsReachCheckpoint$|TestWorldHistoryActionsReachCheckpoint$' -count=1`
  -> `ok github.com/MongooseMoo/barn/vm` (seed tests alone 2.302s; the new test alone PASS 0.04s).
- `go test ./vm -run 'FuzzCheckpointResumeEquivalence$|FuzzWorldCheckpointEffects$' -count=1 -v`
  -> `PASS`, `ok`. Checkpoint-resume corpus entries mostly SKIP ("no task
  suspended"), as before the change is unknown (not measured before).

### Measurements (temporary, uncommitted instrumentation that logged each export/import/difference and went on)

- Fuzz, 60 s each, 4 workers:
  `go test ./vm -run '^$' -fuzz 'FuzzCheckpointResumeEquivalence$' -fuzztime 60s -parallel 4`
  -> `execs: 211`, `PASS`, no export error, import error or difference logged.
  `go test ./vm -run '^$' -fuzz 'FuzzWorldCheckpointEffects$' -fuzztime 60s -parallel 4`
  -> `execs: 816`, `PASS`, nothing logged.
  I also ran the resume target a further 20 s to see the head of its output
  (`execs: 53`, PASS), so that target had 80 s, over the 60 s I was given.
- Both targets sit at `0/sec` for long stretches. An unchanged checkout of
  the same commit does the same (20 s each: 213 and 589 execs), so the
  stalls are not from this change. Why they stall: I don't know.
- Because 211 execs is thin, I wrote a throwaway scan (not committed): the
  seed recipe's pseudo-random 96-byte choice lists (PCG 7, 11) through
  `checkCheckpointResumeEquivalence` for 50 s, and random world histories for
  50 s.
  - Resume lane: 4676 programs, 28 checkpoints taken: 24 exported, reloaded
    in Toast form and agreed; 4 export errors in 2 classes (below). No import
    error, no result difference.
  - World lane: 131 histories, 1118 checkpoints, all exported, reloaded and
    agreed. Every one was a 1-frame task: this lane never suspends below a
    verb call or inside eval().

### Findings: export errors for programs Toast supports

Both come back wrapped by `vm/persistence.go:275`
(`frame %d (#%d:%s): %w`). In production each means
`engine/task_factory.go:489` logs a warning and the checkpoint holds a task
Toast cannot load.

1. **suspend() in a range loop's upper bound.**
   Error: `frame 0 (#0:): no held value for native operand kind 6 from *ir.RangeLoopStmt`
   Produced at `compiler/native_export.go:251` (reached from `:142`); kind 6
   is `NativeRangeNext` (`compiler/native_layout.go:16`).
   Minimal programs (each run through the lane's driver):
   - `for n in [1..suspend()] endfor return 1;` -> export error
   - `for n in [1..2] for k in [3..suspend()] endfor endfor return 1;` -> export error
   - `for n in [suspend()..3] endfor return 1;` -> exports
   - `for n in [1..3] suspend(); endfor return 1;` -> exports
   Generated programs that hit it (3 of 28 checkpoints), loop headers:
   `for n in [m..suspend()]`,
   `for s in [#-2..([]).({1.0, suspend()[...]})]`,
   `for s s in [`#-2 ! E_PROPNF, E_PERM'..(-1.0:look(E_FLOAT, suspend())[l])[!$]]`.
2. **suspend() inside a finally block.**
   Error: `frame 0 (#0:): pending finally reason has no Toast form`
   Produced at `compiler/native_export.go:97` when the try body fell through
   (the frame has 0 pending reasons, 1 stack value) and at `:35` when the try
   body returned (1 pending reason). Same text at both.
   Minimal programs:
   - `try ; finally suspend(0); endtry return 1;` -> export error
   - `try x = 1; finally suspend(0); endtry return x;` -> export error
   - `try return 5; finally suspend(0); endtry` -> export error
   - `try suspend(0); finally x = 1; endtry return x;` -> exports
   Generated program that hit it (1 of 28): `try ; finally ; equal({suspend(0)}, [...]); n; n; endtry`.

Can Toast hold these states? Read from Toast's source in WSL
(`/root/src/toaststunt/src`), not run:

- Class 1, yes. `code_gen.cc:1035-1038` (`STMT_RANGE`) generates `from`, then
  `to`, then `OP_FOR_RANGE`, so while `to` is in `suspend()` the `from` value
  is an ordinary operand on the rt stack. `write_activ` (`execute.cc:4005-4016`)
  writes every rt stack slot.
- Class 2, yes. `EOP_END_FINALLY` (`execute.cc:2609-2620`) replaces the
  finally marker with two plain values, the integer `FIN_FALL_THRU` and zero,
  and the finally body runs with them on the stack until `EOP_CONTINUE`
  (`execute.cc:2623-2645`) pops them. For a return or an error,
  `unwind_stack` (`execute.cc:269-276`) pushes the reason and the value the
  same way and jumps to the handler. Both are ordinary stack slots
  `write_activ` writes.

Shapes probed that export, reload in Toast form and resume to the same
result: suspend in a range loop body, a list loop body, a while body, an
except arm, a try body with a finally, a catch expression, a list literal,
an arithmetic operand, and inside eval() (the new test).

## Decisions and why

- The driver pushes root activations rather than the checkpoint helper
  inventing a call stack. `ExportNativeSnapshot` takes each frame's programmer
  from `callStack[index]`, and `vm.syncTaskLineNumbers` (`vm/vm.go:1085-1098`)
  already treats `Task.CallStack[i]` as frame `i`. The fuzz driver's tasks had
  no root activation, so their call stack was one short and misaligned.
- The root activation copies `engine.prepareIntrinsicEval`'s
  (`IsEvalFrame: true`), because the driver's root is a program run outside any
  verb. The engine package cannot be imported from `vm` tests (import cycle),
  so the literal is repeated in the driver.
- The Barn-form helper stays for the rebuild tests. Their sources include
  shapes `compiler/native_export.go:32-38` refuses (pending finally), and what
  they test is the Barn-form reload, which production still writes on an
  export error. Not a fallback in the lanes: the lanes use only the export
  helper.

## Surprises

- `ExportNativeFrame` (`compiler/native_export.go:28`) has no test caller
  either: grep for `ExportNativeSnapshot|ExportNativeFrame` over `*.go` finds
  only `engine/task_factory.go:485` and `vm/persistence.go`.
- The fuzz driver's tasks carried no root activation (see Decisions).
- I made the first edit of `vm/checkpoint_resume_test.go` with a Python
  script instead of the Edit tool. Later edits use Edit.

- With Toast-form frames a restored frame can hold a variable unbound that
  the suspended Barn frame still held. World history `{0, 127, 0}` last reads
  `ancestor` in `verb_code(ancestor, "probe")` before its `suspend()`; Toast
  compiles a variable's last push before the program's end as
  `OP_PUSH_CLEAR` (`code_gen.cc:1302-1335`), `compiler/native_export.go:158-162`
  leaves it out, and the reloaded frame has `ancestor` unbound (before: `#3`).
  The task's result is unaffected. `TestWorldCheckpointDetectsChangedVerbCode`
  read it from the restored frame and failed with `E_VARNF`.
- The lanes almost never export more than one frame. All 24 exported resume
  checkpoints and all 1118 world checkpoints were 1-frame tasks. Exported
  verb-call and eval frames are covered only by the new plain test.

## Unverified

- No Toast was run. Whether Toast loads and resumes a checkpoint these lanes
  write is not tested by a Barn-only lane.
- The Toast statements under Findings are from reading its source.
- Only the tests in the eight `vm` test files that use the fuzz driver or
  the checkpoint helpers were run locally, not the rest of the suite. The
  driver change (root activations) reaches `FuzzSourceExecution`,
  `FuzzStructuredSourceExecution` and the leak-check replays; their seed
  runs passed, their fuzzing was not run.
- After the reload the lane keeps the pre-checkpoint `task.State`
  (`restored.Task = machine.Task`), so `callers()` and `task_stack()` after a
  reload read the old activations, not the ones the reader built as
  `engine/task_load.go:86-88` installs.
- `engine/task_runtime.go:791` pushes a root activation only when the task
  has a verb name. Whether any production task reaches `TaskSnapshots` with
  fewer activations than VM frames: I don't know.
- Why both fuzz targets stall at `0/sec`: I don't know.

## Next step

The lead decides the fixes for the two export-error classes. Until they are
fixed, a fuzz run of `FuzzCheckpointResumeEquivalence` that reaches either
shape fails with `ExportNativeSnapshot: ...`.
