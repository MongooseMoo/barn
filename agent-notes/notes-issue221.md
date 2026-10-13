# Issue 221 checkpoint

## Findings

- Issue #220 is closed by PR #227, so the fabricated-registry prerequisite is present on `origin/master` at `39dd8f4`.
- `Registry` currently combines dispatch/compiler state with host hooks, mutable handles, cached limits/protected flags, recycle guards, and connection/input state.
- The engine and VM currently pass one `*builtins.Registry` everywhere, so the split requires explicit registry plus session ownership across builtin execution, VM construction, and engine/server wiring.
- The primary checkout has extensive unrelated untracked files; implementation is isolated in `C:\Users\Q\code\barn-issue-221` on `agent/builtins-dispatch-runtime`.

## Current state

- Focused baseline passed before edits: `go test ./builtins ./vm ./engine ./server -count=1`.
- Red test commit: `48e43ab test builtin session ownership`.
- The red test requires a shared dispatch `Registry`, isolated mutable `Session` instances, explicit `NoHost()`, and `Execution` carrying both.
- Production migration is in progress: `Session` exists and mutable builtin receivers/state access are being moved off `Registry`.
- The focused architectural tests now compile and pass after introducing `Session`, `NoHost`, and explicit `Execution.Registry` plus `Execution.Session`.
- Production builtins compile. VM, engine, server, and their tests are at the expected caller-migration frontier; the latest compile-only run reports stale `*Registry` arguments and state-method calls, not a semantic test failure.
- A subsequent `go test ./... -run '^$'` reached compile-only green across the repository after VM, engine, server, CLI, and test callers were migrated to explicit sessions.
- The host cleanup now constructs engine capabilities as one `Host` passed to `NewSession`; server capabilities are assembled as one bundle, installed once with `ConfigureHost`, and checked with `Host.Validate` so missing production capabilities become startup errors.
- The affected behavioral suites passed after the session and host migration: `go test ./builtins ./vm ./engine ./server -count=1`.
- The mechanical cohesion split is applied: signature metadata remains in `signatures.go`, connection/listener builtins are in `connection.go`, and task/system functions have been appended to their cohesive files.
- Repository-wide tests passed after the production changes: `go test ./... -count=1`.
- Focused race runs passed for `builtins`, `vm`, `engine`, and `server`; the explicit engine result was `ok .../engine 303.659s` with exit 0, and server was `ok .../server 1.683s` with exit 0.
- `go vet ./...` and `staticcheck ./...` both exited 0; `git diff --check` exited 0.
- Two earlier output-less engine race wrappers remained CPU-bound alongside the measured run. Their exact same-worktree command lines were verified and only the redundant older processes were stopped; the newest measured run completed normally.

## Blocker

- No external blocker. Remaining work is final diff/scope review, the implementation commit, PR publication, and exact-head CI watching.

## Next action

- Finish final diff/scope review, stage only issue-owned paths, commit the green implementation, publish the PR with `Closes #221`, and watch exact-head CI.
