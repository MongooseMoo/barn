# Fix fork/suspend delay overflow

## Goal
A MOO fork or suspend delay of 2^63 ns (about 9223372036 s) or more must start
the task that many seconds in the future, as Toast does, instead of
overflowing time.Duration and running it at once. Real producer: Mongoose
#9655:arm line 6 `fork tid (secs)` with delays such as 363410236465734.0.

## State
Two Barn commits: the delay overflow fix, then the queued_tasks listing fix
for suspended eval-forked tasks. Conformance rows (four) are on the
conformance repo branch `fix/fork-delay-far-future`. Not pushed, no PR.

## Decisions and why
- Toast authority: `double_to_start_tv` (src/tasks.cc:102-112) adds
  floor(seconds) to gettimeofday as a timeval; used by fork (tasks.cc:1290,
  from execute.cc:2084-2111) and timed suspend (tasks.cc:1304, from
  execute.cc:3520-3540 suspend and :3548-3577 yield_if_needed). queued_tasks
  reports ROUND(start_tv) (tasks.cc:53, :2364, :2436).
- Saturating the Duration is not enough: the largest Duration is 9223372036 s,
  below the tested 1e10 s. So `task.StartAfter(now, seconds)` returns a
  time.Time: in-range delays keep `now.Add` (monotonic clock), longer ones add
  whole seconds to the wall clock. Past IndefiniteSuspendStartTime (2^62 Unix
  s), where Toast's own time_t arithmetic overflows (undefined), it saturates
  at that sentinel. Not tested there.
- `types.ForkInfo.Delay` now carries MOO seconds (float64); the start time is
  computed once in engine/task_factory.go with StartAfter. SuspendSeconds
  uses StartAfter for the wake time (covers suspend() and yin()).
- Not changed (no observed producer, different paths): builtins/curl.go:30,
  builtins/system.go:874 (exec delay), server/input_processor.go:129
  (connect_timeout), server/server.go:107 (checkpoint interval),
  engine/call_verb.go:399 and engine/task_runtime.go:426 (seconds limits).
  Each multiplies MOO-supplied seconds into a Duration and wraps above
  about 9.22e9 s.

## Surprises
- The first Toast run selected the new rows without the `queued_tasks`
  capability provider and skipped them ("assumes 'queued_tasks' which has no
  provider"); the selector must include `capabilities/queued_tasks.yaml`.
- engine/task_manager.go:224-230 (`GetQueuedTasks`) omitted a suspended forked
  task whose VerbName is empty ("evalScaffold"), so a child forked from eval
  code that then suspends was never listed by queued_tasks(), whatever its
  delay. Toast lists every waiting task with no such filter (tasks.cc:2571-2581
  in bf_queued_tasks), and the eval-forked suspend row passes on Toast. The
  filter came from 2026-05-05 commits with no body and no test; the reason
  recorded in agent-notes/notes-conformance-zero-baseline.md:368-372 was
  exact-count exec rows, which now filter by task id
  (server/exec.yaml). Removed in a separate commit with Go test
  `TestQueuedTasksListsSuspendedForkWithoutVerb`. The conformance branch keeps
  the eval-forked suspend row and adds a verb-based one.

## Next step
Coordinator: push both branches, open PRs, run the real-workload measurement.
