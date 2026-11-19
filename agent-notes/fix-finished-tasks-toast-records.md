# fix/finished-tasks-toast-records

## Goal
Make Barn's `finished_tasks()` return what ToastStunt returns when built with
`SAVE_FINISHED_TASKS`, instead of a list of completed task ids. MOO code
written for Toast's records (for example, averaging `time` across them) failed
with E_DIV on Barn.

## State
Done. `engine` records one `builtins.FinishedTask` per task slice, right where
the lag check runs, and `builtins` keeps the newest
`$server_options.finished_tasks_limit` records (default 15, at least 1) on the
session. `finished_tasks()` returns them as Toast's maps. `server_version()`
declares `SAVE_FINISHED_TASKS` as 15 when the builtin is registered, `#-1`
otherwise. New candidate conformance file
`tests/mongoose-conformance/builtins/finished_tasks_records.yaml`.

## Decisions and why
- Kept on by default. `docs/builtin-descriptors.md` records Barn's default
  availability of `finished_tasks` as deliberate policy; Daiverd chose to keep
  it and match Toast's records rather than turn it off (2026-11-19).
- Toast (`execute.cc` `run_interpreter`) records each interpreter run: a new
  task, and every resumption. Fields describe the activation on top of the
  stack when the run started; `foreground` is 1 only for new command and server
  tasks; `suspended` is whether the run ended suspended; `time` is the run's
  wall seconds; an empty `verb`/`fullverb` reads `"n/a"`.
- Barn records once per slice after the commit outcome, committed or not, as
  Toast records whatever the outcome. Conflict retries re-run before that point
  and are not counted.
- `receiver` comes from the frame's `This`, which is already the class for a
  waif call and the prototype for a primitive; Toast uses #-1 for anonymous
  objects.
- Recording is skipped when `finished_tasks` isn't registered, so builds
  without it don't pay a map per slice.

## Surprises
- A forked task's first run starts in the forking verb's fork body, so Toast
  names that record after the forking verb, not the verb the fork calls.

## Oracle
Stock Toast has no `finished_tasks()`. The oracle was the OVH Toast tree
(lisdude 20668a7) built separately with
`cmake -S . -B build-finished-tasks -DCMAKE_BUILD_TYPE=Release -DCMAKE_CXX_FLAGS=-DSAVE_FINISHED_TASKS=15`.

## Next step
Not checked: records for tasks killed mid-run, nested server tasks run from
inside a task (Toast's `handle_uncaught_error`, which records before the outer
run), waif or primitive `this` values, and the order of records from tasks
running concurrently on Barn.
