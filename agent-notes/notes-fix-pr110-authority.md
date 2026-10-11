# PR #110 authority redesign checkpoint

- Task: implement issues #102/#105 redesign in isolated worktree `C:/Users/Q/code/.worktrees/barn-fix-105-atomic-delete-authority`.
- Required integration: normal-merge current `origin/master` at `95b5758`; no rebase or force push.
- Required architecture: one pure store authority rule; one transaction-aware wrapper shared by all five metadata builtins; transaction verb deletion stages exact authority-free deletion with scalar and generation conflict guards; no-transaction fallback applies the pure rule under lock; staged-delete-to-coarse boundaries use `CommitAndRenew`.
- Required coverage: staged owner/flag grant and revocation tables across all five metadata builtins; exact-delete/coarse conflict cases; preserve `E_VERBNF` precedence.
- Current state: implementation has not started in this task turn. No push or PR edit is authorized.
- Current blocker: none known; current source/tests and live GitHub issue/PR state still need inspection.
- Next action: inspect current issue/PR and source/test state, merge `origin/master`, establish focused red tests, implement the centralized authority path, then run focused/full/race/build/vet/format/diff gates.
- This file is a local checkpoint only and must remain untracked and uncommitted.

## Checkpoint 2

- Verified live GitHub state: issue #102 is open because the prior repair did not bind authority to irreversible deletion; draft PR #110 is at `2ee08aa7c29eeac1d50bfe3bbf135b93e283af58` with Build green and conformance red.
- Verified the isolated feature worktree was clean, fetched `origin/master`, confirmed it was exactly `95b5758cbf964f7b8a2fc2443c99090a4c32176d`, and normal-merged it with `git merge --no-ff origin/master`.
- Merge commit is `30f7a2e` and the worktree remains isolated from unrelated root-checkout changes.
- Current-source finding: `properties`, `verbs`, `add_property`, and `delete_property` use `objectAllowsForRead`; `delete_verb` instead duplicates owner/write/wizard authority inside both `StoreTxn.DeleteResolvedVerbAuthorized` and `Store.DeleteResolvedVerbAuthorized`.
- Current-source finding: exact transaction deletion already records a verb-list generation scan, while staged owner/flag reads record the scalar generation; the intended split is therefore a shared transaction-aware builtin wrapper followed by authority-free exact staging.
- Current-source finding: staged verb deletions currently special-case validation inside `FlushStagedToLive`; current master already provides `CommitAndRenew`, which should own the validating staged-delete-to-coarse boundary and transaction replacement.
- Added red tests only: pure `ObjectAllows` table, staged owner/flag grant and revocation across all five metadata builtins, and exact-delete `CommitAndRenew` success/conflict coverage.
- Red result: `go test ./builtins ./db/store -run 'Test(ObjectAllows|ObjectMetadataBuiltinsUseSameTaskStagedAuthority|StoreTxnVerbDeleteCommitAndRenew)' -count=1` fails in `db/store` because `ObjectAllows` and `StoreTxn.DeleteResolvedVerb` are undefined; `builtins` passes because its current duplicated authority paths happen to satisfy the behavior table.
- Blocker: none; the red compile errors are intentional TDD evidence.
- Next action: implement the pure store rule, route all five builtins through one transaction-aware wrapper, make transaction exact deletion authority-free, reuse the pure rule under the no-transaction store lock, and route staged-delete coarse boundaries through `CommitAndRenew`.

## Checkpoint 3 - implementation complete locally

- Implemented one pure `db/store.ObjectAllows` rule and retained one transaction-aware builtin wrapper, `objectAllowsForRead`, as the sole admission path for `properties`, `verbs`, `add_property`, `delete_property`, and `delete_verb`.
- `StoreTxn.DeleteResolvedVerb` is now authority-free exact staging. The builtin wrapper records the staged scalar owner/flags view; verb resolution records the ordered-list generation. Ordinary commit validates both before applying the exact ordered delete.
- The no-transaction fallback still validates exact identity before authority to preserve `E_VERBNF` precedence, and now calls the same pure `ObjectAllows` rule while holding the store lock through deletion.
- Staged-delete-to-coarse boundaries now call `CommitAndRenew`, replace `ctx.StoreTxn`, and mark published mid-task state non-retryable. `FlushStagedToLive` fails closed if given an exact staged verb deletion.
- Added all-five table coverage for same-task staged owner and required-flag grant/revocation; added the pure rule table; converted exact delete success/conflict boundary coverage to `CommitAndRenew`. Existing exact descriptor, scalar conflict, generation conflict, unrelated-read conflict, and coarse-builtin conflict tests remain.
- TDD red: focused `db/store` build failed only on undefined `ObjectAllows` and undefined authority-free `StoreTxn.DeleteResolvedVerb`; `builtins` behavior rows passed through the then-duplicated implementation.
- TDD green before latest master: focused tests x10, package tests, full `go test ./...`, full `go test -race ./...`, build, focused vet, gofmt, and diff checks passed. Full vet showed only the preexisting `vm/stack.go:49` `ReadByte` signature warning.
- Local redesign commit: `20f5fde795543cd0c72de2f64db14da23f9f20f2` (`refactor: centralize metadata object authority`), exactly 8 files, 164 insertions, 121 deletions.
- Per coordinator update, fetched and verified new `origin/master` exactly `26ffaa21fcbbabd8c52f7c44b40c9cbed99defce` and normal-merged it without conflicts as `0e812057d53b7f68f3f9096908a5c5726685fa78`.
- Both required branch-update merges are two-parent merge commits: `30f7a2e` parents `2ee08aa` and `95b5758`; `0e81205` parents `20f5fde` and `26ffaa2`.
- Post-latest-master gates all green: focused authority/exact/coarse tests x10; `go test ./... -count=1 -timeout=15m`; `go test -race ./... -count=1 -timeout=20m` with scheduler 277.349s and explicit exit 0; `go build ./...`; `go vet ./...`; touched-file gofmt; final `git diff --check`.
- Final feature diff versus current master: 13 files, 821 insertions, 73 deletions. This includes original PR commit `2ee08aa` plus redesign commit `20f5fde`; master merge contents are excluded.
- Final worktree is clean and ahead of the remote feature branch by 29 commits. No push or PR edit was performed.
- Blockers: none.
- Next action: coordinating root agent should independently review/audit this local head, then decide when to push/update PR under the campaign workflow.
