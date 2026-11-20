# Current campaign checkpoint 66

## Current state

- Classifier PR #55 is merged as two-parent `9e2795b95784702d91cfaa81f8b0ccce0bf32445`; main push run `31042378172` is fully green.
- The staged issue #54 workflow diff in `moo-conformance-issue-54-staged-workflow` is uncommitted and independently APPROVE at diff hash `2d3e586fe8fa25a0da3cbbcbfba67032c8e714eb`.
- Workflow evidence: 410 full unit tests passed; 65 focused workflow/classifier/fixture tests passed; Ruff and mypy (24 sources) passed; 11,581 tests collected; duplicate baseline exact across 486 YAML and 4,512 tests; YAML, 27 shell blocks, and embedded Python validated.
- Official GitHub behavior requires required checks on the latest PR head/test-merge SHA, while `pull_request_target` built-in job checks attach to the default-branch SHA.
- A separate reporter bootstrap branch/worktree `agent/issue-54-trusted-head-status` / `moo-conformance-issue-54-head-status` now has three explicitly staged files only: reusable trusted reporter, base-owned probe, and three regression tests.
- Reporter bootstrap review is APPROVE: focused tests, actionlint, ShellCheck, permission boundary, injection resistance, failure sentinel, exact Checks API response verification, and rerun behavior passed.
- Unsupported generic conclusion `stale` was removed after review; focused tests/Ruff/diff check are green and a narrow reviewer follow-up is pending.

## Findings

- The reporter has only `checks: write`, performs no checkout/action/artifact/candidate execution, validates the exact lowercase 40-hex head SHA, uses `jq --arg`, first creates a verified failure sentinel, then updates that exact check ID.
- The reporter proves GitHub Actions-app PR-head reporting capability, not immutable workflow provenance; all repository Actions workflows share app ID 15368. The staged workflow's security still comes from base-owned `pull_request_target` code and strict isolation of candidate execution.
- The staged workflow independently re-fetches the open PR and verifies tested head/base before reporting the two literal required contexts; only its isolated reporter has write permission.

## Current blocker

- Live GitHub execution is still required to prove that this repository accepts and associates a Checks API run with the exact PR head SHA. Do not merge workflow activation until the bootstrap is merged normally and its probe is observed on the activation PR head.

## Next action

- Finish narrow re-review, classify/commit/push/open the three-file bootstrap PR, wait for existing required Toast checks, audit and normal-merge, verify two parents/main, then open the staged workflow PR and verify the live probe check attaches to its exact head before proceeding.
