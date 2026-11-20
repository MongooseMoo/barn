# PR 96 repair-boundary investigation checkpoint

## Current findings

- Current root checkout is `aa82abf`, while `origin/master` is `56e1358`.
- PR branch `agent/issue-95-pending-finalizations` is at `0450cf2`.
- Candidate repair branch `agent/pr96-pending-roots-repair` is at `6aeadc3` and has eight commits above `0450cf2`: `ff95ff7`, `25d689e`, `6f1022f`, `55bf832`, `8b774ce`, `dc34264`, `6aec871`, and `6aeadc3`.
- Dirty attempt is in `C:/Users/Q/code/.worktrees/barn-baseline`; exact changed files and content are not yet inspected.

## Observations

- Multiple hypotheses remain live: shutdown reorder only; a scheduler barrier plus atomic store API; selective extraction from the eight-commit repair stack; or a broader lifecycle redesign.
- No Toast process has been launched and no external state has been changed.

## State and blocker

- Investigation is read-only except for this checkpoint file, which is required by the runtime and must remain uncommitted.
- Initial repository inventory output was truncated, so commit/file boundaries and dirty-worktree changes still need focused inspection.

## Next action

- Inspect the exact diffs and scheduler/store call graph at `56e1358`, `0450cf2`, the dirty four-file attempt, and each repair-stack commit; then derive invariants, races/deadlocks, and the minimal correct test/commit boundary.

## Checkpoint 2 findings

- PR head root selection is non-atomic: `CollectPendingFinalizationValues` inspects persistent reachability and each candidate closure through separate store operations, then the caller appends later. A commit can add or remove graph edges between those observations and the snapshot.
- Correct store-side shape is lossless candidate capture from the VM followed by filtering and root minimization at the snapshot linearization point under one exclusive store lock. Commit `ff95ff7` establishes lossless VM candidates; `25d689e` moves normalization into `Store.Snapshot` and makes the full graph copy exclusive.
- The dirty four-file attempt canonicalizes at append time. Its new graph-mutation test cannot pass: appending `{A,B}` while `A -> B` drops `B`; removing `A -> B` later cannot reconstruct the discarded candidate at snapshot.
- PR shutdown calls `Server.Shutdown` from a running task, cancels only the server context, and returns. `server.shutdown` stops input, performs the final checkpoint, and only then stops the scheduler. Therefore an admitted worker can commit, suspend, enqueue deferred GC, or release its VM after the final snapshot.
- A simple `scheduler.Stop` reorder is not sufficient. If the shutdown caller finishes before scheduler cancellation, normal completion releases its VM after enqueuing deferred GC; the deferred request is not itself a persisted pending-finalization root. If cancellation wins, roots are captured, so behavior remains schedule-dependent.
- A synchronous wait in the shutdown builtin is also invalid: the caller is one of the active worker slices being waited upon and would deadlock. Shutdown publication must be asynchronous for task/GC callers: close admission, return from the builtin, let already-admitted producers publish either terminal roots or a complete suspended-task snapshot, then signal the server to checkpoint.
- The eight-commit stack implements this with an admission/producer counter, deferred-GC ownership, and a ready channel, but also expands into WAIF startup finalization, task suspension transitions, task-owned checkpoint roots, and lifecycle hook ordering.

## Current state

- Competing hypotheses narrowed: shutdown reorder alone is ruled out; append-time store canonicalization is ruled out. A scoped scheduler producer barrier plus snapshot-time store normalization is required. Whether the full lifecycle subsystem is necessary remains under review.

## Next action after checkpoint 2

- Audit the exact deferred-GC and suspended-task ownership transitions, identify which repair-stack commits are prerequisites versus independent scope expansion, and derive the smallest coherent file/commit set and required deterministic race tests.
