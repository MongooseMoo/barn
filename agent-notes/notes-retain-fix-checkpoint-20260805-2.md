# Retain/Fix Checkpoint

## Current findings

- PR #109 fixes the merged bcrypt raw-salt parser regression by selecting the first cost delimiter after the prefix. Its focused, race, full test, build, and focused vet gates are green; independent review approved it.
- PR #109 remains draft. Its legacy conformance job is failing on the pre-existing mutable-suite/default-profile CI problem, not on the two changed bcrypt files.
- The delete_verb correction is being moved from an irreversible live mutation to a StoreTxn-staged verb-list deletion.
- The staged deletion uses the transaction-private owner/FlagWrite state, validates the original scalar and verb-list generations, and publishes only after every read and target validation succeeds.
- Focused functional and race tests and the full Go test suite are green on the current uncommitted authority implementation.

## Observations

- Shifted repeated indices replay correctly because deletes are recorded in the same order as mutations to the private verb list and the starting verb generation is validated.
- A code write staged for the deleted target is removed; survivor code writes remain.
- Failed commit or validated flush conflicts leave the live verb list unchanged.
- Adding staged verb deletion to HasStagedTopology exposed an ignored-error defect: flushStagedBeforeCoarse previously discarded FlushStagedToLive errors and marked the task live-mutated.

## Current state

- Issue #72 and issue #102 are reopened.
- The merged PR #98 record was corrected to remove the Toast-fidelity overclaim.
- PR #109 is open as a draft with literal `Closes #72`.
- The authority branch has one earlier partial commit plus uncommitted corrective work. It has not been pushed or opened as a PR.

## Blocker

- No design blocker remains. The current mechanical task is to propagate flushStagedBeforeCoarse errors through every direct coarse-builtin caller without changing unrelated behavior.

## Next action

- Finish caller propagation and add object/verb coarse-builtin conflict regressions.
- Rerun focused race, full tests, build, vet, formatting, and diff checks.
- Independently audit the exact authority head before opening a draft PR with literal `Closes #102`.
- Keep both PRs unmerged until exact scope, checks, and the correctly scoped managed default-profile conformance proof are settled.

