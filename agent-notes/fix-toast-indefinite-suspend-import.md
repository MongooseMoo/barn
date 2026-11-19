# Fix: indefinitely suspended task wrong after Barn loads a Toast checkpoint

## Goal

`server/restart_task_semantics.yaml::indefinitely_suspended_task_across_checkpoint_restart`
passes when Toast writes the checkpoint and Barn loads it. Observed on master:
expected `[[], 1]`, Barn returned `[[0], 0]` (the task had resumed with 0 and
was no longer queued).

## State

Fixed and committed on `fix/toast-indefinite-suspend-import`. Not pushed.

Cause:

- Toast `enqueue_suspended_task` (tasks.cc:1306) gives a `suspend()` with no
  argument `start_tv.tv_sec = INTNUM_MAX`; on the 64-bit oracle that is
  INT64_MAX (include/structures.h:52). `write_suspended_task` (tasks.cc:1917)
  writes it as `9223372036854775807`. `read_task_queue` (tasks.cc:2094-2096)
  keeps it, and maps a negative value to INTNUM_MAX.
- Barn `readSuspendedTask` (db/format/reader_vm.go:27) parses that number into
  `time.Unix(MaxInt64, 0)`. `loadSuspendedTask` (engine/task_load.go) set
  `WakeTime = saved.StartTime` and treated only `saved.StartTime.Unix() < 0` as
  indefinite. `time.Unix(MaxInt64, 0)` is outside what time.Time compares (its
  internal seconds wrap; it prints as year 292277026596 but is not after now),
  so the wake deadline read as already due and the scheduler resumed the task
  with 0.
- Barn's own writer writes `task.IndefiniteSuspendStartTime` (1<<62) for the
  same state (db/format/writer_vm.go:27), which is why Barn-to-Barn passed.

Change: `loadSuspendedTask` treats a saved start time below 0 or at or above
`IndefiniteSuspendStartTime` as indefinite: zero WakeTime and the sentinel
StartTime, the same state a live `suspend()` with no argument has.

Evidence:

- `go test ./engine -run 'TestToastIndefinitelySuspendedImportStaysSuspended$'`:
  red before the change (`wake/start = 292277026596-12-04 ...`), green after.
  Fixture `engine/testdata/toast-suspended-indefinite.db`, record in
  `engine/testdata/toast-suspended-indefinite.md`.
- `go test ./engine ./task ./db/format -count=1`: ok. `gofmt -l engine/` empty,
  `go vet ./engine` clean.
- Managed harness in WSL, suite at its `origin/main`, selection
  `-m 'admission or kept_db_restart' -k 'capability_admission or indefinitely_suspended_task_across_checkpoint_restart'`,
  Toast as `--server-command` and the fixed Barn Linux build as
  `--restart-server-command`: `2 passed`. Barn's log directory for the run
  held the restart's records.

## Decisions and why

- The threshold is `>= IndefiniteSuspendStartTime`, not `== MaxInt64`: Barn
  writes the sentinel for the same state, and no timed suspension reaches it
  (`task.StartAfter` saturates there).
- The fix is in the engine's load, beside the existing `-1` case, not in the
  db/format reader: the reader keeps the number Toast wrote.

## Surprises

- `/mnt/c` in the WSL Debian distro returned `Input/output error` for every
  path from this session (`chdir(...) failed 5` from the relay). I did not
  try to repair the mount. The run used WSL-native storage instead: the suite
  tree, the oracle manifest, the Barn build and the config were piped in over
  stdin, and the checkpoint came back base64 over stdout. The work directory
  and venv under `/root/.cache` were removed afterwards.

## Unverified

- Barn's `queued_tasks()` lists an indefinite task's start time as
  4611686018427387904 (task/task.go `toQueuedTaskInfoLocked` on the 1<<62
  sentinel); Toast's `list_for_suspended_task` (tasks.cc:2442) would list
  9223372036854775807. Barn's writer puts the same 1<<62 in a checkpoint. I did
  not check whether any conformance test reads that element, and changed
  neither.
- The other three failures of the cross-engine job, and the full cross-engine
  script, were not run here.

## Next step

Lead pushes the branch and opens the PR.
