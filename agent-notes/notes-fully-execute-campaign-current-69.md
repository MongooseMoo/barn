# Current campaign checkpoint 69

- Assignment: read-only analyst review of the uncommitted Toast workflow split in `C:/Users/Q/code/.worktrees/moo-conformance-issue-43-admission`.
- Scope: GitHub Actions caller/callee permissions, failure outputs, concurrency, exact PR-head reporting, candidate trust boundaries, and three-profile packaged-test execution.
- Constraint: do not read historical repository Markdown and do not modify the reviewed worktree.
- Current state: review not yet started; this checkpoint is mandated before further tool use.
- Blocker: none known.
- Next action: read the required gauntlet role protocol, inspect the current diff and relevant workflow/tests, then report findings without fixes.

## Review result

- Blocker: on `push`, the wrapper and reusable core resolve the same concurrency group because a called workflow inherits the caller's `github.workflow` and the core input equals `github.sha`. The caller can contend with its own reusable call.
- Additional concurrency defect: paired dispatches sharing one conformance SHA share a core concurrency group. With the default single pending slot, later dispatches can cancel earlier pending Barn-pair validations even though `cancel-in-progress` is false.
- Permission boundary: core and both callers are `contents: read`; only the base-owned PR-head reporter has `checks: write` and `pull-requests: read`.
- Reporting: the reporter re-fetches the PR and verifies exact base and head SHAs; non-success or missing core outputs map to failure.
- Execution preservation: all six original execution jobs are structurally identical in the new core; the three-profile matrix, per-profile full packaged command, and exact per-profile ledger surface remain intact.
- Verification: 31 focused workflow tests passed; actionlint v1.7.7 passed all three affected workflows; `git diff --check` passed.
- State: findings sent to the parent agent; no reviewed worktree files were edited by this analyst.
- Next action: fix and regression-test distinct caller/callee concurrency identities, decide how paired dispatches avoid dropped pending runs, then obtain live PR-target, main-push, and repository-dispatch proof including failed-output propagation.
