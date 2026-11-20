# Retain/fix checkpoint 16

## Current findings

- moo-conformance-tests PR #75 is normal-merged as `49f08f5b8b025109ab9885ea90f60756b0faae16`; the merge has two parents and issue #71 is closed.
- Barn PR #96 exact head `47aefbbecd1cbbc13996c879176e606e18a2c132` was blocked because logical `TaskRunning` ends inside `suspend()` before VM execution and saved-state publication finish.
- A scheduler-owned, refcounted physical execution lease is now implemented locally across `scheduler/scheduler.go`, `scheduler/task_runtime.go`, and `scheduler/run_gc_task_roots_test.go`.
- The lease is held for the full `runTask` invocation. Explicit global GC fails closed around any active sibling or a duplicated caller lease; per-task GC skips active siblings; deferred GC refuses to flush while any task is physically executing.
- The real-transition test registers a test-only suspend barrier, reaches logical `TaskSuspended` before VM handoff, and proves global `run_gc()` succeeds without sweeping. After handoff, it proves the suspended VM root remains while an unrelated orphan is collected.
- Red proof on `47aefbb`: the VM-only anonymous object was recycled during the active suspend handoff.
- Green proof after repair: the real handoff case, existing suspended-sibling case, and duplicate-caller refcount/collector case pass 20 repetitions; an independent focused run also passed.

## Other campaign state

- PR #110 is fresh and behaviorally sound but BLOCKED architecturally: issue #102 requires one store-owned object-authority rule, while delete_verb currently duplicates the owner/wizard/flag predicate instead of sharing the helper used by the other four metadata builtins.
- Next high-priority cluster after the foundation is object-authority mutation: finish #102 through the single owner, then Toast-oracle and fix critical privilege escalation #13.

## Current blocker and next action

- PR #96 is not yet ready: scheduler race, full tests, build, vet, formatting, commit/push, corrected PR body, fresh exact-head review, and corrected integration-branch conformance remain.
- Next: complete those gates; merge the corrected #96 history normally into PR #115; run the full corrected default-profile conformance gate including merged Anon7; independently review and audit PR #115 before any normal merge.

