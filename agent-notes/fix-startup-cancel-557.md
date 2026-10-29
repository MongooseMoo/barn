# Startup cancellation: issue #557

## Goal

Retain cancellation delivered before or concurrently with Server.Start using the existing lifecycle owner; preserve normal resource cleanup and signal behavior.

## State

Scoped guard repair and focused regression are complete on branch fix/startup-cancel-557. The before-start regression failed against the unchanged production source with `Shutdown before Start discarded application cancellation`. After the repair, the entire focused regression passed: `ok github.com/MongooseMoo/barn/server 2.578s`. Root owns publication and CI monitoring; no PR or remote CI result exists yet.

## Decisions and why

Smallest change: Shutdown ignores a non-running server only when its context is already canceled. A constructed server still has a live context, so its early shutdown message and cancellation must be retained. Start and Shutdown already serialize the running transition with the same mutex. Production estimate is one changed guard plus its comment; no new lifecycle service, field or protocol. Existing Start/mainLoop executes normal resource cleanup for the retained request.

## Surprises

None established beyond the issue's lost pre-start cancellation path.

## Next step

Commit source, focused regression and this note, then return the local branch to root for source inspection, PR publication and CI. Do not push or publish from this worker.

## Checkpoint 1

- Issue #557 and worktree instructions read; startup/shutdown source inspection is underway.
- Root authorized the scoped repair after Q said to execute all filed issues.
- No implementation or tests have run yet; no blocker established.
- Next: finish lifecycle trace and add the minimum repair and focused regression.
- Publication and PR CI remain owned by root.

## Verification

Baseline command: `go test ./server -run '^TestShutdownRetainedAcrossStartup/before_start$' -count=1 -timeout 30s`.

```text
--- FAIL: TestShutdownRetainedAcrossStartup
    --- FAIL: TestShutdownRetainedAcrossStartup/before_start
        startup_cancellation_test.go:49: Shutdown before Start discarded application cancellation
FAIL github.com/MongooseMoo/barn/server
```

Repaired command: `go test ./server -run '^TestShutdownRetainedAcrossStartup$' -count=1 -timeout 30s`.

```text
ok github.com/MongooseMoo/barn/server 2.578s
```

The focused test covers shutdown before Start, concurrent with the Start transition, after readiness, and after completed shutdown. Each live case executes normal Start/mainLoop cleanup and checks canceled input, removed listener records, stopped running state and preservation of the shutdown message against a later request. No app-level signal delivery, full suite, conformance or Elle run is claimed; existing PR CI is root's next verification source.

## PR draft

Title: `server: retain shutdown requests across startup`

Body:

Shutdown discarded application cancellation while a constructed server had not yet entered Start. Retain the request when the existing server context is still live, then let the normal startup/main-loop shutdown path release its resources. Requests after completed shutdown remain a no-op.

Closes #557.

Verification: the focused pre-start regression failed on the original guard, and `go test ./server -run '^TestShutdownRetainedAcrossStartup$' -count=1 -timeout 30s` passed after the repair. Remaining checks run in PR CI.

## Checkpoint 2

- Production change is one existing guard and two comment lines; no lifecycle fields or services were added.
- Focused regression reproduced the original lost cancellation, then passed for pre-start, concurrent-start and running cases.
- Cleanup assertions cover input cancellation, listener removal, running state and late-shutdown no-op behavior.
- Only source, the focused test and this living note belong to the local commit; publication remains with root.
- Next: root inspects, publishes the PR and reads its CI results.

## Checkpoint 3

- Scoped source, regression and note are committed locally on fix/startup-cancel-557.
- Focused regression passed; broader checks remain for PR CI, with no runtime signal-delivery claim.
- No push, PR, review, merge or external publication was performed by this worker.
- This appended checkpoint remains uncommitted as requested by the checkpoint hook.
- Next: root verifies the local branch and publishes the issue-specific PR.
