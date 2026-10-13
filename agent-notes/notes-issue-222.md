# Issue 222 checkpoint

## Findings

- Issue #222 asks to move `ActivationFrame` below `task` so `types.Result.CallStack` can be `[]types.ActivationFrame` instead of `interface{}`.
- The adjacent `BytecodeVMValue`, `SetBytecodeVM`, and fork-body interface fields are explicitly out of scope.
- The direct ownership migration is feasible without a `task.ActivationFrame` compatibility alias.
- The primary checkout contains extensive unrelated untracked files, so work is isolated in `C:\Users\Q\code\barn-issue-222` on `agent/issue-222-typed-call-stack` from `origin/master` at `90e7bd1`.

## TDD state

- Red commit `ef5cd7d` adds `TestResultCallStackHasStaticActivationFrameType`.
- The focused red output was `Result.CallStack type = interface {}, want []types.ActivationFrame`.
- The frame struct and its methods have been moved to `types/activation_frame.go`; task, engine, server, VM, database-format, builtin, and test references are being migrated directly to `types.ActivationFrame`.
- The focused type contract is green.

## Current blocker

- No implementation blocker.
- PR #229's first `Build & unit tests` job failed at `test -z "$(gofmt -l .)"`. Local reproduction named only `types/activation_frame.go`: the file had been untracked when the changed-file formatting list was built, so it was omitted from that pass.
- The user approved the one-file formatting correction. Commit `a800714` formats only `types/activation_frame.go` and is pushed.
- Replacement CI run `31985507509` is queued against exact PR head `a8007144578aaf2fc59b37d9623d6b4683b344f7`.

## Next action

- Watch replacement `Build & unit tests` and `MOO conformance suite` checks through terminal state, then re-fetch PR #229 to verify the exact head and final check rollup. Do not merge without a separate user request.

## Published state

- Draft PR: https://github.com/MongooseMoo/barn/pull/229
- Commits: `ef5cd7d` red contract, `8b474d3` green ownership migration, `a800714` approved formatting correction.
- PR body: `Closes #222`.
- After the formatting correction, `gofmt -l .` returned no files, `git diff --check` passed, `go test ./... -count=1` completed, `go vet ./...` passed, and `staticcheck ./...` passed locally.

This file is a local checkpoint only. Do not stage, commit, or push it.
