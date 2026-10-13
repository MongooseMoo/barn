# Listener lifecycle regression tests

## Current state

- Assigned scope: tests only for Barn issue #100 in `C:\Users\Q\code\.worktrees\barn-issue-100-listener-zero`.
- Target implementation: PR #101 commit `4fe05db`.
- Required coverage: runtime descriptor-0 WebSocket lifecycle plus TLS duplicate/remove/socket-release behavior.
- Production files must remain unchanged; no commits or pushes are authorized.

## Findings and observations

- Added test-only coverage in `server/connection_manager_test.go` for a real runtime WebSocket port-0 listener: exact descriptor/path metadata, live HTTP listener readiness, duplicate descriptor rejection, exact descriptor removal, empty listener inventory, and rebound of the released OS socket.
- Extended `server/tls_listener_test.go` to cover real TLS port-0 duplicate rejection, exact descriptor removal, empty listener inventory, and rebound of the released OS socket.
- Production code remains unchanged; only the two assigned `_test.go` files are modified.
- Focused PR #101 proof is green: `go test ./server -run 'TestRuntimeWebSocketListenerZeroLifecycle|TestAddTLSListenerReportsMetadata' -count=1` passed, followed by `-count=10` passing.
- `gofmt` and `git diff --check` pass.
- Refactored TLS coverage into the standalone `TestRuntimeTLSListenerZeroLifecycle`, leaving the pre-existing PR test unchanged.
- Exact red proof against detached `origin/master` (`aa82abf`) failed as intended: both WebSocket and TLS returned/exposed nonzero bound ports, accepted a duplicate descriptor-0 request, and rejected removal by descriptor 0.
- The WebSocket test, TLS test, and their two shared helpers were byte-for-byte matched between the red and green worktrees before execution.
- Final PR #101 proof is green: the two exact tests passed 10 consecutive runs, then the complete `./server` package passed once.
- Final diff is test-only: 107 added lines in `server/connection_manager_test.go` and 65 added lines in `server/tls_listener_test.go`; no production file was modified.

## Blocker

- None currently.

## Next action

- The validated disposable red worktree has been removed. Hand off the uncommitted test-only diff and red/green evidence to the parent agent.
