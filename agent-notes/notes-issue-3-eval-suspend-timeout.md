# Issue #3: eval suspend timeout

## 2026-07-30 terminal CI checkpoint

- PR #80 CI run `30543763680` is terminal green.
- `Build & unit tests` passed in 58 seconds.
- `MOO conformance suite` passed in 4 minutes 41 seconds.
- No checks were rerun.
- PR #80 was not merged.
- Next action: final read-only audit of PR state/head/base/diff and preservation of the main and temporary worktrees, then report ready for independent review.

## 2026-07-30 publication checkpoint

- Committed the exact two-file kept slice as `2935466` (`Fix eval suspend timeout result`).
- Verified the committed diff against `origin/master` contains only modified `scheduler/eval.go` and added `scheduler/eval_timeout_test.go`.
- Preserved managed receipts under untracked `reports/runs/`; they were not staged or removed.
- Pushed `agent/issue-3-eval-suspend-timeout`.
- Opened ready PR #80: `https://github.com/MongooseMoo/barn/pull/80`.
- Verified PR #80 is open, non-draft, targets `master` at base `eb05220`, points to head `2935466`, contains one commit and exactly the intended two files, and its body begins with exact `Closes #3`.
- CI run `30543763680` has `Build & unit tests` and `MOO conformance suite` queued.
- Current blocker: none.
- Next action: watch every configured check to terminal state; do not rerun unchanged failures and do not merge.

## 2026-07-30 implementation and local-gate checkpoint

- Added focused regression `TestEvalBareSuspendTimeoutReturnsInvalidArgument`.
- The initial invocation had an unused test import and was not counted as red evidence.
- Valid behavioral red: after the existing 10.01-second production deadline, `EvalCommandOutput` returned `[]string{"{1, 0}"}` instead of `"{2, {E_INVARG, \"Invalid argument\", 0}}"`.
- Applied the smallest direct fix in `scheduler/eval.go`: label the existing yielded-control loop and use `break resumeLoop` on the bare-suspend timeout. No helper, interface, adapter, clock shim, or arbitrary test sleep was added.
- Focused regression passed: `ok barn/scheduler 11.051s`.
- Relevant targeted scheduler package passed: `ok barn/scheduler 16.866s`.
- Full `go test ./... -count=1` passed: 22 tested packages green; 8 packages have no test files.
- `gofmt -d scheduler/eval.go scheduler/eval_timeout_test.go` produced no output.
- `git diff --check` passed.
- Exact managed conformance command passed: 11,451 passed, 128 skipped, 0 failed in 291.91 seconds.
- Managed receipts are preserved at `C:\Users\Q\code\barn-issue-3-eval-suspend-20260730\reports\runs\20260730_063654`.
- Current intended diff is exactly `scheduler/eval.go` and `scheduler/eval_timeout_test.go`.
- Current blocker: none.
- Next action: audit and exact-path stage only those two files, commit the single kept slice, push the branch, open a ready PR with exact `Closes #3`, verify metadata/diff, and wait for every configured CI check to become terminal without merging.

## 2026-07-30 validation checkpoint

- Live issue #3 is open: `scheduler: eval suspend()-timeout E_INVARG is discarded by ineffective break (SA4011)`.
- Verified new temporary path `C:\Users\Q\code\barn-issue-3-eval-suspend-20260730` was absent before creation.
- Fetched and verified `origin/master` at green merge commit `eb05220d8c9bced36250b2dc27ebe03595b5f044`.
- Created a clean detached worktree at that exact commit; main checkout was not switched, stashed, reset, cleaned, or edited.
- Current code confirms the issue premise: `scheduler/eval.go` assigns an `E_INVARG` result after the 10-second bare-`suspend()` deadline, but the unlabeled `break` exits only the inner switch and unconditional `bcVM.Resume()` overwrites the result.
- Existing scheduler tests cover suspend lifecycle but do not cover this exact `EvalCommandOutput` timeout boundary.
- Current local Staticcheck cannot reproduce SA4011 because the installed binary was built with Go 1.23.4 while the module requires Go 1.24.6; the source-level control-flow defect remains directly present and valid.
- Managed conformance cannot express the exact internal adapter contract: the 10-second synthetic deadline exists only inside Barn's `EvalCommandOutput`; Toast has no corresponding injectable adapter timeout, so a row would observe transport non-response rather than the discarded `E_INVARG` result.
- Authorized fallback: deterministic Go regression calling `EvalCommandOutput` with bare `suspend()` and relying on the existing production deadline. The test requires no arbitrary sleeps, clock helper, interface, or adapter.
- Created branch `agent/issue-3-eval-suspend-timeout` only after validation.
- Current blocker: none.
- Next action: add only the focused Go regression, capture the required red result, then apply the smallest direct control-flow fix and replay the required gates.
