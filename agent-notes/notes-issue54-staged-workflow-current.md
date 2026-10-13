# Issue 54 staged workflow current checkpoint

- Worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-54-staged-workflow`
- Exact base: `9e2795b95784702d91cfaa81f8b0ccce0bf32445`.
- Owned edits only: `.github/workflows/toast-conformance.yml` and `tests/test_paired_candidate_workflow.py`; no commit or push.
- Current design uses `pull_request_target`, trusted exact base and candidate exact head checkouts, the trusted `change_boundary` classifier, controller/data/neutral lanes, candidate controller quality isolated from authoritative Toast evidence, trusted quality/data validation, all three Toast profiles, and the exact execution ledger.
- Controller and neutral modes select trusted-base data; data mode selects candidate data. Candidate checkouts use `persist-credentials: false` and the checkout-v7-required `allow-unsafe-pr-checkout: true`; trusted/oracle checkouts do not opt in.
- Candidate quality has only `contents: read`, no secret/token environment, and no authoritative artifact/evidence production.
- Trusted `moo_conformance.startup_fixtures` validation runs fail-fast in trusted quality and again per Toast profile; JSON evidence is uploaded.
- Local evidence: focused fixture/classifier/workflow suite 63 passed; full unit suite 408 passed; Ruff clean; mypy clean across 24 sources; diff check clean.
- Independent review confirmed that `pull_request_target` built-in jobs attach to the base/default-branch SHA and cannot satisfy the two app-bound required contexts on the PR head. It also confirmed checkout v7 needs `allow-unsafe-pr-checkout: true` on exact candidate checkouts under `pull_request_target`; this opt-in is now present only on candidate tree/controller/data checkouts.
- Current reporter design is a single base-owned PR-only final job with only `checks: write` and `pull-requests: read`. It has no checkout, artifact download, or candidate execution. It re-fetches the PR, verifies it is still open with the exact tested head/base SHAs and base repository, then creates completed GitHub Actions check runs on the exact head SHA with the two literal required names, mapping only trusted aggregate result strings to success/failure.
- Candidate quality retains only `contents: read`; no candidate-executing job can write statuses/checks or receive a token environment.
- Next action: replace superseded status-reporter regression assertions with exact final Checks API reporter assertions, rerun focused gates, obtain reviewer approval, and return the uncommitted owned-file diff to root.
