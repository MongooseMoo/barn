# Fix F28v2 — indefinite suspend must sort LAST in queued_tasks()

## The regression F28 uncovered

F28 (`f395706`) flipped `builtinQueuedTasks` to ascending-by-StartTime (Toast
order). That surfaced a latent Barn bug: an INDEFINITE `suspend()` (no/negative
seconds) was given `StartTime ≈ now`, so under the ascending comparator it sorted
FIRST. Toast sorts it LAST. The full suite then failed one test:
`waif::queued_tasks_returns_valid_waifs_for_programmers` (expected `:c` before
`:a`, got `:a` before `:c`).

## Toast authority (source is the oracle; WSL down)

`enqueue_suspended_task` in `~/src/toaststunt/src/tasks.cc:1295-1321`. When the
suspend is indefinite (`data == NULL`):

```
} else {
    when.tv_sec = INTNUM_MAX;   // tasks.cc:1306
    when.tv_usec = 0;           // tasks.cc:1307
}
...
t->t.suspended.start_tv = when; // tasks.cc:1314
```

`INTNUM_MAX` = `INT32_MAX` / `INT64_MAX` (`structures.h:45,52`). So an indefinite
suspend's `start_tv` is effectively infinite and sorts after every timed task.

## The Barn bug

`task/manager.go SuspendTask` mapped `seconds < 0` → `task.Suspend(0)`, leaving
`StartTime` at the task's creation time (~now). queued_tasks() sorts on, and
reports, `Task.StartTime` — so the indefinite task sorted first.

## The fix (StartTime sentinel, mirroring INTNUM_MAX)

`task/task.go`:
- Added `IndefiniteSuspendStartTime = time.Unix(1<<62, 0)` — a far-future
  sentinel (Barn's analogue of Toast's INTNUM_MAX `start_tv`).
- Added `Task.SuspendIndefinite()`: sets `State = TaskSuspended` and
  `StartTime = IndefiniteSuspendStartTime`. **WakeTime is left zero**, so
  `WakeDue()` (which requires `!WakeTime.IsZero()`) stays false — the scheduler
  never auto-wakes it. Only an explicit `resume()` does.
- `Task.Resume()` now clears the sentinel: if `StartTime == sentinel`, it resets
  `StartTime = time.Now()`.

`task/manager.go`:
- `SuspendTask` `seconds < 0` branch now calls `task.SuspendIndefinite()` instead
  of `task.Suspend(0)`.

The F28 ascending comparator in `builtins/tasks.go` is unchanged.

## Why Resume must clear the sentinel (the resume-path trap)

The prompt warned to check the resume path. It DOES key off StartTime:
`scheduler/scheduler.go:147` gates readiness with
`(t.WakeTime.IsZero() || !t.WakeTime.After(now)) && !t.StartTime.After(now)`.
A far-future sentinel StartTime makes `!StartTime.After(now)` false, so a naive
sentinel would leave a resumed indefinite task permanently unrunnable. Clearing
the sentinel inside `Resume()` (only when StartTime equals it) keeps resume
working while preserving the sort behavior of still-suspended tasks. `runTask`
re-stamps `StartTime = time.Now()` when the resumed VM actually runs
(`task_runtime.go:61`), so the brief queued window is the only place the cleared
value is observed — and a resumed task is no longer "indefinitely suspended".

## suspend(0) unchanged (distinct from indefinite)

`SuspendTask` `seconds == 0` still does `task.Suspend(0)` then
`task.Resume(0)` — StartTime stays ~now, never equals the sentinel, so `Resume`'s
sentinel-clear is a no-op for it. Zero-yield behavior is untouched. Timed
`suspend(N>0)` is also untouched.

## Tests

- `builtins/review_io_test.go`
  - NEW `TestReview_IO_QueuedTasksIndefiniteSuspendSortsLast`: a `fork(100)`-style
    timed task (StartTime = now+100s) and an indefinite-`suspend()` task; asserts
    the indefinite task sorts AFTER the timed one, that its StartTime is the
    sentinel, and that `WakeDue(now)` is false (no auto-wake). This is the unit
    analogue of the failing `:c`-before-`:a` conformance case.
  - FIXED existing `TestReview_IO_QueuedTasksSortOrder`: it set StartTime then
    called `SuspendTask(-1)`, which now overwrites StartTime with the sentinel.
    Moved the StartTime assignment to AFTER the suspend so the comparator is still
    exercised with two distinct finite times. Still PASS.
- `scheduler/task_runtime_test.go`
  - NEW `TestIndefiniteSuspendNotAutoWokenThenResumeRuns` (end-to-end):
    `suspend(); return 42;` runs to the indefinite suspend (asserts sentinel
    StartTime, no WakeDue), `ProcessReadyTasks()` does NOT auto-wake it, then
    `Manager.ResumeTask(...)` wakes it (asserts sentinel cleared) and a final
    `ProcessReadyTasks()` runs it to `TaskCompleted`. Proves resume() still works.

## Gate

- `go vet ./task/ ./scheduler/ ./builtins/` — clean.
- `go test ./builtins/ -run 'QueuedTasks|Suspend|Resume|Tasks' -v` — PASS.
- `go test ./task/... ./scheduler/... ./builtins/...` — only PRE-EXISTING red:
  `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent` (scheduler),
  `TestReview_Data_IsMemberStrCaseSensitiveBug`,
  `TestReview_Data_PcreMatchEmptySubject` (builtins). Proven pre-existing by
  running them in a clean `git worktree` at HEAD — all three fail there too,
  unchanged by this fix.
- `go test -race ./scheduler/ ./task/` — no data races; `./task/` ok; `./scheduler/`
  only the pre-existing IDCollision red.

## Before / after

- Before: indefinite `suspend()` → StartTime ~now → sorts FIRST → conformance
  `waif::queued_tasks_returns_valid_waifs_for_programmers` FAIL (3987/1/131).
- After: indefinite `suspend()` → StartTime = far-future sentinel → sorts LAST,
  resume() still wakes it. Expectation: full suite back to 3988/0/131 (Verifier
  re-runs the suite).

## Commit

`ac95153` on `review/branch-stocktake-2026-06-25`.
