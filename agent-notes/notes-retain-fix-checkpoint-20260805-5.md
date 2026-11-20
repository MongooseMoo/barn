# Retain/Fix Checkpoint 5

## Current findings

- Managed default-profile WSL Toast run passed canonical admission plus the new delete/coarse/error lifecycle case: 2 passed, 74 deselected.
- The same admitted managed selection passed against Barn authority head `2ee08aa7c29eeac1d50bfe3bbf135b93e283af58` using a managed `go -C ... run ./cmd/barn` server template.
- The independent reviewer withdrew the blocker and approved both the Barn authority head and the conformance case.
- Conformance harness unit tests passed: 459 passed.

## Observation

- Toast preserves both the deletion and later add_verb after a subsequently raised E_INVARG. Successful coarse mutation is therefore an irreversible logical boundary; task-wide rollback is not the contract.
- Failed validated coarse flush remains retryable and leaves the live store untouched.

## Current state

- Conformance commit: `9ebea987598186305c2b45de355408e79f66be19`.
- Draft conformance PR: `MongooseMoo/moo-conformance-tests#66`, titled `Pin delete_verb coarse-boundary semantics`.
- PR #66 has exactly one commit, one changed file, 22 additions, and no deletions.
- Barn authority branch remains clean and unpushed at approved head `2ee08aa7c29eeac1d50bfe3bbf135b93e283af58`.
- Draft Barn bcrypt PR #109 remains open and unmerged.

## Blocker

- No implementation blocker remains. Required CI checks have not yet completed for conformance PR #66, and the Barn authority PR has not yet been published.

## Next action

- Push the exact approved Barn authority head and open a draft PR with literal `Closes #102`.
- Audit both PRs' files, commits, bodies, and checks.
- Do not merge until required checks are green and the exact normal merge commands are audited.

