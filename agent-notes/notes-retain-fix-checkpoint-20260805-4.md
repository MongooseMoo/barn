# Retain/Fix Checkpoint 4

## Current findings

- A clean conformance worktree now exists at `C:/Users/Q/code/.worktrees/moo-conformance-retain-fix` on branch `agent/retain-fix-delete-boundary`, based on current `origin/main` `acc2718cbc3093c7527448faec0f6c162dac9fe6`.
- One focused case was added to current `src/moo_conformance/_tests/builtins/verbs.yaml`: delete a target verb, perform later coarse add_verb, raise E_INVARG, then inspect verbs in the next task.
- The intended observation is whether Toast preserves both verb-list mutations after the later error, which establishes whether that sequence is an irreversible logical boundary rather than task rollback.

## Managed oracle attempt

- Working directory: `C:/Users/Q/code/.worktrees/moo-conformance-retain-fix`.
- Managed server template used the required WSL oracle `/root/src/toaststunt/build-release/moo` with `{db}`, `{db}.new`, `{port}`, and `{server_dir}` placeholders.
- Expected: one selected oracle case.
- Actual: exit 1 before server startup. The fail-closed harness rejected packaged conformance without a selected canonical admission test or matching external admission evidence.
- No server was manually launched and no tracked database was used directly.

## Current state

- The conformance case is uncommitted.
- The Barn authority branch remains unpushed and reviewer-blocked on the successful coarse boundary.
- No external PR or merge action has been taken.

## Blocker

- The oracle result is not yet known because admission was not included in the first selected run.

## Next action

- Run the canonical admission item and the selected verbs oracle case together in one managed WSL Toast session, or use matching generated admission evidence if same-session selection cannot express both.
- Preserve the default profile and avoid unrelated profile expansion.

