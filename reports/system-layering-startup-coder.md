# Startup input and failure resource ownership

## Goal

Reject unreadable startup scripts before constructing runtime/listener/profile
resources, and drain the existing builtin session on listener-reservation
failure after stopping runtime owners.

## State

Read the composition scout, app startup sequence, existing listener-failure
cleanup, profile publication and relevant fixture patterns. Added focused
regressions in the two existing listener_reservation_test.go files. Both fail
against the original owners and pass after the direct corrections. Other local
work remains outside this bounded change.

RED command inside verified Debian WSL from this worktree:

```text
go test ./internal/app ./server -run '^(TestRunRejectsMissingStartupScriptBeforeProfileAndServer|TestStartListenerFailureClosesBuiltinHandles)$' -count=1 -timeout=30s
```

Raw RED excerpts:

```text
--- FAIL: TestRunRejectsMissingStartupScriptBeforeProfileAndServer
Run = load database: load database: unsupported database format: invalid database, want native missing-script error before server/database work
missing script created profile manifest: <nil>
FAIL github.com/MongooseMoo/barn/internal/app 0.042s
--- FAIL: TestStartListenerFailureClosesBuiltinHandles
file_name after failed Start: found=true result={Val:"startup-owned" Flow:0 Error:E_NONE ...}, want E_INVARG
sqlite_info after failed Start: found=true result={Val:[... "path" -> ":memory:" ...] Flow:0 Error:E_NONE ...}, want E_INVARG
FAIL github.com/MongooseMoo/barn/server 0.030s
```

## Decisions and why

Move the existing script read/line preparation to the pure validation phase;
retain later startup-line composition and execution order. Complete the existing
ReserveListeners failure branch with Session.Close and join cleanup errors to
the native listen error. No general Close facade, blanket shutdown defer,
panic behavior change or LoadDatabase redesign.

The original script read/line-preparation block now follows pure option/tool/
listener validation and precedes tracing, server construction, profile creation,
listener binding, database load and memory watching. Later StartScriptFirst,
StartLine and script appends are untouched; nil versus empty input remains the
same. The listener failure branch still cancels, closes listeners, stops input
and Runtime, joins background work and clears running state. It now closes the
existing builtin Session and joins that error with the wrapped native listen
error. Normal and panic shutdown paths are untouched.

The app regression uses valid configuration/profile inputs and an invalid
database to show missing-script validation wins before profile creation and
database work. The server regression pre-opens a real file and in-memory SQLite
handle through Runtime.Eval, verifies them before Start, holds an actual TCP
listener to force reservation failure, checks the native net.OpError remains
visible and proves both former handles are rejected afterward. Cleanup drains
the retained Session even when the regression fails against the old code.

GREEN output from the identical two-new-test selector:

```text
gofmt_exit=0
diff_check_exit=0
ok github.com/MongooseMoo/barn/internal/app 0.013s
ok github.com/MongooseMoo/barn/server 0.034s
```

Only the four assigned literal Go files were formatted. The existing local
checkpoint hunks in server/server.go were preserved. No broad local tests,
build, benchmark, Close-error fault injection or panic test rerun was performed.
Existing panic and startup-order coverage remains for PR CI. No measured
performance claim is made.

## Next step

Return the scoped startup hunks and two regression files for parent review and
independent PR CI. No git action, publication or merge was performed by this
worker.
