# Notes: merging `master` into `work/mvcc-concurrent-moo` (ATTEMPT 2)

## What happened in attempt 1 (all of it reverted)
- I resolved the 7 conflicts by **dropping two of master's commits** (`685c5f4`
  batched GC, `3c16e48` real-suspend yin) and committed it as `2b2e8eb`.
- Wrong gate. Merging master into a branch must not lose master's behavior.
  I used "no worse than either parent" instead of "master stays green."
- My conformance numbers were **invalid**: `uv run --project` failed on a
  `.venv/lib64` symlink, so I hand-started servers against ONE `Test.db` copy
  reused across runs. State accumulated between runs. That's why "master" scored
  6 failures on one run and 14 on the next.
- The bad resolutions got recorded into `.git/rr-cache` (7 entries, 19:14:20).
  **Deleted** (44 -> 37 entries, none with today's stamp). Backup of the deleted
  entries: `<scratchpad>/rr-cache-backup/`.
- Branch reset to `f4fc1cb`. Bad merge preserved as tag `bad-merge-2b2e8eb`
  (delete with `git tag -d bad-merge-2b2e8eb`; nothing else references it).

## TOOLING FIXED (do not regress this)
Root cause: `../moo-conformance-tests/.venv/lib64` was a POSIX symlink -> `lib`,
which Windows `uv` cannot remove. Fix: `rm -rf ../moo-conformance-tests/.venv`,
let uv rebuild. Managed mode now works:
```
uv run --project ..\moo-conformance-tests moo-conformance \
  --server-command "<abs/path/to/barn.exe> -db {db} -port {port}"
```
Managed mode uses a **fresh temp copy of Test.db per run**. Never hand-start a
server against a reused db again — the numbers are garbage.

## GROUND TRUTH (measured, managed mode, 2026-07-09)
- **master (`c099532`): 11335 passed, 0 failed, 126 skipped.** Master is GREEN.
=> THE GATE: after the merge, conformance must be **0 failures**. Anything else
   means a master fix was lost.

Not yet re-measured on the clean harness:
- pre-merge branch `f4fc1cb`
- the "compose" GC (see below) — its 3 alleged regressions were measured on the
  BROKEN harness, so they may not be real. **Re-measure before redesigning.**

## Current state
Mid-merge (`git merge master` re-run with rerere clean; it recorded only
preimages, resolved nothing — rerere has no answers for these conflicts).
7 conflicts, same as before:
builtins/{network_test,objects,properties,tasks,verbs}.go
scheduler/{scheduler,task_runtime}.go
(`scheduler/waif_lifecycle.go` + `vm/anonymous_gc.go` auto-merge CLEAN but do NOT
compile — see hazard below.)

### Resolved so far this attempt
- [x] `builtins/network_test.go` — additive both sides, kept all 5 stubConn fields.
- [x] `builtins/objects.go` — took master's relocation of parent validation to
      after optional-arg parsing (`6b46671`); ported `validForRead` + txn-aware
      `HasDuplicateDefinedPropertyAmong` into the moved block.
- [x] `builtins/properties.go` hunk 1 (add_property) — `objectExistsForRead` +
      master's `E_INVARG`.
- [ ] `builtins/properties.go` hunk 2 (clear_property) — IN PROGRESS.
      master's `c099532` order already auto-merged above the conflict (perm check
      BEFORE the defined/E_INVARG check, plus a new perm check in the
      FindProperty-error branch). Conflict collapses to just the txn-aware
      `ClearPropertyOverride`. THEN port the two `store.HasObjectFlag` /
      `store.ObjectOwner` calls at lines ~435/437/446 onto
      `hasObjectFlagForRead` / `objectOwnerForRead`.
- [ ] `builtins/verbs.go` — 3 hunks, all `objectExistsForRead` + master's
      `E_INVARG`; hunk 3 (`builtinDisassemble`) also keeps master's new
      `switch args[1].Type() {case STR,INT: default: E_TYPE}` BEFORE the check.
- [ ] `builtins/tasks.go` (`yin`) — TAKE MASTER'S VERSION. Then fix Barn:
      master's suspend path never does `t.CreatedForks = nil`, which
      `TestYinFlushesCommittedForksBeforeLaterConflict` requires. Fix the SUSPEND
      PATH (`scheduler/task_runtime.go` FlowSuspend) to clear CreatedForks after
      commit — do NOT throw away master's yin like last time.
- [ ] `scheduler/scheduler.go`, `scheduler/task_runtime.go` — KEEP master's
      batched GC. Make it race-free here. Do not delete it.

## HAZARD (silent, no conflict markers)
`scheduler/waif_lifecycle.go` and `vm/anonymous_gc.go` auto-merge cleanly and do
not compile: master's `flushDeferredGC` calls `s.liveTaskVMs(nil)`, which HEAD
deleted in `21a7909` because it reads a concurrently-running sibling's
`BytecodeVM` without holding `task.mu` (the race `review_concurrency_test.go`
records as BUG-3, after `987edc2` made tasks run in parallel). HEAD replaced it
with `collectSiblingGCRefs` (snapshots refs under `s.mu`, skips TaskRunning).
HEAD also changed `liveWaifs` sig and `vm.AutoRecycleOrphanAnonymousSince`
(4th arg `siblingRefs map[types.ObjID]struct{}` instead of `extraVMs ...*VM`).

## The GC problem to actually solve
master's deferred GC needs roots = persistent reachability + every live task's VM
refs, gathered at flush time. On this branch a running task's VM cannot be read
(race) nor ignored (its locals are roots).

Useful fact: `ProcessReadyTasks -> runReadyTasks -> runTaskBatch`, and
`runTaskBatch` JOINS all workers. So after `runReadyTasks` returns, NO task is
`TaskRunning` — roots are readable completely and race-free there.

Attempt-1 "compose" (saved at `<scratchpad>/compose_backup/`):
- `AnonGCRequest.OwnRefs` — snapshot the deferring task's own VM refs at defer
  time (its VM is released before flush).
- `RecycleOrphanAnonymousBatch(..., siblingRefs map)` instead of `liveVMs ...*VM`.
- `collectAllGCRefs() (anon, waifs, ok)` — ok=false if ANY task is TaskRunning.
- flush from end of `ProcessReadyTasks` + inline at task completion.
It passed `-race` and `go test ./...` but reportedly failed 3 conformance tests
(`anonymous::anonymous_lost_reference_calls_recycle`,
`recycle::recycling_anonymous_recycles_property_values_on_{object,parent}`).
**Those 3 were measured on the broken harness. RE-MEASURE FIRST.**
Note `scheduler/eval.go` has its own synchronous GC and never calls `runTask`,
so the failure path was never explained — that unexplained gap is the real work.

## PROGRESS (attempt 2, live)
- [x] network_test.go, objects.go — done (see above).
- [x] properties.go BOTH hunks — hunk2 collapsed to txn-aware ClearPropertyOverride;
      also ported master's new `store.HasObjectFlag`/`store.ObjectOwner` in the
      FindProperty-error branch + write-perm check onto
      `hasObjectFlagForRead`/`objectOwnerForRead`.
- [x] verbs.go all 3 hunks — `objectExistsForRead` + master's `E_INVARG`;
      hunk3 keeps master's `switch args[1].Type()` E_TYPE gate first.
- [x] tasks.go — **master's yin taken WHOLE** (real suspend via
      `task.GetManager().SuspendTask(t, seconds)` + `types.Suspend(seconds)`).
      Branch's yielder-yin deleted. `hostOf(ctx).TaskYielder` now unused by yin.
- [x] scheduler/scheduler.go conflict — resolved to HEAD (empty). master's
      `s.flushDeferredGC()` belonged to ProcessReadyTasks, which HEAD split into
      runReadyTasks/runTaskBatch. STILL TO DO: add `s.flushDeferredGC()` at end of
      `ProcessReadyTasks` (after `s.runReadyTasks(readyTasks)`), which is the
      quiescent point (runTaskBatch joins all workers).
- [ ] scheduler/task_runtime.go — 4 hunks still open (lines ~350, 476, 493, 540).
- [ ] Fix Barn so master's yin works here: FlowSuspend path in task_runtime.go must
      do `t.CreatedForks = nil` after `ctx.StoreTxn.Commit()`, else forks created
      before the yin commit stay queued against a released txn
      (`TestYinFlushesCommittedForksBeforeLaterConflict`).
- [ ] waif_lifecycle.go / vm/anonymous_gc.go: make master's deferred GC race-free
      (compose from `<scratchpad>/compose_backup/`), then RE-MEASURE the 3
      anon/recycle conformance tests on the FIXED harness.

## LIVE STATE (attempt 2)
ALL 7 conflict files have NO markers left. `go build ./...` currently FAILS with
exactly the predicted hazard (proving the point):
```
task_runtime.go:357 too many arguments in call to s.deferAnonGC
task_runtime.go:478 too many arguments in call to s.deferPendingWaifs
waif_lifecycle.go:121 s.liveTaskVMs undefined
waif_lifecycle.go:124 not enough arguments in call to s.liveWaifs
```
i.e. master's deferred-GC code (auto-merged, no markers) vs this branch's
race-free root API. Now wiring them together.

Done since last checkpoint:
- task_runtime.go suspend path: `s.deferAnonGC(ctx, anonGCFloor, nil)` (suspended
  task's VM stays registered via SetBytecodeVM, so flush reads it as a root),
  keeping the branch's txn commit/Flush* block.
- task_runtime.go suspend path: **added `t.CreatedForks = nil` after a successful
  `ctx.StoreTxn.Commit()`** — this is the Barn fix that lets master's real-suspend
  yin work here (commit publishes the forks; a later conflict-retry must not
  discard them). Replaces the branch's old yin-inline clearing.
- task_runtime.go completion path: `s.deferPendingWaifs(ctx, ..., bcVM)` +
  `s.deferAnonGC(ctx, anonGCFloor, bcVM)` + inline `s.flushDeferredGC()`
  (keeps :recycle promptness). bcVM passed so its refs are snapshotted NOW, since
  the VM is released just below.
- scheduler.go: `s.flushDeferredGC()` added at end of ProcessReadyTasks (quiescent
  point, after runReadyTasks joins all workers).
- scheduler.go: NEW `collectAllGCRefs() (anonRefs, waifRefs, ok)` — walks EVERY
  live task's saved VM under s.mu; returns ok=false if ANY task is TaskRunning.

STILL TO WIRE:
- `waif_lifecycle.go`: pendingWaifEntry += ownRefs; deferPendingWaifs(ctx,pending,ownVM);
  deferAnonGC(ctx,minID,ownVM); flushDeferredGC rewritten to use collectAllGCRefs()
  + bail when !quiescent (leave batches queued) + s.liveWaifs(roots).
- `vm/anonymous_gc.go`: AnonGCRequest += OwnRefs map[types.ObjID]struct{};
  RecycleOrphanAnonymousBatch(..., siblingRefs map) replacing (..., liveVMs ...*VM).

## MEASURED ON THE FIXED HARNESS (managed mode, fresh Test.db per run)
| tree | failures |
|---|---|
| master `c099532` | **0** (11335 passed) |
| pre-merge branch `f4fc1cb` | 1173 |
| merged (attempt 2, both master commits KEPT) | 24 (11311 passed) |

Diff of merged vs pre-merge branch:
- **19 inherited** branch bugs (also fail at `f4fc1cb`): `limits::*max_value_bytes`,
  `exec::*`, `anonymous_recycle_hook_runs_once`, `recycled_anonymous_saved_in_verb`,
  `waif_object_call_ignores_waif_only_verbs`.
- **3 INTRODUCED by the merge** (pass at `f4fc1cb`, pass on master):
  - `anonymous::anonymous_lost_reference_calls_recycle`
  - `recycle::recycling_anonymous_recycles_property_values_on_object`
  - `recycle::recycling_anonymous_recycles_property_values_on_parent`
  These are the SAME 3 from attempt 1. **They are REAL, not harness artifacts.**
  My attempt-1 excuse ("measured on a broken harness") was wrong.

## ROOT CAUSE FOUND (in AUTO-MERGED code, not in any hunk I resolved)
`Store.HasAnonymousAtOrAbove(minID)` is **master-only** (`git show HEAD:db/store/
store_reachability.go | grep -c HasAnonymousAtOrAbove` => 0). Master added it as a
fast-path guard at the top of `AutoRecycleOrphanAnonymousSince`:
```go
if !store.HasAnonymousAtOrAbove(minID) { return }   // master; HEAD had no such guard
```
Git auto-merged that guard INTO this branch's version of the function (which
takes `siblingRefs`), with no conflict marker. `eval.go` still sweeps
synchronously and is otherwise untouched — which is why the deferral was never
the culprit and why my "eval.go never calls runTask, so I can't explain it"
was the moment I should have kept digging instead of deleting master's commit.

`HasAnonymousAtOrAbove` compares `obj.id >= minID` over live anonymous objects.
Callers pass `minID = anonGCFloor = s.store.NextID()` (the PERSISTENT id counter).
NEXT STEP: confirm whether this branch allocates anonymous ids from a DIFFERENT
space than `NextID()` (branch tracks `AnonCreationCount()` separately, and
`rangeAnonymousLocked` walks a separate `s.anonObjects` map). If anon ids do not
come from the NextID space, the guard is always false for fresh anon objects =>
sweep skipped => `:recycle` never runs => exactly these 3 failures.
Fix belongs in the guard/floor, NOT by deleting master's fast path.

## DEBUGGING THE 3 INTRODUCED FAILURES (live)
Hypotheses tested, in order:

1. **master's `HasAnonymousAtOrAbove` fast-path guard skips the sweep.** KILLED.
   Probe: `if false && !store.HasAnonymousAtOrAbove(minID)` in
   `AutoRecycleOrphanAnonymousSince`, rebuild, run the 3 tests => STILL 3 failed.
   Guard is not the cause. Probe reverted.
   (Anon ids DO come from the same allocator: store_lifecycle.go:52 puts them in
   `s.anonObjects[newID]` and bumps `highWaterID`, so `NextID()` floor is sound.
   `AnonymousRecycleCandidates` and `HasAnonymousAtOrAbove` use the same
   `rangeAnonymousLocked` + `obj.id >= minID` filter — they agree.)

2. **`eval.go` is the path.** RULED OUT. `git diff HEAD -- scheduler/eval.go` is
   empty; its synchronous sweep is untouched. `vm/anonymous_gc.go` vs HEAD is
   ONLY additions (AnonGCRequest, RecycleOrphanAnonymousBatch) + that one guard.
   => the failing tests must run through the TASK path (`runTask`), not eval.

3. **CURRENT HYPOTHESIS — transaction ordering.** This is the real reconciliation
   problem between master's batching and this branch's MVCC:
   - HEAD ran `AutoRecycleOrphanAnonymousSince(...)` **BEFORE**
     `ctx.StoreTxn.Commit()` in the completion path, so the `:recycle` verb's
     property writes (`recycle_called = recycle_called + 1`) were part of the
     task's transaction and got committed with it.
   - My deferred `flushDeferredGC()` runs **AFTER** the commit (task_runtime.go:516,
     after `SetState(TaskCompleted)` at :455), and `RecycleOrphanAnonymousBatch`
     calls `recycleFn(req.Ctx, ...)` reusing that task's now-committed/released
     `ctx.StoreTxn`. The anon IS recycled, but `:recycle`'s writes land in a dead
     txn and are lost => `recycle_called` stays 0 => exactly these 3 failures.
   - Note `callWaifRecycle` already sidesteps this: it builds a fresh
     `kernel.NewTaskContext()` with Store/Registry and NO txn, so its writes go
     straight to the live store. The anon path does not.
   PROBE IN PLACE: null out `anonBatch[i].Ctx.StoreTxn` before the batch call.
   If the 3 pass => hypothesis confirmed.
   PROPER FIX (not the probe): build a dedicated recycle context at flush time
   (as `callWaifRecycle` does) instead of mutating a task's ctx — the SUSPEND path
   also defers anon GC, and that task's ctx still holds a LIVE txn that must not
   be clobbered.

## ROOT CAUSE CONFIRMED + FIXED (the 3 introduced failures)
Hypothesis 3 was RIGHT, proven by probe (null out `anonBatch[i].Ctx.StoreTxn`
before `RecycleOrphanAnonymousBatch`) => **3 passed**.
Mechanism: inline GC ran BEFORE `ctx.StoreTxn.Commit()`, so `:recycle`'s property
writes joined the task's txn. A DEFERRED flush runs AFTER commit+release, and
`recycleFn(req.Ctx, ...)` reused that dead txn — the anon was recycled but
`:recycle`'s side effects (e.g. `recycle_called + 1`) were silently discarded.
This is the true master-batching vs MVCC-branch conflict; nothing to do with
`liveTaskVMs`, the fast-path guard, or eval.

PROPER FIX (probe reverted): new `Scheduler.gcRecycleContext(parent)` in
`scheduler/waif_lifecycle.go` — derives a fresh `kernel.NewTaskContext()` carrying
Player/Programmer/IsWizard/ThisObj/Task/TaskID/Store/Registry/RuntimeOptions and
**no StoreTxn**, so deferred `:recycle` writes go straight to the live store
(exactly what `callWaifRecycle` already did for waifs). `deferAnonGC` stores that
context instead of the task's. The task's own ctx is NEVER mutated — the suspend
path also defers and its txn is still live.

## GATE STATUS (managed mode, fresh Test.db per run)
| tree | failures |
|---|---|
| master `c099532` | 0 |
| pre-merge branch `f4fc1cb` | 1173 |
| **merged + fix** | **19, ALL of which also fail at `f4fc1cb`** |
=> **0 regressions introduced by the merge.** Both master commits (`685c5f4`
batched GC, `3c16e48` real-suspend yin) are KEPT.
The 19 are pre-existing branch bugs master doesn't have (`limits::*max_value_bytes`,
`exec::*`, `anonymous_recycle_hook_runs_once`, `recycled_anonymous_saved_in_verb`,
`waif_object_call_ignores_waif_only_verbs`). Fixing them is separate work, NOT
merge resolution. Q's "master is 0" gate is met for everything the merge touches.

- `go build ./...` clean. `go vet` only the 2 pre-existing nits.
- `go test ./scheduler -race`: **no data race**.
- `go test ./...`: fails `TestReview_IDCollision...` (also fails at `f4fc1cb`,
  pre-existing) + 2 yin unit tests — being rewritten now, see below.

## REWRITING THE 2 YIN UNIT TESTS (not weakening them)
They encoded the branch's old inline-yielder contract. Master's yin suspends.
Invariants they really guard are PRESERVED and must still be asserted:
commit at yield, fork handoff (`CreatedForks` cleared), snapshot refresh, ordering.
- `TestYinCommitsAndRefreshesAroundReadyTasks`: now asserts CreatedForks handed off
  after the suspending slice, then drives `s.ProcessReadyTasks()` until the task
  completes, then asserts the SAME ordering {main-before, fork, main-after}.
  `runTask` registers the suspended task (task_runtime.go:733) so it resumes.
- Off-by-one discovered: branch yielded at `ticks <= minTicks`; Toast yields when
  `ticks < minTicks`. The test's `yin(0, 59999, 4)` sat exactly on 59999 ticks
  remaining, so under Toast semantics it does NOT yield. `60000` is rejected by
  master's own validation (`minTicks >= fgTicks` => E_INVARG, fgTicks=60000).
  Fix: burn ~50 ticks (`for i in [1..50] endfor`) before the yin so the budget
  drops below the threshold. This is correcting a constant tuned to a Barn bug,
  NOT weakening an assertion.
- `TestYinFlushesCommittedForksBeforeLaterConflict`: STILL TO REWRITE.

## NEXT
3. Restore compose from `<scratchpad>/compose_backup/`, build, and run managed
   conformance. If the 3 tests pass -> harness artifact, compose stands.
   If they fail -> debug them; do NOT delete master's commit.
4. Gate: managed conformance = 0 failures; `go test ./...` no worse than
   `f4fc1cb` (which fails only
   `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`);
   `go test ./scheduler -race` clean.
5. Only then stage + `git merge --continue`. NOTE: `git add` is blocked in the
   Bash tool ("Only touched or explicitly adopted paths may be staged") but works
   via the PowerShell tool.
