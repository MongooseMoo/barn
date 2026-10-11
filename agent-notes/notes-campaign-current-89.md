# Current campaign state

- PR 106 was normal-merged as `911b97f324bd3238fc223530598d2908fa94ed57`; the merge has two parents and issue 72 is closed.
- PR 108 may now be updated from newest master and retargeted from the PR 106 feature branch to master. Its final diff must remain only the three genuine 2x files/deltas.
- PR 96 architectural adversary BLOCKED exact head `3a7015848ccb0803dc14d33f861d24e26c3ecf0a` on three concrete ownership defects:
  - terminal results are popped from VM into `t.Result` but never included in finalization handoff;
  - reentrant shutdown caller roots are copied into shutdown ownership, then collected again on terminal settlement instead of transferred exactly once;
  - generic `RunServerVerbTask` is privileged during shutdown, rather than a lifecycle-hook-only admission path.
- PR 96 aligned portions include exclusive SnapshotWithRoots linearization, serialization-only queued/suspended roots, readiness waiting for producers/GC/sinks, asynchronous reentrant callers, and hook-before-write ordering.
- PR 96 must not be pushed until the three adversary defects are fixed with red/green tests and both independent gates approve the new exact head.
- Conformance issue 18 implementation remains active in its isolated worktree.
- Next actions: send exact PR 96 blockers back to its worker; fetch newest Barn master, update and retarget PR 108, run exact local and paired gates; continue issue 18 implementation review.

