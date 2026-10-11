# PR 96 pending-root repair checkpoint

## Current findings

- Rejected head `25d689e` checked out cleanly in `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair` on `agent/pr96-pending-roots-repair`.
- Shutdown cancellation and task finalization used different state: server cancellation did not mark scheduler completion as pre-checkpoint shutdown.
- Terminal VM activations had already popped their frames before the scheduler attempted to collect roots.
- `kernel.TaskContext.TaskLocal` duplicated the canonical `task.Task` value and was not updated by `set_task_local()`.
- Store snapshot normalization collected only anonymous roots, so loaded pending WAIFs were dropped.
- Pending append deduplicated via `Value.String()`, collapsing distinct same-class WAIF identities.
- WAIF properties were not traversed for nested anonymous serialization dependencies.

## Current implementation state

- Added explicit scheduler pre-checkpoint shutdown state set by the server before cancellation.
- Captured ANON and WAIF identities at VM frame-pop boundaries; shutdown completion hands them to the store and skips ordinary orphan GC.
- Removed `TaskContext.TaskLocal`; `task.Task` is the sole owner and VM collection uses a narrow `GetTaskLocal()` interface.
- Preserved/filtered pending WAIFs by instance identity, retained the deterministic anonymous closure/cycle planner, and rewrote nested anonymous references through identity-preserving WAIF graph clones.
- Writer WAIF reference indexing now explicitly keys on `WaifIdentity()`.
- Added red/green contracts for real task-local roots, shutdown single/cycle anonymous roots, task-local WAIF shutdown, loaded/distinct/persistently-filtered WAIF roots, and WAIF identity/backref/nested-anon database round-trip.

## Verification state

- Red evidence captured for VM task-local collection, store WAIF preservation/filtering, format WAIF round-trip, and missing scheduler shutdown state.
- Focused contracts pass for 10 repeats and 3 race-enabled repeats.
- Changed packages pass; full `go test ./... -count=1` and `go build ./...` pass.
- `go vet ./...` exits successfully with the pre-existing `vm/stack.go` `ReadByte` signature warning.
- Managed conformance run `20260805_044427` passed Anon1, Anon2, and Anon6: 3 passed, 11576 deselected, 0 failed.

## Blocker

- The nested worktree required a temporary junction `C:\Users\Q\code\.worktrees\moo-conformance-tests` to the real sibling conformance repo. Two validated PowerShell `Remove-Item` attempts were rejected by execution policy before running. No alternate deletion primitive was attempted. The junction remains outside the Barn worktrees and needs authorized cleanup.
- Managed conformance generated untracked `reports/runs/` in the repair worktree; it must remain unstaged or be removed safely before the repair commit.

## Next action

- Audit the final diff and intended file list, preserve generated evidence without staging it, rerun final gates if code changes, stage only the cohesive repair files, commit locally, and report exact head/files/gates plus the junction cleanup blocker.
