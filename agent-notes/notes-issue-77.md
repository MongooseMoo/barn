# Issue 77 delivery notes

## Current findings

- Issue #77 is open and describes failed coarse commits retaining staged writes, scheduler recommit, and non-idempotent contents additions.
- Existing PR #116 (`agent/fix-29-45-77-coarse-atomic`) addresses the common root shared by issues #29, #45, and #77. Its body contains the literal `Closes #77`.
- PR #116 preserves TDD commit boundaries: `257b9d3` adds regression tests first; `aa33a86` implements the fix; `09992c8` merges the then-current master.
- The previous GitHub Actions run `31119775938` failed during the GitHub `Set up job` phase before checkout and exposes no actionable job log. The conformance job was cancelled.
- Current `origin/master` is `2fac7db`, eight commits ahead of PR #116's merge base, including the scheduler-to-engine and module-path changes.

## Reproduced red evidence

At detached test-only commit `257b9d3`:

- `TestStoreTxnCoarsePreflightIsAtomicBeforeClockOrPublication` fails because the clock advances and private state is published.
- `TestStoreTxnTerminalPreflightFailureAndValidationConflictRemainDistinct` fails because the terminal transaction remains recommittable.
- `TestGenericCoarseBuiltinPropagatesFailedLegacyTopologyFlush` fails because private name/location reach live state and writes remain staged.
- `TestApplyContentsDeltasAddIsIdempotentAcrossRetry` fails with first apply `[1 1]` and retry `[1 1 1]` instead of `[1]`.
- `TestRunTaskDoesNotRecommitAfterTerminalCommitFailure` fails with three commit attempts instead of one.

## Worktree state

- Primary checkout is on unrelated clean tracked branch `agent/issue-53-exec-env` with extensive pre-existing untracked material.
- Feature worktree: `C:/Users/Q/code/.worktrees/barn-fix-29-45-77-coarse-atomic`, clean at `09992c8`.
- Red-proof worktree: `C:/Users/Q/code/.worktrees/barn-issue77-red`, detached at `257b9d3`.
- This notes file must remain uncommitted and must not be staged or pushed.

## Blocker and next action

- No external blocker. The stale-base conflict has been resolved.
- Current feature head is merge commit `90623754391801c6906fd71257154a4b998cf198`, with parents `09992c8` and current master `2fac7db`.
- Resolution preserved the issue fix while adopting the scheduler-to-engine rename, `NewRuntime`, and canonical `github.com/MongooseMoo/barn` imports.
- Current PR diff against `origin/master` is limited to 10 issue paths: five production files and five regression-test files under `builtins`, `db/store`, and `engine`.

## Green evidence after current-master merge

- All seven focused issue regressions passed once.
- All seven focused issue regressions passed 20 repetitions.
- Focused `-race` regressions passed.
- `go test ./... -count=1 -timeout=10m` passed.
- `go test -race ./... -count=1 -timeout=20m` passed; the engine package completed in 235.702 seconds.
- Scoped `gofmt -l`, `go vet ./...`, `go build ./...`, and `git diff --check origin/master...HEAD` passed.

## Next action

- Completed. PR #116 passed Build & unit tests in 56 seconds and MOO conformance in 4m32s; conformance reported 11,177 passed, 410 skipped, and 0 failed.
- Final audit confirmed current base `2fac7db`, head `9062375`, the 10-file issue diff, preserved red/green commit boundaries, and literal recognized closing references for #29, #45, and #77.
- Merged with the exact normal-merge command `gh pr merge 116 --merge`.
- GitHub merge commit `34de084f4032ad9e861b4c1a79542f3767f4e530` has exactly two parents: `2fac7db` and `9062375`.
- Issues #29, #45, and #77 closed at `2026-08-08T21:28:35Z`.
- No blocker or remaining delivery action.
