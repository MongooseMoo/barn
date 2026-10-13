# Current conformance CI campaign checkpoint

- Scope: make required checks run every packaged conformance case against the pinned Toast oracle while preserving a base-owned trust boundary.
- Issue classifier PR #55 was normal-merged earlier as `9e2795b95784702d91cfaa81f8b0ccce0bf32445`; issue #54 remains open for the activation PR.
- Trusted PR-head reporter bootstrap PR #56 passed quality, all three Toast profiles, and its exact-union ledger: 11,580 packaged, 11,580 executed, zero failures/errors in every profile.
- PR #56 was normal-merged as `aacfe65c253fb9328cf59a3b7316c52243f188d7`; verified exactly two parents, both intended base/head ancestors, and exact `origin/main`.
- The staged workflow activation is committed as `b590d2d` and has now normally merged current `origin/main` into branch `agent/issue-54-staged-ci-workflow`, producing `a427af8`.
- Activation diff owns `.github/workflows/toast-conformance.yml`, `src/moo_conformance/startup_fixtures.py`, `tests/test_paired_candidate_workflow.py`, and `tests/test_startup_fixtures.py` only; the reporter files are inherited from main.
- Current blocker: none. Required next action is rerun full local verification/actionlint on `a427af8`, obtain a fresh independent review of the exact post-merge diff, push/open the activation PR with literal `Closes #54`, and empirically verify the base-owned probe reports success on the exact PR-head SHA before any merge.
- Preserve all unrelated dirty/untracked material. Do not stage, commit, or push this note.
