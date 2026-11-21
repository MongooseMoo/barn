# Fix: task state read after the execution lease is released

## Goal

Follow-up to barn PR 1037 (runServerVerbTask read `t.Result` after releasing the
execution lease). (1) Read every remaining read of `t.Result`, or of another task
field a resumed slice writes, that happens after `releaseTaskExecution` in
`engine/` and `server/`; fix each that can observe a resumed slice's value, with a
forced-interleaving Go test per fixed site. (2) The network_matrix CI failure on
PR 1034, in its own commit.

## State

(1): two sites fixed (output suffix; task timeline finished twice), the rest
shown unable to see a resumed value. Rebased onto origin/master after PR 1037
merged, so the branch stands alone. Suite coverage for the suffix order on
`test/suffix-before-resumed-output` (worktree
`C:/Users/Q/code/moo-conformance-tests-suffix-order`). Committed locally, not
pushed. (2), the network_matrix failure, moved to its own branch
`fix/booted-connection-queries` (note `agent-notes/fix-booted-connection-queries.md`
there); it is not on this branch.

## Findings for (1)

The lease is released in the lifecycle defer of `runTaskSliceAdmitted`
(`engine/task_runtime.go`, `s.releaseTaskExecution(t.ID)`). From then on a
suspended task that was requeued by `handOffSuspended` can be resumed by a
scheduler worker, which writes `t.Result` (task_runtime.go `t.Result = result`
after `launchSlice`), `t.Status`, and runs that slice's own settle.

Reads after release, by site:

- `runTaskAdmitted` `completion.callback(t.Result)`: the callback is only taken
  (`TakeOnCompleteForDelivery`) on the terminal path, after the `FlowSuspend`
  return. A terminal task is never resumed, so nothing else writes `t.Result`.
  Cannot observe a resumed value. Covers `CreateLoginHookTask`'s `onComplete` and
  `StartEval`'s `finish` (engine/eval.go).
- `runServerVerbTask` (task_factory.go): fixed in PR 1037; returns the slice
  result captured under the lease. Covers do_command, do_blank_command,
  do_out_of_band_command, handle_uncaught_error and handle_task_timeout.
- `reportUncaughtException` / `callTaskTimeoutHook` (`t.Result.Error`,
  `t.Result.Val`, `t.Result.CallStack`): called inside the slice before the
  defer, under the lease. Cannot observe.
- `ResumeReadingTask` (task_factory.go), `ExecuteVerbTaskSyncWithStart`
  (task_runtime.go): read no `t.Result`; both call `settleTaskResult`.
- `settleTaskResult` (runtime.go) -> `CloseDoneIfSettled`: reads status and
  `resultPending` under the task lock, written for exactly this race. Safe.
- `settleTaskResult` -> `flushTaskOutput` (runtime.go): reads and clears
  `t.CommandOutputSuffix`, then sends it. **Can observe**: see below. Fixed.
- `runTaskAdmitted`'s first defer: `t.GetState()`, then
  `CommandDiagnostic.TaskFinished` and `finishTimeline`. **Can observe**: if the
  worker resumes and finishes the task before this defer runs, both invocations
  see a terminal state and both call them. `TaskFinished` (metrics/command.go) is
  idempotent; `finishTimeline` (engine/task_timeline.go) is not: it calls
  `store.NoteTaskRetries` and `slowTimelines.add` and may log "slow task
  timeline" a second time. Only with `CONFLICT_CENSUS` tracking on (timelines are
  nil otherwise, task_timeline.go `timelineFor`). Fixed by construction, see
  below.

### Task timeline finished twice: the second fixed site

- Fix: `finishTimeline` (engine/task_timeline.go) returns at once when the
  timeline is already `Finished` (checked and set under `tl.mu`), so the census
  count, the slow-timeline add and its log happen once per task whichever
  invocation gets there first. `Finished` is never reset. No seam was needed.
- Test: `engine/finish_timeline_once_test.go` `TestFinishTimelineCountsTaskOnce`:
  census tracking on, a task run to completion by `runTask`, then a second
  `finishTimeline` call, as the other invocation's defer makes it. Without the
  fix the census row counts the task twice (`Tasks = 2`); with it once. The test
  calls the function directly rather than forcing the scheduler interleaving:
  the fix makes the function idempotent, which is the property under test.

### Output suffix: the fixed site

- Toast `tasks.cc` `do_command_task`: the command verb runs synchronously through
  `do_input_task` (execute.cc:3372) until it returns or suspends, then
  `if (tq->output_suffix) notify(tq->player, tq->output_suffix);`. The suffix is
  sent before any resumption can run.
- Barn: `ExecuteVerbTaskSyncWithStart` puts the suffix on the task
  (`t.CommandOutputSuffix = outputSuffix`). It was sent only by `settleTaskResult`
  after the lease release. A worker could resume the task in between: its output
  went first, and its own settle could send the suffix (both settles read and
  clear the field unsynchronised).
- Fix: `handOffSuspended` sends a still-pending suffix (`flushTaskOutput`) before
  `SetVM`/`RequeueYield`, while the lease is held. Terminal slices keep the
  settle-time flush, so batch flush order is unchanged.
- Test: `engine/post_release_reads_test.go`
  `TestCommandSuffixPrecedesResumedSliceOutput`. Command verb
  `suspend(0); notify(player, "after");` with suffix `SUFFIX`; the output flusher
  drains the scheduler before recording a suffix. Without the fix:
  `sent = ["after", "SUFFIX"]`; with it `["SUFFIX", "after"]`.
- `engine/worker_pool_test.go` `TestSuspendedCommandFlushesSuffixOnlyOnce`
  expected two flusher calls `[end, ""]`; the hand-off now adds one, so it
  expects `[end, "", ""]`. The suffix is still sent exactly once, first.

### Suite coverage (Toast-first)

- `server/input_task_completion.yaml`
  `native_command_suffix_precedes_output_after_suspend`: logged-in connection,
  `OUTPUTPREFIX ==START==`, `OUTPUTSUFFIX ==END==`, then a command verb
  `notify(player, "before"); suspend(0); notify(player, "after");`; expects
  output exactly `["==START==", before, "==END==", after]`.
- Managed runs with the file selector and `-k "capability_admission or
  (input_task_completion)"`: stock WSL Toast 4 passed (load 12.00); Barn without
  the suffix fix (origin/master `engine/task_runtime.go`) 4 passed in 3 runs
  (load 13.89, 12.68, 12.10); fixed Barn 4 passed in 2 runs (load 11.91, 12.78).
- The harness does not hit the window on unfixed Barn: the resumed slice would
  have to run between the lease release and the input goroutine's settle, a few
  microseconds. The test is kept as Toast-proven coverage of the order; the Go
  test is what forces the interleaving.

## Runs

- `go test ./engine -run 'TestCommandSuffixPrecedesResumedSliceOutput$'` without
  the task_runtime.go change: FAIL as above. With it: ok.
- `go test -race ./engine -run '...Suffix...|TestProcessReadyTasksFlushesInReadyOrder$' -count=3`: ok.
- `gofmt -l engine`: clean; `go vet ./engine`: clean; `golangci-lint run ./engine/...`: 0 issues.
- `go test -race ./engine ./server -count=1`, first run (before the
  worker_pool_test.go update): engine FAIL only
  `TestSuspendedCommandFlushesSuffixOnlyOnce` (`[end "" ""]`), 518 s; server ok, 58.8 s.
- Same command after the update: engine ok (547.2 s), server ok (57.3 s), exit 0.
- `TestFinishTimelineCountsTaskOnce` before the task_timeline.go change: FAIL
  (`census counted the task 2 times, want 1`); after: ok.

## Decisions and why

- Flush at the suspend hand-off, not in the lifecycle defer for every slice: a
  per-slice flush would run on scheduler workers in parallel and break
  `TestProcessReadyTasksFlushesInReadyOrder`'s ready-order flush, which only
  terminal slices need.

## Surprises

- `finishTimeline` was not idempotent, unlike `TaskFinished`.

## Next step

Lead publishes the suite PR and the Barn PR. Nothing open.

## Barn PR body

Title: Send a suspended command's output suffix before it can resume

Toast's do_command_task runs a command until it returns or suspends, then sends
the connection's output suffix, before the suspended task can resume. Barn sent
the suffix from settleTaskResult, after releasing the task's execution lease, so
a scheduler worker could resume the task first: its later output went out ahead
of the suffix, and both settles read and cleared the suffix unsynchronised.

- handOffSuspended sends a pending suffix before the task is requeued.
- finishTimeline acts once per task. A suspended task's first invocation can
  reach its terminal-state check after a worker has resumed and finished the
  task, and both then finished the timeline, counting the task twice in the
  conflict census (and repeating the slow-timeline record) with CONFLICT_CENSUS
  on.

Tests: `TestCommandSuffixPrecedesResumedSliceOutput` (forces the resume between
the lease release and the settle; `["after", "SUFFIX"]` before, `["SUFFIX",
"after"]` after), `TestFinishTimelineCountsTaskOnce`, and
`TestSuspendedCommandFlushesSuffixOnlyOnce` updated for the extra flush call
(the suffix still goes once, first). Suite test
`server/input_task_completion.yaml::native_command_suffix_precedes_output_after_suspend`
(suite PR) passes on stock Toast and on Barn.

## Suite PR body

Title: Cover the output suffix preceding a suspended command's later output

Adds `native_command_suffix_precedes_output_after_suspend` to
`server/input_task_completion.yaml`: with OUTPUTPREFIX/OUTPUTSUFFIX set, a
command that prints, suspends with `suspend(0)` and prints again must produce
`==START==`, `before`, `==END==`, `after` in that order. Toast's do_command_task
sends the suffix after the command's first run returns or suspends, before the
task can resume. Managed runs: stock WSL Toast passes; Barn passes before and
after the matching Barn PR, because the harness does not hit the microsecond
window that PR closes (a Go test there forces it).
