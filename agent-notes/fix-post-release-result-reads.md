# Fix: task state read after the execution lease is released

## Goal

Follow-up to barn PR 1037 (runServerVerbTask read `t.Result` after releasing the
execution lease). (1) Read every remaining read of `t.Result`, or of another task
field a resumed slice writes, that happens after `releaseTaskExecution` in
`engine/` and `server/`; fix each that can observe a resumed slice's value, with a
forced-interleaving Go test per fixed site. (2) The network_matrix CI failure on
PR 1034, in its own commit.

## State

(1): one site fixed (output suffix), one found but not fixed (task-finished
diagnostics), the rest shown unable to see a resumed value. Committed locally,
not pushed. (2), the network_matrix failure, moved to its own branch
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
  nil otherwise, task_timeline.go `timelineFor`). Not fixed: there is no point
  after the release and before this defer where a test can run the scheduler, so
  a forced-interleaving test needs a new seam. Left for the lead.

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

## Runs

- `go test ./engine -run 'TestCommandSuffixPrecedesResumedSliceOutput$'` without
  the task_runtime.go change: FAIL as above. With it: ok.
- `go test -race ./engine -run '...Suffix...|TestProcessReadyTasksFlushesInReadyOrder$' -count=3`: ok.
- `gofmt -l engine`: clean; `go vet ./engine`: clean; `golangci-lint run ./engine/...`: 0 issues.
- `go test -race ./engine ./server -count=1`, first run (before the
  worker_pool_test.go update): engine FAIL only
  `TestSuspendedCommandFlushesSuffixOnlyOnce` (`[end "" ""]`), 518 s; server ok, 58.8 s.
- Same command after the update: engine ok (547.2 s), server ok (57.3 s), exit 0.

## Decisions and why

- Flush at the suspend hand-off, not in the lifecycle defer for every slice: a
  per-slice flush would run on scheduler workers in parallel and break
  `TestProcessReadyTasksFlushesInReadyOrder`'s ready-order flush, which only
  terminal slices need.

## Surprises

- `finishTimeline` is not idempotent, unlike `TaskFinished`.

## Next step

Lead publishes. Open: the double `finishTimeline` (needs a test seam).
