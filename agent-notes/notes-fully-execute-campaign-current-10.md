# Current campaign checkpoint 10

## Current findings

- Bootstrap PR #36 merged normally as two-parent commit `19c69adc2787852f89b7f5e0e0d277a97d375a2c`; exact head `7630a8a` is an ancestor and issue #18 remains open.
- Exact-head run 30988415721 passed harness quality, 64-bit/outbound-on, 64-bit/outbound-off, 32-bit/outbound-on, and protected `Full suite` exact-union accounting.
- Live paired expected-success dispatch 30989004525 validated both immutable SHAs and passed the complete reusable Toast admission, proving the default-branch dispatch/reusable path.
- The live paired Barn job then failed closed in candidate quality before Barn execution. Exact cause: tests ran from the caller root, so `tests/test_execution_ledger.py` could not find repository-relative `ci/toast-never-executed.json`; result was 253 passed and one harness failure.
- TDD repair on branch `agent/issue-18-paired-working-directory`: set `working-directory: conformance` for candidate quality and use the same root-relative commands as the ordinary quality job.
- Red proof: new workflow regression failed because `working-directory` was absent.
- Green proof: 10 focused tests, 255 full harness tests, Ruff, mypy across 19 files, actionlint v1.7.12, and diff checks all pass.
- Exact staged repair is two files only: `.github/workflows/paired-candidates.yml` and `tests/test_paired_candidate_workflow.py`, 22 additions and 6 deletions.

## State and blocker

- No known implementation blocker. Independent review of the stable two-file snapshot is pending.
- Issue #18 still lacks the required exact red-before-fix and green-after-fix fixture pair. The first live green attempt exposed the working-directory bug and therefore is not acceptance evidence.
- No project historical Markdown was read.

## Next action

- Obtain independent verdict, commit/push the two-file repair, open a scoped follow-up PR, require complete Toast CI, normal-merge after audit, then rerun the live paired expected-success dispatch. Preserve the strict red-fixture contract; do not reinterpret the mass capability-probe failure as acceptable red evidence.
