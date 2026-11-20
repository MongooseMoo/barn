# PR #34 final verifier checkpoint

- Scope: read-only verification of `MongooseMoo/moo-conformance-tests` PR #34 for issue #17. No server starts or project-history reads.
- Superseding exact head: `9ebb7a84b495509d51f94680e1f5c601a72f45fa`, base `a46fa56603451cff34533ffc0b0f104dab5bdf1f`.
- Exact CI run under review: `30981685916` is terminal with conclusion `failure` on the exact requested head.
- Older heads/runs, including `fe4d4b...` / `30980948415`, are obsolete and excluded from final evidence.
- The newest commit changes exactly three files with a clean diff check. It retains the `-2147483648 % -1` expression and expected value `0`, replaces the dead 32-bit feature skip with the literal crash skip, adds the identical literal to the reviewed never-executed baseline, and updates the asserted baseline size from 40 to exactly 41.
- Quality passed with 228 units, 11,566 collected cases, exact 45-name/28-content duplicate accounting, Ruff, and mypy over 18 files.
- Both 64-bit JUnits account for exactly 11,566 identities with zero failures/errors: outbound-on is 11,495 passed plus 71 skipped; outbound-off is 11,480 passed plus 86 skipped.
- The 32-bit JUnit also accounts for exactly 11,566 identities, and confirms the new minint case skipped with the exact reviewed reason, but it contains 22 failures, 11,479 passes, 65 skips, and zero errors. Residual failures cover slice string starts, 64-bit overflow expectations, integer/object authority, and dump persistence.
- Protected `Full suite` failed its prerequisite gate because the 32-bit profile failed. No `toast-execution-ledger` artifact exists; only the three profile artifacts and quality artifact were uploaded.
- PR remains draft, mergeable at the Git level, and `BLOCKED`. Body contains recognized `Closes #17`, but its proof is stale at 222 harness tests versus the exact run's 228. Draft status and required-check failure both block merge eligibility.
- Workflow review: least-privilege `contents: read`; action SHAs, Toast commit, and uv lock are pinned; quality gates include units, packaged collection, exact duplicate baseline, Ruff, and mypy; three profile jobs retain JUnit/log artifacts; the protected `Full suite` aggregates exact JUnit identities and fails on missing/unknown/duplicate/failure/error/unreviewed-never-run/stale/reason-drift conditions.
- Branch protection requires strict `Full suite` and enforces admins. Normal merge commits are allowed, but no merge is eligible until the PR is non-draft and the required context succeeds.
- Final gate: `NO-MERGE`. Repair or correctly profile-gate the 22 oracle-backed 32-bit mismatches, obtain all-green exact-head CI and a successful ledger artifact, refresh the PR proof, and mark the PR ready before re-verification.
