# Issue 54 push fix checkpoint

- Finding: `full-suite` depended on the always-running aggregate `quality` but lacked its own explicit job condition, allowing skipped-ancestor propagation from the intentionally skipped `candidate-quality` lane on neutral push runs.
- Red evidence: the focused regression failed with `KeyError: 'if'`.
- Green state: adding the exact fail-closed `always()` condition requiring successful classification and aggregate quality made the focused regression pass.
- Scope: only `.github/workflows/toast-conformance.yml` and `tests/test_paired_candidate_workflow.py` in the dedicated push-fix worktree are intended for the commit. This note is outside that worktree and must remain uncommitted.
- Blocker: none currently.
- Next action: collect the running focused/full/Ruff/mypy/actionlint results, verify the exact diff, and commit the two owned files only.
