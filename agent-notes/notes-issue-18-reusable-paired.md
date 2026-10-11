# Issue 18 reusable paired workflow checkpoint

- Scope: conformance-repository half only in `C:\Users\Q\code\.worktrees\moo-conformance-issue-18-reusable` on `agent/issue-18-reusable-paired`; no Barn source changes.
- Exact base: `origin/main` at `acc2718cbc3093c7527448faec0f6c162dac9fe6`.
- Red TDD commit: `54b65ab9548ede95cdd952f3550e0c8740ca88e7` adds strict reusable workflow contract tests; red evidence was 5 failures and 31 existing passes.
- Current implementation files: `.github/workflows/paired-candidates.yml`, `.github/workflows/toast-conformance-core.yml`, `.github/workflows/toast-conformance.yml`, and `tests/test_paired_candidate_workflow.py`.
- Findings: reusable callers are restricted to `barn_repository`, `barn_sha`, and `conformance_sha`; reusable mode forces success/packaged/empty bad identities; `job.workflow_repository` and `job.workflow_sha` identify the called workflow; `$/` keeps the nested core at the called workflow commit; dispatch expected-failure accounting remains separate.
- Verification so far: 40 workflow-focused tests passed; 465 full unit tests passed; 11,584 packaged cases collected; duplicate lint and ruff passed; mypy passed after the locked dev extra was installed; 5 workflow YAML files parsed; 46 shell blocks passed `bash -n`; 7 embedded Python blocks compiled.
- Defects caught during verification: fixed an indented Python heredoc terminator; direct mode simulation then found normalized expected-result fields were shell-local, so the workflow now exports `EXPECTED_RESULT`, `EXPECTED_PHASE`, and `REQUIRED_BAD_IDENTITIES` before the preserved Python validator.
- Current blocker: none. The export fix has not yet been reverified or committed.
- Next action: rerun focused tests and direct reusable/dispatch/unknown-mode simulations, rerun final syntax/static/diff gates, commit the green implementation, and audit the exact two-commit/file scope without pushing or opening a PR.
