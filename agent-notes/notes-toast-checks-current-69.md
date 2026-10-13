# Current Toast checks repair checkpoint

- Current conformance main: `09056a9daea918b0b851d81f66c7025be38e7985`.
- Managed paired dispatch run `31050391689` ended in `startup_failure` with zero jobs and no logs or annotations.
- The direct main Toast matrix is green and executed the full packaged inventory in all three required profiles; the defect is the paired reusable-workflow startup path.
- Confirmed contract mismatch: `.github/workflows/paired-candidates.yml` calls `.github/workflows/toast-conformance.yml` with read-only permissions, while the called workflow contains a PR-only reporter job requesting `checks: write`. GitHub documents that a called reusable workflow cannot elevate permissions passed by its caller.
- Required architecture: preserve the paired dispatch as read-only. Separate the read-only reusable Toast core from the base-owned PR-head reporter wrapper instead of granting the paired candidate workflow write authority.
- TDD red proof: focused `tests/test_paired_candidate_workflow.py` now has five expected failures because the new wrapper and reusable result contract do not exist yet; 26 other tests pass.
- Files currently edited only in the isolated conformance worktree: `tests/test_paired_candidate_workflow.py`.
- Next action: change `toast-conformance.yml` into the read-only reusable core with explicit quality/full-suite outputs, add `toast-checks.yml` as the pull-request/push wrapper and sole checks writer, run focused/full gates, open a narrow PR, then re-dispatch the exact managed Barn/conformance pair and require real Toast/admission/verdict jobs before closing issue 43.
