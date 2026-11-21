# Going back to Toast

How to take a database Barn has been running and start ToastStunt on it. Each
step says what has been checked and how; anything marked **not verified** has
not been run.

## Steps

1. **Take Barn's newest checkpoint.** A running Barn writes its checkpoint to
   the `--db` path plus `.new` (`db/format.BeginCheckpoint`), with a WAIF
   identity sidecar beside it (`.new.waifids`); an emergency checkpoint goes to
   `.new.PANIC`. The input `--db` file is never rewritten. Stop Barn, or wait
   for a checkpoint to finish, and copy `<db>.new` to a working name, for
   example `back.db`.

   On shutdown (`server/server.go` `shutdownWithCheckpoint`) Barn calls
   `#0:shutdown_started`, closes listeners and connections, stops input, and
   then writes a final checkpoint to the same `<db>.new` (through
   `dbformat.BeginCheckpoint`, not renamed afterwards) when
   `--checkpoint-interval` is above 0 (default 3600) or the shutdown came from
   SIGINT or SIGTERM (`server/signals_unix.go`). With `--checkpoint-interval 0` and any other shutdown
   (`shutdown()` from MOO) it logs `final checkpoint skipped (checkpointing
   disabled)` and writes nothing, so `<db>.new` is the last periodic
   checkpoint. A failed final checkpoint logs ERROR `final checkpoint failed`
   and shutdown continues. Before taking the file, run
   `bin/barn_logs --level error` on the run's log: it exits 1 when the run
   logged any error, and `final checkpoint failed` among them means `<db>.new`
   is not the final state. After a shutdown that did not come from a signal,
   the final checkpoint also logs `checkpoint complete`; a signal shutdown
   writes it without the checkpoint hooks and logs no completion line, so
   there the absence of the ERROR is the check. Covered by
   `server/lifecycle_hook_test.go` `TestShutdownFinalCheckpointRunsHooksBeforeRuntimeStops`
   (source read; that test was not run for this doc).

2. **Write a copy Toast can load.**

   ```
   barn offline-checkpoint --db back.db --toast-loadable
   ```

   This loads `back.db`, restores its tasks into a runtime that never runs,
   and writes `back.db.new` the way a running server writes its checkpoint,
   except that every suspended or interrupted task Barn kept in its own frame
   form is left out. No MOO code runs and `back.db` is not changed. It prints
   one line per task it left out, then the counts, and exits 0:

   ```
   read: 0 queued, 13 suspended or interrupted tasks, 0 connections
   omitted task 18: #-1:, this #-1: frame 1 (#4:initialize): create: stock Toast does not resume a create, recreate or recycle continuation from its checkpoint
   ...
   wrote back.db.new: 0 queued, 6 suspended or interrupted tasks, 0 connections, 7 tasks omitted for Toast
   ```

   Each line names the task id, the verb (`#object:name`, `#-1:` for an eval
   or a fork from one), `this`, and the reason Barn gives. Without
   `--toast-loadable` the same command keeps every task, and a file holding a
   Barn-form task is refused by Toast as a whole:

   ```
   *** READ_ACTIV: Bad PC for suspended task.
   *** READ_TASK_QUEUE: Bad suspended task vm, count = 7
   *** DB_LOAD: Cannot load database!
   ```

   `back.db.new.waifids` is Barn's sidecar; Toast does not read it.

3. **Start Toast on the copy** with the Mongoose build the live game runs,
   `0ff6fd77659bd0ad001176dffdd007f854afa96b`
   (`profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`,
   `docs/mongoose-conformance.md`):

   ```
   moo back.db.new toast-out.db PORT
   ```

## What is left out, and why

A task is left out when Barn cannot write its frames in Toast's form
(`vm.ExportNativeSnapshot`, `compiler.ExportNativeFrame`). Toast reads every
saved frame as its own and stops the whole load at the first one it cannot
read. Reasons that a test has produced (`internal/dbtool/toast_loadable_test.go`):

- A task suspended inside a verb that `create()`, `recreate()` or `recycle()`
  called (`initialize`, the object's `recycle`), directly or through
  `call_function`: `ErrToastLosesObjectContinuation`. Stock Toast does not
  resume its own form of these continuations correctly (issue #981).
- A call whose unbound variables depend on the path into its block:
  `compiler.ErrPathDependentClear`.
- A task suspended in a WAIF's `_set_index` verb (`w[i] = v`):
  "instruction position N is not paused at a call".

Other refusals exist in `compiler/native_export.go` and `vm/persistence.go`
(builtin continuations with no Toast form, operand stacks Barn no longer
holds, a stack over Toast's `max_stack`, and others) that no test has produced
yet.

A left-out task is gone on Toast: it does not resume, and whatever it would
have done after its suspension does not happen. The objects it created or was
recycling stay as they were at the checkpoint (an object whose `recycle` verb
was suspended is still valid; an object `create()` made before `initialize`
suspended exists).

Queued (forked, not yet started) tasks are written as source and always kept.

## What was verified

All runs on the workstation's WSL Debian, recorded in
`agent-notes/fix-toast-loadable-copy.md`:

- A Barn-written file holding 13 suspended tasks (6 in Toast's form, 7 in
  Barn's: path-dependent clear, `create`, `call_function("create")`,
  `recreate`, `recycle`, `call_function("recycle")`, WAIF `_set_index`):
  stock Toast (`03af4fa2`) and the Mongoose `0ff6fd7` build both refuse it;
  both load the converted copy in emergency mode with exactly the six
  Toast-form tasks, and the world walk Toast prints (every object's name,
  parents, location, contents, verbs, properties) is identical to Barn's on
  the same copy.
- Through the managed conformance harness, Barn writes the checkpoint,
  `scripts/toast-loadable-restart.sh` converts it, and stock Toast resumes:
  a task suspended in a verb call resumes with the right result while a task
  in `create()`'s `initialize` is left out; without the conversion Toast does
  not boot. All 14 of Barn's own checkpoint restart tests
  (`tests/mongoose-conformance/server/*checkpoint*.yaml`) pass through the
  conversion, so it keeps every task those tests leave for Toast.

- Each fuzz cycle, the generated cross-engine lane
  (`scripts/fuzz_cross_engine.py`, job ToastCrossEngineCheckpoint) converts
  every Barn-written checkpoint of a generated program that stock Toast does
  not boot on and requires Toast to boot on the copy; its report counts the
  omitted tasks by reason under `## converted copies`. A local run on the
  programs that found this (a task suspended in `recreate()`'s initialize and
  one in `call_function("create", ...)`'s): Toast booted on both copies, one
  task omitted for each.

**Not verified:** the lane on a GitHub runner; resuming the converted copy on the `0ff6fd7` build (only
loading it was run there); the command on the Mongoose dump; how many tasks of
the live game are in Barn's frame form at a given checkpoint.

## Other differences that matter when going back

- Verbs calling builtins Barn has and Toast lacks load in Toast as
  `call_function("name", ...)` and fail when they run. `barn toast-check`
  names them (issue #977; the AST builtins exist in `0ff6fd7`, not in older
  Mongoose builds).
- An indefinitely suspended task's start time is written by Barn as 2^62
  (`task.IndefiniteSuspendStartTime`) where Toast writes `INTNUM_MAX`; Toast
  loads it as a timed wake 2^62 seconds away, so it still never wakes on its
  own. No issue filed; seen by the #976 machine.
- `ancestors()` of an object with several parents comes out in a different
  order on Barn (breadth first) than on Toast (depth first). Code that
  depends on that order behaves differently after the switch. No issue filed;
  seen by the #976 machine.
- Barn strings are Unicode; Toast's are bytes. `length()` of a non-ASCII
  string differs. By design.
