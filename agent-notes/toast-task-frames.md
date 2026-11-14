# Barn writes Toast-form task frames

## Goal

A checkpoint Barn writes must load in ToastStunt (the `mongoose` fork) and resume
its suspended and interrupted tasks, so a switch from Toast to Barn can be undone.
Each saved frame must carry the pc and operand stack Toast holds at the same call,
not Barn's instruction position and stack. A task that cannot be written that way
is left out of the file and logged.

## State

- Tests first: `engine/toast_task_export_test.go` (Barn-started tasks in seven
  frame shapes, plus a refused for-loop shape) and the Toast-fixture round trip in
  `engine/rebuild_load_test.go`. Red before the change, green after.
- Implemented: `compiler/native_export.go` `ExportNativeFrame`;
  `vm.ExportNativeSnapshot`; `Runtime.TaskSnapshots` exports or leaves out and
  logs; the writer writes native frames in Toast's form and the VM header with
  the root vector and func_id 0; `bytecode.Program.ForkBody`;
  `barnImportLayout.forkDepths`; `compileNativeVector` also returns the whole
  layout; `lowerer.importSlotGap` shared by import and export.
- Toast proof, run by hand-written scratch tooling (not committed): a Barn
  checkpoint of the public Test.db with two Barn-started suspended tasks (a fork
  body while loop; verb frames with an operand, try/finally, catch expression),
  loaded by the managed Toast build of the live fork commit through the managed
  conformance harness: admission and the resume check passed. The same tasks
  written in Barn's old form failed Toast's load with "Bad PC for suspended task".
- Real dump census (scratch, not committed): all 12 tasks with saved frames on
  the 2026-11-12 Mongoose dump import and export back to Toast's own pc, error
  pc, operand stack and root vector.
- Only the tests above were run; the rest of the suite is for CI.

## Decisions and why

- Export is the inverse of `compiler.ConvertNativeFrame` over the same tables
  (`compileNativeVector`, `NativeLayout` continuations, `barnImportLayout.calls`).
- It produces the `task.VMFrameSnapshot` with `Native` set that the reader already
  makes from a Toast file, so Barn reads its own checkpoint back through the
  existing import; the reader tells the forms apart by the activation's temporary
  value (a `__barn_vm_frame` list marks a Barn frame).
- The export runs in `Runtime.TaskSnapshots`, which only checkpoint serialization
  uses.
- A Barn-started fork frame (`ExtractFork`) is not the Toast fork-vector compile,
  so it is looked up in the whole program's tables at the fork body's offset,
  without the operands the parent held at the fork statement.
- Refused (task left out, logged): builtin continuations, pending finally reasons,
  WAIF-setter result locals, for-loop state, operands Barn no longer holds, stacks
  longer than Toast's max_stack, a program that is not the tables' compile.

## Surprises

- Toast's `read_activ` reads the saved operand stack into an allocation of exactly
  the vector's `max_stack` with no bound check, so a longer stack would overrun it.
- 11 of the 12 real tasks on the 2026-11-12 dump have frame 0 in Toast fork vector 0;
  Barn wrote root vector -1 for every task.

- Toast's error pc is the start of the call instruction (pc-2 after a builtin
  call, pc-1 after a verb call), not the pc; the fixture showed it.
- Barn's import keeps only variables the frame's own vector names, so a Toast
  fork frame loses parent variables its body never uses; the export cannot
  write them back. Pre-existing, logged by the fixture test.
- Barn no longer writes its own frame form for live tasks; the Barn-form writer
  is now reached only by tests (the reader still reads old Barn checkpoints).
  Whether to delete it is a decision for Q.

## Next step

Push and let CI run the full suite (needs the lead / Q). Open: make the Toast
resume check a durable managed test; Toast loading a whole real Barn-written
Mongoose checkpoint; the interrupted-task (read) writer path has no test of its
own; decide whether the Barn-form frame writer, now reached only by tests, goes.
