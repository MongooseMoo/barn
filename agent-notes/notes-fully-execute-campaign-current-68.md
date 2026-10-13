# Current conformance CI campaign checkpoint

- Scope: required checks must run the complete packaged conformance inventory against the pinned Toast oracle through the trusted staged workflow.
- PR #57 landed as normal merge `cc3d0cd8f2f8b3575e72c38b80a84e4f71c26713`, but its first main push run `31048060207` failed: classifier and trusted quality succeeded, candidate-quality intentionally skipped on neutral push, the Toast matrix was implicitly skipped, and the ledger failed closed.
- Issue #54 was immediately reopened because acceptance was not met.
- Root cause: the Toast matrix lacked an explicit `always()` admission condition, so GitHub propagated the intentionally skipped candidate-quality dependency through the successful aggregate quality job.
- Fix commit `aa8699b0d8a08d5f9ac27a70434fe02d26607ef0` adds explicit fail-closed Toast admission requiring classifier and aggregate-quality success, plus a regression. Exact diff hash `24c8a7aef60b6e6720ab2d9b74b3c7557e3bc128`.
- Red proof: regression failed with missing `full-suite.if`. Green proof: 30 focused tests, 414 full unit tests, Ruff, mypy, and actionlint 1.7.7.
- Independent adversarial review approved the exact two-file fix.
- PR #59 live run `31048550986` passed classifier, trusted quality, isolated candidate quality, all three Toast profiles, exact ledger, and reporters. Ledger: 11,580 packaged, 11,580 executed, zero failures/errors in every profile. Both required checks passed on exact head under GitHub Actions app 15368.
- PR #59 has one commit, exactly the workflow and regression files, literal `Closes #54`, and was submitted with normal merge command. Command returned successfully but merge commit/parents are not yet verified.
- Current blocker: none known. Next action: verify PR #59 normal two-parent merge and issue #54 closure, then require a green post-merge main run with all three Toast profiles and exact ledger before reporting completion.
- Preserve all unrelated dirty/untracked material. Do not stage, commit, or push this note.
