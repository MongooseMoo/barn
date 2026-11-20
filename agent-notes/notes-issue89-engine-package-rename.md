# Issue #89 engine package rename

## Current findings

- Issue #83 is merged by PR #118 as two-parent merge `a199d0bc6253cedb455a93dac8d1de733f5d73df`; issue #83 is closed and both required CI checks passed.
- Issue #89 is open and explicitly requires `scheduler` to become `engine`, no `runtime` import alias, no compatibility facade, and stutter-free public names.
- The package owns much more than scheduling: eval, verb calls, task execution and lifecycle, MVCC retry, WAIF lifecycle, and GC coordination.
- The accurate concrete API is therefore `engine.Runtime`, with `NewRuntime` and `NewRuntimeWithOptions`; retaining `Scheduler` would preserve the semantic mismatch.
- `gopls` v0.17.1 and `gorename` are installed. Semantic identifier renames will use `gopls`; the directory/import-path move will remain a single structural Git rename.
- Scope is current code, tests, active scripts/configuration, and code comments. Historical reports, plans, experiments, investigations, and prior notes are evidence and will not be rewritten.

## Current state

- Local `master` is at the verified #83 merge and has no tracked changes.
- No #89 branch, edits, commit, push, PR, issue mutation, or merge has occurred yet.
- Unrelated untracked material, including this note and the #92/#83 notes, must remain untracked and unstaged.

## Blocker

- None.

## Next action

1. Create `agent/issue-89-engine-package-rename` from `a199d0b`.
2. Inventory tracked current references and use `gopls rename -w` for `Scheduler`, `NewScheduler`, and `NewSchedulerWithOptions`.
3. Rename package declarations/import identity to `engine`, move `scheduler/` to `engine/`, and update active path references without a facade.
4. Prove zero current Go/import/path remnants, then run gofmt, vet, full tests, race-sensitive coverage, and documented managed conformance.
5. Commit only intended paths, open a ready PR with exact `Closes #89`, watch CI, normal-merge only when green, verify two parents and closure, then clean the feature branch.

## Checkpoint 2026-08-08 after semantic rename

- Branch `agent/issue-89-engine-package-rename` was created from verified merge `a199d0b`.
- `gopls rename -w` renamed exported type `Scheduler` to `Runtime`, constructors to `NewRuntime` and `NewRuntimeWithOptions`, all receiver/call-site references, server import aliases, the private worker-count constructor, runtime tick/helper names, and test identifiers containing the old component name.
- Renaming the package declaration with `gopls` wrote every package/import edit but then returned a Windows directory-handle error (`getFile ... scheduler: Incorrect function`). Inspection proved all 32 package files became `package engine` and all server import paths became `/engine`; no partial package edit remained.
- The validated `scheduler/` directory was moved to `engine/`, and `engine/scheduler.go` was renamed to `engine/runtime.go`. No compatibility package or facade exists.
- All server imports now use the actual `engine` package name, never the old `runtime` alias. `Server.scheduler` was semantically renamed to `Server.runtime`.
- Focused `go test ./engine ./server -count=1` passes after the structural changes.
- Test locals that held an `engine.Runtime` in seven files were mechanically renamed from `scheduler` to `rt` with Go's `gofmt -r` rewrite facility.
- The component-ownership documentation pass now says engine/runtime for broad execution ownership, while genuinely scheduling-specific concepts such as scheduler cycles, queue ordering, and batch scheduling remain intentionally named as such.
- No commit, push, PR, issue mutation, or merge has occurred for #89. This note remains untracked and must never be staged.

## Current blocker

- None. The `gopls` package-rename exit was a post-write Windows tooling error, not a partial result; focused compilation verifies the resulting package graph.

## Immediate next action

1. Finish code/test comment and identifier cleanup, including active `./scheduler` commands and stale `scheduler.go` path references.
2. Prove no current Go/import/config path points at the old package, then run full format/list/build/vet/test/race gates.
3. Run the documented managed conformance command, audit the final rename-only scope, and proceed through the #89 PR/CI/normal-merge gates.

## Final pre-merge checkpoint

- Commit `ba753dfc4c8681fdcc5cf8d2251a6936f08b8d31` contains the complete rename/refactor pass: 64 intended files, with all 32 former `scheduler/` files recognized as Git renames.
- The old directory, import path, `Scheduler` type, `NewScheduler` constructors, private constructor, server `runtime` import alias, and `Server.scheduler` field are absent from current code.
- Active API documentation reports package `github.com/MongooseMoo/barn/engine` with `Runtime`, `NewRuntime`, and `NewRuntimeWithOptions`. Historical reports/plans/notes were not rewritten.
- Local verification passes: `go list ./...`, `go test ./... -count=1`, `go build ./...`, `go vet ./...`, tracked-file `gofmt -l`, and `go test -race ./engine ./server -count=1` (`engine` 237.824s, `server` 1.883s).
- PR #119 is ready, mergeable, has one commit, 64 expected files, and exactly one literal `Closes #89`.
- Required CI is green: Build & unit tests passed in 53 seconds; MOO conformance suite passed in 4 minutes 33 seconds.
- The exact authorized merge command is `gh pr merge 119 --merge`. It has not yet been run.

## Next action after this checkpoint

1. Re-audit PR state, files, commit, checks, closing phrase, and tracked cleanliness immediately before merge.
2. Run the exact normal-merge command, verify a two-parent merge commit and issue #89 closure.
3. Fast-forward local `master`, remove only the verified #89 feature branch locally/remotely, and perform the final #92/#83/#89 closure and cleanliness audit.
