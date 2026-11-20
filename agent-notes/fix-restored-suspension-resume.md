# Restored timed suspension resumed early

## Goal
resume() of a timed suspension restored from a checkpoint must make it ready at once, as Toast's resume_task does (start_tv = 0). Found by the Jepsen task lane (PR #750, CI run 42974481407: five released suspensions still queued 45 s after resume) and by moo-conformance-tests local branch `jepsen/held-task-restart` (`server/restart_held_task_semantics.yaml`, Toast 4/4 pass, Barn 3 fail).

## State
2026-11-11: fixed; regression test `engine/restored_suspension_resume_test.go` `TestRestoredTimedSuspensionResumedEarlyIsReadyNow` fails without the fix (ready at the saved deadline, an hour later) and passes with it. `go test ./engine ./task ./db/format` OK.

## Decisions and why
- Cause: a live `suspend(N)` keeps StartTime = task start and WakeTime = deadline (task/task.go Suspend). The checkpoint saves only the deadline as the start time (task/snapshot.go PersistenceSnapshotLocked). `loadSuspendedTask` set both StartTime and WakeTime to it. `ResumeAtRow` sets WakeTime = now, but `readyDeadlineLocked` takes the later of StartTime and WakeTime, and the waiting heap stops at a future StartTime (engine/scheduler.go readyLocked), so the task waited for the deadline.
- Fix at the loader: a restored suspension never gets a future StartTime (now instead), as a live one never has. queued_tasks() lists WakeTime for a suspended task (task/task.go queuedStartLocked), so its listed start is unchanged; the test checks that too.
- Team-lead decision: separate PR; #750 stays harness-only.

## Surprises
None beyond the cause.

## Next step
Merge (blocked until the D5 conformance fix lands, per team-lead), then merge master into #750.
