# Issue 220 checkpoint

## Findings and observations

- Issue 220 asks to remove the fabricated `builtins.NewRegistry()` fallbacks from `VM.registryForLimits` and `setAtIndex` so limit checks require the caller-owned registry.
- The main checkout contains extensive unrelated untracked material, so implementation is isolated in `C:\Users\Q\code\barn-issue-220` on branch `agent/registry-required-limit-checks` from `origin/master` at `f394551`.
- Production VM construction paths supply a registry. Nil registries remain in pool seeds and tests, but only limit operations now require them.
- TDD red proof: `go test ./vm -run '^TestLimitOperationsRequireRegistry$' -count=1` failed both the VM opcode and indexed-assignment cases because the old fallbacks accepted nil.
- Commit `8021c20` adds the red architectural contract. Commit `9b9d7eb` removes both fallbacks, replaces all `registryForLimits()` calls with `vm.Builtins`, updates the boolean-map test to supply a registry, and adds non-default string/list/map limit coverage.
- Focused green proof: `go test ./vm -run '^(TestLimitOperationsRequireRegistry|TestLimitOperationsUseProvidedRegistry|TestBooleanMapKeysSurviveBothInsertionOrders)$' -count=1` passed.
- `gofmt -l .` was empty, `go vet ./...` passed, and `staticcheck ./...` passed in the current turn.
- `go test -race ./vm` and the first `go test ./...` run exposed an existing test-only nil-registry construction in `TestMapRangeAssignmentUsesPendingListValueByteLimit`; the implementation commit now updates that test to supply a real registry, like production.
- Exact-head verification after that update passed: clean `gofmt -l .`, `go vet ./...`, `go build ./...`, `staticcheck ./...`, `go test ./...`, and `go test -race ./vm`.

## Current state

- Worktree is two commits ahead of `origin/master`; implementation commit `45c6808` is cleanly separated from red test commit `8021c20`.
- Branch `agent/registry-required-limit-checks` is pushed and ready-for-review PR #227 is open with the approved title/body and `Closes #220`.
- PR head, local head, and CI run `31978448468` all resolve to `45c6808186b325e27069487b42075a3a596a786e`.
- Both required checks completed successfully on that exact head: `Build & unit tests` and `MOO conformance suite`.
- The checkpoint file itself is intentionally untracked in the main checkout and must remain uncommitted.

## Blocker

- No blocker remains for the requested publish-and-watch scope. The PR remains open and unmerged.

## Next action

- Await review or an explicit merge request. If merging is later requested, re-audit files, commits, issue scope, required checks, closing phrase, and use a normal merge commit.
