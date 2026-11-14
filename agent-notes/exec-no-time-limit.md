# exec() has no time limit

## Goal

Barn's `exec()` killed its child after 30 seconds. ToastStunt has no such
limit, and a Mongoose backup run through `exec()` takes well over two minutes.
Remove Barn's limit so `exec()` waits until the child exits, as Toast does.
Nothing else: no configurable timeout.

## State

Done on branch `exec-no-time-limit`; pull request open.

- `builtins/system.go`: the task path uses `context.WithCancel` (kill_task
  still cancels it) instead of a 30-second `context.WithTimeout`; the
  no-task fallback calls `execCommandWithContext(context.Background(), ...)`
  and the `execCommand` wrapper with its own 30-second limit is deleted; the
  now-unreachable `DeadlineExceeded` branch is deleted.
- `builtins/system_exec_test.go`: `TestBuiltinExecWaitsForChildPastThirtySeconds`
  runs a 31-second child through the exec builtin's task path and expects
  `{0, "done", ""}`. Before the fix it got `{1, "", ""}` at 30.10 s on Windows;
  after, it passes in 31.26 s.
- `spec/builtins/exec.md`: removed the "Timeout" row that said Toast limits
  exec time.

## Decisions and why

- Toast evidence is its source, `src/exec.cc` at the `mongoose` branch:
  `bf_exec` suspends with no timer; the task resumes only from
  `deal_with_child_exit` after `exec_complete` (child exit) or a stdin write
  failure, or ends by `kill_task` (`TWS_KILL`). No exec timeout anywhere in
  `src`.
- No conformance test: only a child that outlives 30 seconds can show the
  difference, so such a test would take 31+ seconds on each engine. The Go
  test is the regression guard. It cannot run in parallel (the exec fixture
  helper uses `t.Chdir`), so it adds about 31 s to `go test ./builtins`.

## Surprises

- On Windows a child killed by `exec.CommandContext` returns an
  `*exec.ExitError` with exit code 1, so the old timeout surfaced as
  `{1, "", ""}`, not the `{-1, "", ""}` the code's error path suggests.

## Next step

CI on the pull request, including the Linux run this change has not had.
