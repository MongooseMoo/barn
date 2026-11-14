# Barn writes Toast-form task frames

## Goal

A checkpoint Barn writes must load in ToastStunt (the `mongoose` fork) and resume
its suspended and interrupted tasks, so a switch from Toast to Barn can be undone.
Each saved frame carries the pc and operand stack Toast holds at the same call,
not Barn's instruction position and stack. A live Barn checkpoint never leaves a
task out: a task Toast cannot hold is written whole in Barn's own frame form, with
a warning that Toast cannot load that file. A separate checkpoint option, off by
default, leaves such tasks out to make a file Toast can load.

## State

- Tests: `engine/toast_task_export_test.go` (Barn-started tasks in fifteen frame
  shapes, including list, map, range and string `for` loops and a self-appending
  list; a mixed Toast-form/Barn-form checkpoint that reloads whole; the
  Toast-loadable option) and the Toast-fixture round trip in
  `engine/rebuild_load_test.go`.
- Implemented: `compiler/native_export.go` `ExportNativeFrame` (with loop state);
  `vm.ExportNativeSnapshot`; `Runtime.TaskSnapshots` exports or keeps Barn form
  with a WARN; the writer writes native frames in Toast's form and the VM header
  with the root vector and func_id 0; `Checkpoint.OmitBarnFrameTasks` /
  `Omitted` (not wired to any flag: the `offline-checkpoint` branch owns the
  command); `bytecode.Program.ForkBody`; `barnImportLayout.forkDepths`,
  leading-splice and self-append list prefixes; `compileNativeVector` returns the
  whole layout; `lowerer.importSlotGap` shared by import and export.
- Toast proof, scratch tooling only (scratchpad, not in the repo): Barn
  checkpoints of the public Test.db, loaded by the managed Toast build of the live
  fork commit through the managed conformance harness: two tasks (fork-body while
  loop; verb frames with operand, try/finally, catch expression) and three tasks
  (inside a list loop at a verb call, a map loop, a range loop) resume with the
  right results. The old Barn form fails Toast's load ("Bad PC for suspended task").
- Real dump census (scratch, deleted): all 12 saved-frame tasks of the 2026-11-12
  Mongoose dump import and export back to Toast's own pc, error pc, stack, vector.

## Decisions and why

- Export is the inverse of `compiler.ConvertNativeFrame` over the same tables
  (`compileNativeVector`, `NativeLayout` continuations, `barnImportLayout.calls`).
- It produces the `task.VMFrameSnapshot` with `Native` set that the reader already
  makes from a Toast file; the reader tells the forms apart per frame by the
  activation's temporary value (a `__barn_vm_frame` list marks a Barn frame), so
  one checkpoint may mix tasks of both forms.
- A Barn-started fork frame (`ExtractFork`) is not the Toast fork-vector compile,
  so it is looked up in the whole program's tables at the fork body's offset,
  without the operands the parent held at the fork statement.
- Loops: Toast holds the container and its next 1-based index, or a map's next
  key (CLEAR after the last); Barn holds value/key columns and a 1-based current
  index. A map is rebuilt from its columns; a list is its value column. A string's
  column keeps no mark that it was a string, so it is written as the list of its
  characters: Toast then binds the same elements Barn would.
- Not written in Toast's form (task kept in Barn form, WARN): builtin
  continuations, frames inside a finally clause, WAIF-setter result locals,
  computed except codes, operands Barn no longer holds, stacks longer than
  Toast's max_stack, a program that is not the tables' compile.
- The Barn-form frame writer stays: live tasks in those shapes use it.

## Surprises

- Toast's `read_activ` reads the saved operand stack into an allocation of exactly
  the vector's `max_stack` with no bound check, so a longer stack would overrun it.
- 11 of the 12 real tasks on the 2026-11-12 dump have frame 0 in Toast fork vector 0;
  Barn wrote root vector -1 for every task.
- Toast's error pc is the start of the call instruction (pc-2 after a builtin
  call, pc-1 after a verb call), not the pc.
- Barn's import labelled a leading `@expr` list and the self-append idiom
  `v = {@v, ...}` with origins Toast's layout does not use, so a call inside such a
  list could not be imported or exported; fixed in `barnImportLayout`.
- Barn's import keeps only variables the frame's own vector names, so a Toast
  fork frame loses parent variables its body never uses. Pre-existing.

## Next step

Push and let CI run the full suite (needs the lead / Q). Open: a durable managed
Toast resume test in the repo; Toast loading a whole real Barn-written Mongoose
checkpoint; the interrupted-task (read) writer path has no test of its own; wire
`OmitBarnFrameTasks` into the offline checkpoint command on that branch.
