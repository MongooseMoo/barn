# Fix F10 — `t.Done` closed on suspend, not just on termination

## The bug
`ProcessReadyTasks` (`scheduler/scheduler.go`) unconditionally called
`close(t.Done)` after `runTask` returned. But `runTask` returns `nil` for BOTH
suspend/yield (`FlowSuspend`) and terminal completion. A task that merely
SUSPENDED therefore had its `Done` channel closed, falsely signaling termination
to any waiter (`<-t.Done` wakes believing the task finished while it is alive).

`Done` is a Barn-internal completion signal; ToastStunt has no equivalent, so
this is Go task-state correctness, not a Toast-behavior question. Authority: the
`Done` channel must close exactly once, only on actual termination.

## Terminal-vs-suspend distinction
After `runTask` returns, the authoritative discriminator is `t.GetState()`:
- `runTask` (`scheduler/task_runtime.go`) sets `TaskCompleted` on normal return
  (line 293) and `TaskKilled` on the uncaught-error/kill paths (lines 30, 39,
  93, 114, 122, 215, 262).
- Suspend leaves the task in `TaskSuspended`/`TaskQueued` (re-queued for resume).

The fix closes `Done` only when `state == TaskCompleted || state == TaskKilled`.
On suspend/yield it leaves `Done` open; the channel is closed later when the
resumed task finally reaches a terminal state. Suspend/resume behavior is
otherwise unchanged.

## Double-close guard
Added `Task.CloseDone()` in `task/task.go`: under `t.mu`, it no-ops when `Done`
is nil or a new `doneClosed bool` is already set, then sets the flag and closes.
This makes Done close-exactly-once even if a terminal task were ever processed
twice, with no close-of-closed-channel panic. `ProcessReadyTasks` now calls
`t.CloseDone()` instead of the raw `close(t.Done)`.

## Who waits on Done
`task.Task.Done chan struct{}` (`task/task.go:159`) is the per-task completion
signal. In production it is currently set only on the server input path
(`server/input_processor.go`) via the separate `command.Input.Done` channel;
`Task.Done` itself was latent (review ARCH-4: "nothing sets Task.Done today"),
which is why F10 was latent. `TestReview_DoneChannelClosedOnSuspend` wires a
`Done` channel onto a suspending task and is the active waiter the fix protects:
it asserts Done is NOT closed after suspend.

## Tests
```
$ go test ./scheduler/ -run TestReview_DoneChannelClosedOnSuspend -v
=== RUN   TestReview_DoneChannelClosedOnSuspend
--- PASS: TestReview_DoneChannelClosedOnSuspend (0.00s)
PASS
ok  	barn/scheduler	0.774s

$ go test -race ./scheduler/ ./task/
--- FAIL: TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent (0.00s)
    review_concurrency_test.go:134: BUG: ID collision at 3 ...
FAIL	barn/scheduler	0.446s
ok  	barn/task	1.177s

$ go test ./scheduler/... ./task/...
--- FAIL: TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent
ok  	barn/task
FAIL	barn/scheduler   (only the intentional F8 red test)

$ go vet ./scheduler/ ./task/      # clean, no output
```

## Before/after failure list
- Before: `TestReview_DoneChannelClosedOnSuspend` (F10) FAILED;
  `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent` (F8) FAILED;
  `TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask` (F6) race-clean.
- After: F10 PASSES. F8 still fails — it is an intentionally-red test that
  unconditionally calls `t.Errorf` (different bug, independent ID counters,
  untouched by this fix). F6 stays race-clean under `-race`.
- No NEW functional failures introduced (`barn/task` all green; `barn/scheduler`
  only the pre-existing intentional F8 red).

## Commit
`COMMIT_HASH_PLACEHOLDER` on branch `review/branch-stocktake-2026-06-25`.

## Files changed
- `task/task.go` — added `doneClosed` field + `CloseDone()` once-guarded closer.
- `scheduler/scheduler.go` — close `Done` only on `TaskCompleted`/`TaskKilled`
  via `t.CloseDone()`.
