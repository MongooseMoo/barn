# Current checkpoint

## Fresh check results

- PR #45 exact head `d03abea108555cc7f01cabbec69011e2c3e3b221`, run `31039263498`, failed before Toast execution in every profile and quality job.
- Exact shared failure: trusted main controller rejected candidate field `wait_for_server_exit` while parsing `fixture_provenance/panic_live_waif_anon_roots.yaml`.
- This failure is correct for the current trust boundary: candidate controller Python is not executed as authoritative Toast logic.
- PR #22 exact head `be763f95c7b4cea60bc5f69d1383f6f76c6d3a26`, run `31039279705`, is green with 11,583 packaged / 11,583 executed and zero failures/errors.
- PR #22's green result is numerically complete but not semantic proof of its controller changes: the base controller parsed/executed the three additive cases with the old expectation enforcement.

## Common root cause

The current trusted-controller workflow safely handles data-only changes but has no staged path for harness/schema evolution. Mixed controller plus dependent-data PRs either fail before Toast on new schema or can appear green under old semantics. Candidate Python cannot simply become the authoritative verifier without weakening the security boundary.

## Issue and implementation

- Filed conformance issue #54: `Stage harness and data changes before trusted Toast execution`.
- Acceptance requires mechanical controller/data separation, candidate controller unit/lint/type gates in a non-authoritative job, trusted-main Toast execution, and aggregate fail-closed checks.
- Created clean worktree `C:\Users\Q\code\.worktrees\moo-conformance-issue-54-staged-ci` on branch `agent/issue-54-staged-controller-data-ci` from main `1e61c1d`.
- A worker owns issue #54 implementation in that worktree; no push is authorized yet.

## Required landing order after #54

1. Split PR #22 into controller files first, then its single packaged YAML after the controller is trusted on main.
2. Split PR #45 into controller files first, then fixtures/cases, then the provenance workflow step.
3. Each data stage must receive a fresh managed Toast matrix and exact zero-omission ledger.

## Next action

Review and verify issue #54 implementation, open its PR with `Closes #54`, require fresh managed Toast proof, normal-merge after green and independent review, then stage PRs #22 and #45 in the enforced order.
