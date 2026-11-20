# Current campaign checkpoint 64

## Current state

- `moo-conformance-tests` issue #54 tracks staged controller/data CI.
- Classifier bootstrap PR #55 merged normally as `9e2795b95784702d91cfaa81f8b0ccce0bf32445` with parents `1e61c1dedb515387874fac5f961a7df54bca4786` and `69039ba5ae3618e86da4010124f76895b8dc80fb`.
- PR #55 run `31041805065` passed trusted quality, all three pinned Toast profiles, and the exact-union `Full suite` gate. Ledger: 11,580 packaged, 11,580 executed, zero failures/errors; profile skips were 30, 45, and 49 with exact union coverage.
- Issue #54 remains open intentionally for the workflow activation stage.
- Main push run `31042378172` is in progress for merge `9e2795b`.
- Workflow activation branch `agent/issue-54-staged-ci-workflow` exists at exact base `9e2795b` in `C:\Users\Q\code\.worktrees\moo-conformance-issue-54-staged-workflow`.
- Worker `/root/implement_issue54_staged_workflow` owns only `.github/workflows/toast-conformance.yml` and `tests/test_paired_candidate_workflow.py`; no commit or push is authorized.

## Findings

- PR validation must be base-owned with `pull_request_target`; candidate controller code may run only in a credentialless/no-secrets quality job and may never produce authoritative Toast evidence.
- The trusted classifier rejects mixed/unknown changes, non-top-level roots, and every index mode except `100644`; `_exec_fixtures/` is controller-owned.
- All admitted modes and main pushes must still run the complete suite against all pinned Toast profiles and feed the exact zero-omission ledger.
- Controller-mode Toast must select trusted-base data, because controller changes can include host executable fixtures. Data-mode Toast must select candidate data.

## Open design question

- Candidate startup fixture changes currently self-update `startup-fixtures.sha256`, while the workflow verifies with the trusted-base checksum. The staged data lane must preserve a non-self-attesting trust rule without making legitimate fixture additions impossible. Inspect current source behavior before accepting the worker implementation.

## Next action

- Inspect current paired-inventory/startup-fixture source, resolve the trusted checksum policy, review the worker diff, run red/green workflow tests plus full gates, obtain independent adversarial review, then open a PR with `Closes #54` only if the main classifier run and new staged workflow checks are fully green.
