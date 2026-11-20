# Trusted PR workflow checkpoint

- Scope: only `.github/workflows/toast-conformance.yml` and `tests/test_paired_candidate_workflow.py` in `C:\Users\Q\code\.worktrees\moo-conformance-trusted-pr-check`.
- Current finding: PR runtime checks must be owned and executed by an immutable trusted base controller; candidate checkout remains credentialless data only.
- Required workflow shape: `pull_request_target` plus existing push/workflow_call; trusted controller checked out at `base.sha || workflow_sha`; Toast profile generation, admission, conformance, paired inventory, and ledger run from trusted controller with explicit candidate paths.
- Required ledger shape: exact `--inventory`; no baseline compatibility path.
- Blocker: none known yet; implementation and verification have not started.
- Next action: inspect current workflow/tests and paired-candidate source precedent without reading historical Markdown, then patch assigned files and run focused/full tests and Ruff.
- This file is an uncommitted campaign checkpoint and must never be staged, committed, or pushed.

## Implementation and proof update

- Workflow now uses base-owned `pull_request_target`, preserves `push` and `workflow_call`, has explicit `contents: read`, scopes concurrency by PR number/candidate identity, and cancels only matching PR-target runs.
- Every job checks out canonical `MongooseMoo/moo-conformance-tests` at immutable `${{ github.event.pull_request.base.sha || github.workflow_sha }}` as `trusted-controller` with credentials disabled.
- Fork candidates are checked out as credentialless data from `${{ github.event.pull_request.head.repo.full_name || github.repository }}` at `${{ inputs.conformance_sha || github.event.pull_request.head.sha || github.sha }}`.
- No job syncs, imports, or executes the candidate Python project. Trusted code owns unit/static gates, candidate YAML inventory/collection/duplicate lint, Toast profile generation, admission, conformance, paired inventory, and execution ledger.
- Runtime receives explicit candidate root, suite, primary DB, and DB fixture directory. Startup checksum uses the trusted controller manifest against candidate fixture data.
- Final ledger runs from trusted controller against a freshly trusted `paired_inventory` output via exact `--inventory`; its command contains no legacy skip-baseline path.
- Focused workflow tests: 21 passed. Full unit suite: 372 passed. Ruff: clean. Mypy: clean. `git diff --check`: clean.
- Trusted candidate-data proof: schema-v2 inventory validated 11,580 trusted and 11,580 candidate identities with zero additive drift; explicit candidate collection found 11,581 items including admission; duplicate lint scanned 486 YAML files/4,512 source tests and matched the trusted duplicate baseline.
- Current blocker: none. Remaining action: complete the exact workflow security review, inspect final status/diff, and report changes and `pull_request_target` residual risk to the parent without committing or pushing.
