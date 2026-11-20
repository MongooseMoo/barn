# Current campaign checkpoint 38

## Current state

- Opened moo-conformance-tests issue #47, `CI: require every packaged test to execute against Toast`.
- Created worktree `C:/Users/Q/code/.worktrees/moo-conformance-all-toast-zero-skips` on branch `agent/all-toast-zero-skips` from merged main `7df5d1891a9849a10e3b5ec4f80235f1b5e03714`.
- Added a TDD regression proving a reviewed skip cannot excuse a case skipped by every Toast profile. Red proof: the old ledger returned success instead of raising.
- The ledger now has no skip-baseline input or parser, raises for every never-executed identity, emits schema version 2, and reports only the exact passed union.
- Deleted `ci/toast-never-executed.json` and removed its workflow argument.
- Local proof: 331 unit tests passed; 11,588 items collected (1 admission and 11,587 packaged); duplicate baseline, Ruff, mypy, and diff check passed.
- PR #46 merged normally with a verified two-parent commit; issue #43 remains open for its required live post-merge dispatch proof.

## Current blocker

- The stronger protected Toast union will intentionally fail on the current 41 permanent skips until their underlying profile/test causes are repaired.
- PR #45 owns none of those 41 and remains separate.

## Next action

Commit and push the zero-exception ledger change, open a draft PR closing #47, capture the managed CI red proof, then eliminate the 41 by source-proved Toast profiles and managed-oracle-correct test changes.
