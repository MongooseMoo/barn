# Toast all-conformance checks - terminal CI checkpoint

- PR: MongooseMoo/moo-conformance-tests#34, now ready for review, with literal `Closes #17`.
- Exact head: `8eaf7db4e7ed6257ff11a2a119b718badcf58199`.
- Exact run `30982355955` is fully green: quality, all three Toast profiles, and protected `Full suite`.
- Quality: 228 harness tests; 11,574 packaged cases; duplicate baseline exactly 45 name groups and 28 content groups; Ruff green; required mypy scope green across 18 source files.
- JUnit: each profile has 11,574 unique identities and zero failures/errors. 32-bit/outbound-on is 11,484 pass and 90 skip; 64-bit/outbound-off is 11,480 pass and 94 skip; 64-bit/outbound-on is 11,495 pass and 79 skip.
- Ledger artifact `8920748250`: 11,533 executed identities plus 41 reviewed never-executed identities, disjoint exact partition of all 11,574 packaged cases.
- Adversary found no code blocker; verifier verdict is MERGE for the exact head.
- PR body has been refreshed with exact proof and the draft flag removed.
- Live main protection remains strict with admin enforcement, no force pushes/deletions, and required context `Full suite`; no rulesets exist.
- Blocker: none. A final pre-merge scope/files/commits/checks/closing-phrase audit is required.
- Next action: audit exact PR state, run only `gh pr merge 34 --repo MongooseMoo/moo-conformance-tests --merge`, verify a two-parent merge commit and issue #17 closure, then add the quality context to branch protection without replacing existing settings and verify both required contexts.
