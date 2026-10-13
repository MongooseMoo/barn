# PR 96 final analysis checkpoint

- Review target: exact worktree HEAD `3a7015848ccb0803dc14d33f861d24e26c3ecf0a`.
- Scope inspected so far: the two requested commits, changed-path list, scheduler shutdown/admission code, task factories/runtime, deferred GC, server shutdown/checkpoint integration, and task snapshot roots.
- Current observations: changed paths are code/tests only; no plan, note, report, handoff, or Markdown path is in the two-commit diff. Worktree was clean when checked. Full and focused race tests are running.
- Potential risk under investigation: producer-token ownership around immediate and ready tasks; shutdown publication after an already-published boundary; pending-finalization sink concurrency; lock ordering among scheduler, GC, task snapshot, and store locks; serialization-only root ownership.
- Blocker: the configured role protocol path `C:\Users\Q\.Codex\AGENTS.md.d\gauntlet.md` does not exist. The installed `protocols:gauntlet` skill was read instead.
- Next action: complete current-code/diff inspection, collect test/race results, exercise targeted failure modes if necessary, and return a strict APPROVE/BLOCK verdict without edits.

## Confirmed analysis findings

- BLOCK: task roots are not excluded from pending-finalization normalization. `planAnonymousSerializationLocked` normalizes pending candidates against persistent roots before `extraRoots` are collected, and pending WAIF filtering likewise has no task-root set. A root shared by a suspended task and a pre-shutdown deferred candidate is serialized both as task-owned and pending-finalization-owned.
- BLOCK: suspended-task handoff drops frame-popped finalization candidates. Suspend queues one coarse `TaskOwned` anonymous request; shutdown skips every such request, while `PersistenceVMSnapshot` and `Snapshot.RootValues` do not carry `VM.PendingFinalizations` or `VM.PendingWaifs`. A helper-created root that has left the live frames before `suspend()` is neither a task snapshot root nor a pending root.
- BLOCK: direct return roots are not captured. `OP_RETURN` pops the returned value before `VM.Return`; frame-pop capture reads locals/args/context but not the `value` argument, and terminal settlement does not receive `result.Val`. `return create(...)` / direct WAIF returns can disappear from shutdown publication.
- BLOCK: post-publication admission is unrestricted for `RunServerVerbTask`, including ordinary connection hooks. Such producers increment after `shutdownReady` is already closed and therefore cannot delay Panic's checkpoint. Reentrant unclean `shutdown()` from `shutdown_started` also starts `Panic` immediately in a goroutine, allowing concurrent checkpoints against the same `.tmp`/`.new` paths.

## Verification state

- `go test ./...`: pass.
- `go test -race ./db/store ./db/format ./scheduler ./server ./vm`: pass.
- Twenty-count focused race stress for shutdown/finalization/GC and persistence roots: pass.
- Two-commit diff check: clean; current review worktree clean; no Markdown, plan, note, report, or handoff path in the diff.
- Next action: collect final exact line evidence and send BLOCK verdict to the parent agent. No implementation changes will be made.
