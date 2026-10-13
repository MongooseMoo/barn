# PR #226 review fixes

## Current state

- Branch: `agent/store-txn-non-optional`
- PR: #226, draft, head `2ce3f2c5fdfdd53f08ab3972839aaa972dc64686`
- Existing tracked worktree changes: none; many unrelated untracked files are present and must remain untouched.
- The supplied two P2 findings are not live GitHub review threads; the thread-aware fetch returned no reviews or threads.

## Findings

1. `engine/runtime.go` routes any execution carrying a task through `CallVerbInContext`. Deferred anonymous GC can carry a completed task while its recycle context has the store-owned direct transaction, so it incorrectly inherits task caller/context instead of using sweep-owned standalone hook semantics.
2. `engine/task_runtime.go` calls `captureTaskRetryState` before `runTask` assigns the runtime store to the task context. A task created with public `task.NewTaskFull` and queued with `Runtime.QueueTask` therefore reaches retry capture with `Context.Store == nil`. On the current head this does not panic because `(*Store).DirectTxn()` explicitly handles a nil receiver and returns nil; the initial public-queue execution test passes. The dependency-ordering gap is still real, and the queue boundary can populate only missing dependencies before retry capture without overwriting an existing task transaction.

## Red evidence

- `go test ./engine -run 'TestDeferredAnonymousGCRecycleUsesStandaloneCallerWithCompletedTask|TestQueueTaskInitializesNewTaskFullContextBeforeRetryCapture' -count=1`
  - recycle regression fails with `deferred :recycle caller = #0 (E_NONE), want #-1`.
- `go test ./engine -run '^TestQueueTaskInitializesNewTaskFullContextBeforeRetryCapture$' -count=1 -v`
  - passes on the current head, disproving the review comment's claimed panic mechanism.

## Implemented and verified

- Commit `1e8fc62`: direct transactions no longer select `CallVerbInContext`; the deferred anonymous-GC regression observes standalone `caller == #-1`.
- Commit `d89e5d6`: `QueueTask` populates runtime dependencies only when both the context store and transaction are missing; existing task transactions are not overwritten.
- Focused tests pass normally and under `-race`.
- `go test ./engine -count=1`, `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `staticcheck ./...` exited successfully.
- Exact-head CI run `31973905634` passed `Build & unit tests` and `MOO conformance suite` for `d89e5d6a3ad0d77e98b724406e6b3bcaeb46fa64`.
- PR #226 is ready, mergeable, and clean. Its four commits and 111-file diff match issue #219's non-optional StoreTxn scope plus the two review fixes. The body contains `Closes #219.`
- Repository merge settings: merge commits enabled; squash and rebase merges disabled.

## Next action

- Run exactly `gh pr merge 226 --merge`.
- Re-fetch PR #226 and issue #219, verify the merge commit has two parents, and confirm issue closure.

## Blocker

- None.
