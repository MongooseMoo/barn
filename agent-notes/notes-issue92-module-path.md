# Issue #92 canonical module path

## Current findings

- The tracked `master` worktree is clean; the checkout contains extensive unrelated untracked material that must remain untouched.
- Issue #92 is open and requires `go.mod` to declare `github.com/MongooseMoo/barn`, all tracked Go imports to use that prefix, `go list -m` to report it, and Go/build/CI verification to stay green.
- Issue #89 is the only other open naming/refactor ticket, but it explicitly follows issue #83's per-engine task-manager ownership. The agreed sequence is #92, then #83, then #89 as separate PRs.
- Baseline `gofmt -l` and `go vet ./...` are clean. Baseline `go list -m` reports `barn`.
- `gopls` and `gorename` are installed, but neither is appropriate for changing a module identity. Use `go mod edit -module=github.com/MongooseMoo/barn`, mechanically rewrite exact tracked `"barn/..."` imports, then `go mod tidy`.
- GitHub CLI is installed and authenticated. The default branch is `master`. PRs must use normal merge commits, include exact closing phrases, wait for required CI, and be verified as two-parent merges.

## Current state

- Branch `agent/issue-92-canonical-module-path` was created from `master`/`origin/master` at `b3d730e8bd3579517124fd011126f0d514547ce2`.
- `go mod edit '-module=github.com/MongooseMoo/barn'` updated the module identity.
- 690 old imports in 276 tracked Go files were migrated. Three remaining Go-source references in a compiler comment, a store comment, and the `cmd/barn_logs` synthetic stack fixture were migrated explicitly.
- The first full test run exposed the incomplete synthetic stack fixture. After correction, its focused test passed 20 consecutive runs and `go test ./... -count=1` passed all packages.
- `go list -m`, `go list ./...`, `go mod verify`, `gofmt`, `go build ./...`, and `go vet ./...` are green. No old noncanonical `barn/...` reference remains in tracked Go source.
- Commit `179878a5075078aa82e9bca5de6c71bc20746a1f` was pushed in PR #117 with exactly one `Closes #92` line.
- CI passed: Build & unit tests in 57 seconds; managed MOO conformance in 4 minutes 34 seconds.
- PR #117 was merged with `gh pr merge 117 --merge` as merge commit `1e0f7d06c10b21d3481a104912ca618b21d88e24`.
- The merge commit has exactly two parents: base `b3d730e8bd3579517124fd011126f0d514547ce2` and feature `179878a5075078aa82e9bca5de6c71bc20746a1f`.
- Issue #92 is closed and `origin/master` contains the merge.
- Open PR #116 is unrelated and must not be imported into this work.
- This note is intentionally untracked and must not be staged or committed.

## Blocker

- None for issue #92.

## Next action

1. Return to clean `master`, remove the merged feature branch, then start issue #83.
