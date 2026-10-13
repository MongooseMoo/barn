# Retain/fix checkpoint 32

## Current findings and state

- Barn PR #115 is merged at `95b5758cbf964f7b8a2fc2443c99090a4c32176d`; its merge commit was verified to have two parents. Issues #72, #95, #107, #111, and #113 are closed. Superseded PR #108 is closed.
- Conformance PR #76 is merged and its Toast-backed default-profile case is green.
- Barn PR #99 / issue #91 is repaired at local and remote head `00af402d8f1a16dc4229cebb2fc442b0cc49b745`. Independent review returned CODE APPROVE. Build/unit CI is green; the fresh default-profile conformance job is still running. No merge has occurred.
- Barn PR #94 / issue #93 is repaired locally at `13098599a5d6e7911ef6da627c004c3fa9f6700b`, with normal merge commit `825b9ad14620fb4a77aeeae745aae63cc1d1a596` retaining current-master lifecycle/GC work. Focused, full, race, build, scoped vet, formatting, and diff gates are green. Repository-wide vet has only the issue #91 warning fixed by PR #99. Independent review is running. Nothing is pushed and the PR body is not yet updated.
- Barn PR #110 / issue #102 is being consolidated locally after a normal merge of current master. The worker reports the single pure authority rule, one transaction-aware wrapper for all five builtins, authority-free resolved verb deletion, CommitAndRenew boundary, and staged grant/revocation/conflict tests are implemented. Final gates are running. Nothing is pushed and the PR is not edited.

## Blockers

- PR #99 cannot merge until the exact-head default-profile conformance check completes green.
- PRs #94 and #110 must absorb the newest master after #99 merges, pass exact-head review and CI, and receive literal scope/closing-phrase audits before normal merge.

## Next action

1. Verify PR #99 conformance results, audit files/commits/issue/checks/closing phrase, normal-merge, verify a two-parent merge commit, and verify issue #91 closes.
2. Normal-merge the new master into repaired PR #94, rerun gates/review, correct its provenance text, push, require fresh CI, then audit and normal-merge.
3. Finish PR #110 gates and independent review; incorporate newest master, push, require fresh CI, then audit and normal-merge.
4. Re-enumerate current open Barn issues and PRs from GitHub and continue the cleanup queue from current source and tests only.

This note is intentionally uncommitted and must not be staged, committed, or pushed.
