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
  `engine/rebuild_load_test.go`. Red before the change.
- Implementation: in progress.

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

## Next step

Implement the export, writer form and `TaskSnapshots` change; run the tests above.
