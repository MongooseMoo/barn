# Issue 103 finished_tasks permission gate

## Current state

- Assigned production scope: `builtins/signatures.go` only.
- Assigned test scope: narrow `builtins` tests for `finished_tasks` permission and validation ordering.
- Required workflow: dedicated worktree/branch from current `origin/master`, TDD red/green, full Go gates, explicit commit/push, and PR with `Closes #103`; no merge.
- No Barn or Toast server will be launched manually.

## Findings and observations

- Prior current-state analysis located the defect in `builtinFinishedTasks`: arity is checked, then the global task manager is enumerated without a wizard gate.
- The adjacent `builtinThreads` and `builtinThreadPool` functions already reject non-wizards with `E_PERM`.
- Tests must avoid populating or cleaning the global task manager; a non-wizard denial before manager access and a wizard empty-or-list success assertion can be independent of fixture contents.

## Blocker

- None currently.

## Next action

- Refresh and verify the current remote master, create the dedicated worktree, inspect issue #103 and current owned files, then add tests and capture the red result before implementing the gate.
