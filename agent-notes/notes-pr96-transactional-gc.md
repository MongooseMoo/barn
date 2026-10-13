# PR 96 transactional explicit-GC repair

## Findings

- Exact PR head `5e1b25802768956e713c618fafc937e75a92d01a` failed the normal `runTask`/`StoreTxn` multi-evaluation sequence: the task staged the numbered persistent-root clear, but explicit GC scanned the pre-commit live store and skipped the orphan recycle hook (`recycle_called` stayed 0).
- A pure transaction read-timestamp scan is unsafe for global GC because it can miss persistent roots committed after the transaction began.
- The accepted design is a private post-commit projection: clone the current live numbered and anonymous property census under the store read lock, overlay this task's staged creates/scalar flags/property changes in commit order, remove staged recycles, add runtime VM roots, expand anonymous edges, and freeze sorted candidates before callbacks.
- The VM uses this projection only when a task transaction exists; the non-transaction live-store path is unchanged.

## Current state

- Production changes are uncommitted in the PR worktree: `db/store/store_reachability.go` and `vm/anonymous_gc.go`.
- Uncommitted regressions cover staged persistent-root removal, a persistent root committed after transaction begin, staged anonymous-edge removal, staged anonymous-edge addition preserving a detached cycle, and the full multi-evaluation recycle-callback-created stash sequence.
- Focused store and scheduler tests were green before adding the final staged-edge-addition case.
- Full `db/store`, `vm`, and `scheduler` package tests were green before that final case.

## Blocker

- No design blocker remains. The final staged-edge-addition test and race gate still need execution.

## Next action

- Format and run the focused candidate tests, full affected package tests, and race-focused affected tests; inspect the final diff; report results to the parent agent. Do not commit or push.
