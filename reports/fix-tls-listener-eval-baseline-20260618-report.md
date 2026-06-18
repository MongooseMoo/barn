# Fix TLS Listener Eval Baseline Report

## Findings

- Reproduced the target failure with `go test ./server -run TestTLSListenerLoginAndEval`: the test read `"I couldn't understand that.\r\n"` instead of `{1, 3}`.
- The TLS transport and `EvalCommand` path were not the fault. The connection manager intentionally enqueues an initial empty input on new connections, matching the documented Toast-style startup input behavior.
- The test fixture's `do_login_command` returned `#2` for every input, including that initial empty startup input. The explicit `connect test` line was then processed after login as an ordinary command, queued the huh response, and displaced the later eval response read.

## Changed Files

- `server/tls_listener_test.go`
  - Updated the test `do_login_command` fixture to return `#2` only for an explicit `connect` command.
  - The fixture now returns `#-1` for the startup empty input, preserving the production connection/login behavior and keeping the test focused on TLS login plus eval.

No DB/store cleanup files or storage architecture paths were changed.

## Verification

- `go test ./server -run TestTLSListenerLoginAndEval` - passed
- `go test ./server` - passed
- `go test ./db ./builtins ./vm` - passed
- `git diff --check` - passed

## Commit

- `26ad30f13ab4dfaaf1acd47a3c6acd99c0e34894` - `Fix TLS listener eval baseline`

## Worktree Note

The repository had pre-existing unrelated modified and untracked files before this task, including `go.mod`, `go.sum`, `toastcore.db`, and many untracked prompt/report/test artifacts. I staged and committed only `server/tls_listener_test.go`.
