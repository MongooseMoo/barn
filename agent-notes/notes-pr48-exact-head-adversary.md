# PR 48 exact-head adversarial audit checkpoint

State: read-only audit of moo-conformance-tests PR 48 head 3c4e91909d6e82e0a9eef5236868449f5ecf4e2c. No Toast process launched and no target-worktree files edited.

Current findings:

- Local HEAD and GitHub PR head both equal 3c4e91909d6e82e0a9eef5236868449f5ecf4e2c.
- PR is open and draft, has four linear commits based directly on main 7df5d1891a9849a10e3b5ec4f80235f1b5e03714, and includes literal `Closes #47`.
- Managed run 31019415969 is green for quality, three Toast profiles, and Full suite. GitHub executed synthetic merge commit dcc3607b8d2d7d614acdee09b161f20d878b68be, whose tree SHA da792dac439e5b55c356cee9c1939c434698b0da exactly matches head 3c4e919 tree SHA.
- Run evidence: candidate collection 11,581 total items, consisting of one admission and 11,580 conformance identities. Profile results were 11,550 passed and 30 skipped for 64-bit outbound-on; 11,535 passed and 45 skipped for 64-bit outbound-off; 11,531 passed and 49 skipped for 32-bit outbound-on. Ledger reported 11,580 of 11,580 union-executed.
- Toast checkout is pinned to aecc51e9449c6e7c95272f0f044b5ba38948459e. Workflow runs `git diff --exit-code` before out-of-tree builds; run logs show the pinned checkout and command in all profiles.
- Material blocker: the protected Toast workflow checks out and executes candidate code, and execution_ledger.main computes expected identities using candidate-local packaged_case_ids(). It does not invoke validate_candidate_inventory(), even though that existing function compares trusted vs candidate inventories and rejects deletions. Thus deleting or renaming away an identity removes it from both candidate expectations and JUnit reports, producing a false green. This PR itself deletes seven identities while still reporting exact equality.
- The permanent 41-entry baseline and workflow argument are removed. enforce_execution_surface now requires each candidate identity to pass in at least one supplied profile, rejects per-profile missing or extra identities, rejects failures/errors, and rejects union-never-executed identities.
- PR removals inspected: four read_stdin skeleton identities plus its presence identity, file_version_public, and minint_modulus_edge_case. Issue 47 permits correcting or removing expectations that do not describe pinned Toast, but current CI contains no deletion-preservation gate, so future removals need no independent proof.

Blocker: architecture cannot establish that future agents cannot evade the protected surface by deletion or rename because expected inventory is candidate-derived.

Next action: finish source-mutation, remaining skip/profile, JUnit parser, diff/commit/scope, and deletion-justification audits; then report exact evidence and MERGE or NO-MERGE to parent.
