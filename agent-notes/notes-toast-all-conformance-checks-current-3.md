# Toast all-conformance checks - current checkpoint

- Repository/worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-17`
- PR: MongooseMoo/moo-conformance-tests#34, draft, `Closes #17`
- Exact head: `8eaf7db4e7ed6257ff11a2a119b718badcf58199`
- Exact CI run: `30982355955`
- Prior pinned 32-bit evidence at `9ebb7a8`: 11,479 passed, 65 skipped, 22 failed. The failures were eight `slice()` string-start expectations and fourteen 64-bit-width contracts.
- Current correction: eight explicit 32-bit `E_RANGE` slice counterparts while preserving the eight original 64-bit identities; whole-suite 64-bit requirements for entirely width-specific overflow/persistence suites; case-level 64-bit gates for mixed map/object suites.
- Local gates: 228 harness tests passed; 11,574 packaged cases collected; duplicate baseline exactly 45 name groups and 28 content groups; Ruff passed; required mypy scope passed across 18 source files; diff check passed.
- Managed canonical WSL Toast proof for the seven affected files: 82 passed, 8 declared 32-bit-only cases skipped on the 64-bit oracle.
- Adversarial review: no implementation blocker; final operational verdict waits for exact-head CI.
- Current CI state: quality job green; 32-bit outbound-on, 64-bit outbound-on, and 64-bit outbound-off exhaustive profile jobs still running.
- Blocker: none beyond awaiting complete pinned-profile evidence and the protected execution ledger.
- Next action: require all three profile JUnits and `Full suite` ledger to be green, inspect exact identity/skip accounting, refresh the PR body, obtain final verifier verdict, then audit and normal-merge only if every guardrail is satisfied.
