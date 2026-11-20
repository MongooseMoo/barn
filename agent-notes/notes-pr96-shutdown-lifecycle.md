# PR 96 shutdown lifecycle repair

- Scope: only issue #95 shutdown lifecycle/persistence in `barn-pr96-normal-merge-update`.
- Starting head: `8bd26306dc8f0860a12e1bb9722139998ea6924d`.
- Current finding: independent review reports `Server.shutdown()` checkpoints before `scheduler.Stop()` cancels and joins sibling tasks, allowing pending roots to be appended after the last checkpoint.
- Required proof: a wired shutdown regression that persists and reloads the output DB, failing under the old ordering.
- Blocker: none at assignment time.
- Next action: inspect current shutdown, scheduler stop, and issue #95 tests in the isolated worktree; add the red regression before production changes.
- This note is intentionally uncommitted and must never be staged or pushed.

## Current state

- Replaced the synthetic direct-sink regression with a wired `Server.shutdown()` test using two scheduler VMs and a persisted/reloaded `.new` database.
- Red proof at old ordering: final database contained zero pending roots.
- Prototype scheduler-owned quiescent capture makes focused test and existing checkpoint-hook contract green; capture fails closed on `TaskRunning`.
- Important semantic concern under review: suspended tasks are checkpointed and resume after restart, so treating their VM locals as pending finalizations may be too broad even though it proves the persistence boundary. The production defect described by review concerns running sibling tasks canceled/joined at shutdown.
- Current blocker: need narrow the regression/capture to expiring shutdown VMs without incorrectly finalizing resumable suspended-task roots.
- Next action: inspect task persistence and shutdown ownership, revise test to model real running/canceled siblings or explicitly distinguish resumable tasks, then rerun red/green before retaining the implementation.

## Oracle adjudication and final disposition

- Caller-free managed pinned Toast result: `0 values pending finalization`, `2 suspended tasks`, and both anonymous cycle members serialized.
- Current Barn/PR115 matches that exact behavior.
- The experimental pre-checkpoint capture would have duplicated resumable task roots as pending finalizations and was therefore invalid.
- All uncommitted experimental edits were reverted from the isolated PR96 worktree; it is clean at exact head `8bd26306dc8f0860a12e1bb9722139998ea6924d`.
- No commit or push was made.
- Next action: retain the existing PR96 implementation and use the caller-free conformance case to guard the distinction between pending finalization roots and serialized suspended-task roots.

## Real Anon7 serialization gap

- Revised caller-free managed oracle: pinned Toast writes `0` pending finalizations and `2` suspended tasks, with both A/B cycle payloads serialized.
- Current integrated Barn matches the classification (`0` pending, `2` suspended) but omits one anonymous object payload while a task record still references it.
- Correct scope: no shutdown-order or pending-root change. Serialized queued/suspended task values must seed the existing store anonymous reachability plan and be rewritten with the same identity-to-serialization mapping.
- Proposed ownership: `db/format.WriteCheckpoint` coordinates the store snapshot with task snapshots; `task.Snapshot` owns one reusable traversal/transform over every persistence value surface; `db/store` remains the sole anonymous graph planner/rewriter.
- Required surfaces include queued fork variables/activation this-value and suspended wake/task-local plus VM locals, operand stack, this/args, program constants, pending error, and saved this-value.
- Blocker: none. No implementation has been made yet for this revised gap.
- Next action: add a real checkpoint/reload red test for two suspended task VMs holding opposite members, then implement the shared-root snapshot/rewrite path and verify managed Anon7 at suite head `ac02fbfe3fba6115e2ec90809f1a9f63fd17346d`.

## Implementation progress

- Red checkpoint/reload proof: classification stayed `0` pending and `2` suspended, but `AnonymousObjs` was `0`, expected `2`.
- Green focused prototype now routes task values through `db/format.WriteCheckpoint`, which accepts the live `*store.Store`, gathers task roots through one `task.Snapshot.TransformPersistenceValues` traversal, calls `Store.SnapshotWithRoots`, then rewrites the same task surfaces with the returned snapshot mapping.
- The store remains the sole owner of anonymous reachability, serialization ID assignment, object rewriting, and external value rewriting.
- The real regression now passes and verifies both task-held values use emitted IDs, both marker payloads survive, and both cycle edges are complete, while classification remains unchanged.
- Current concern under audit: ensure the task traversal matches every value actually emitted by queued, suspended, and interrupted writers without retaining non-serialized values; ensure snapshots are detached before mutation.
- Blocker: none.
- Next action: add exhaustive traversal coverage, run focused format/store/task tests, then managed Anon7 red/green and full gates before commit/push.

## Persistence-surface audit

- `TransformPersistenceValues` now distinguishes queued records (`Fork.Variables` plus queued activation `ThisValue`) from VM-backed suspended/interrupted records.
- VM-backed surfaces covered: ordinary suspended wake value, task-local value, program constants, locals, operand stack, frame this-value, args, pending error value, and saved this-value.
- Interrupted records deliberately exclude `WakeValue` because the writer replaces it with `E_INTRPT`; task-local and VM surfaces remain included.
- `Snapshot.IsInterruptedForPersistence` centralizes the same classification used by the writer and traversal.
- Focused format regression remains green. The new exhaustive traversal test exposed a test-fixture initialization issue: a zero `ReadingPlayer` classifies as interrupted because the actual default is `ObjNothing`; set ordinary suspended fixtures explicitly to `ObjNothing`.
- Blocker: none; this is a test construction correction, not a production failure.
- Next action: correct the fixture, repeat focused gates, then run managed Anon7 at the pinned suite head.

## Explicit GC root gap and focused repair

- Managed Anon7 exposed a second, earlier loss boundary: its explicit `run_gc()` recycled both A/B anonymous objects before shutdown, so task-local values reached the checkpoint as `TYPE_ANON` with `ObjNothing` identities.
- Red proof: `TestExplicitRunGCPreservesAnonymousCycleHeldBySuspendedSiblingVMs` failed because both objects held only by suspended sibling VMs were recycled.
- Narrow repair: the existing `run_gc` registry hook now collects the existing scheduler sibling GC references and passes them to `AutoRecycleOrphanAnonymousSince`; that function already includes the caller VM through `TaskContext`.
- Green proof: the new scheduler regression passes 20 consecutive runs. The combined scheduler/task/format/store/server focused set also passes 20 consecutive runs.
- No pending reclassification or shutdown-order change is included.
- Next action: rebuild the managed Barn binary, rerun pinned capability admission plus Anon7 through the documented conformance harness, then run full Go gates and audit the final diff before commit/push.

## Verification and audit status

- Rebuilt `.tmp/barn-pr96.exe` from the current worktree.
- Official managed conformance run at suite head `ac02fbfe3fba6115e2ec90809f1a9f63fd17346d` passed both selected tests: capability admission and `startup_repair_anon7`; summary was `2 passed, 11584 deselected`, with strict markers and fail-on-unexpected-skip enabled.
- Full `go test ./... -count=1` passed.
- Touched-package race gate passed: `go test -race ./scheduler ./task ./db/store ./db/format ./server -count=1`.
- `go build ./...`, scoped `go vet`, gofmt check, and `git diff --check` all passed.
- Audit observation: the new explicit-GC regression currently proves the red/green object-liveness boundary, but it should also assert that the evaluated `run_gc()` command completed successfully so a future command failure cannot create a false green.
- Blocker: none.
- Next action: strengthen that assertion if the eval result surface permits it, rerun focused proof, complete final diff/commit audit, then commit and push only the intended PR96 files.

## Commit and push state

- Strengthened the explicit-GC test to require exact successful eval output `{1, 1}`; it remains green for 20 consecutive runs.
- Final staged scope contained exactly nine intended files and excluded this notes file plus all `.tmp` evidence.
- Created commit `4f6e82878be71b9be289a80c268e98a100ed9285` (`fix: preserve task-rooted anonymous graphs`).
- Verified the remote PR branch was still exactly at the starting head `8bd26306dc8f0860a12e1bb9722139998ea6924d`, then pushed normally without force to `agent/issue-95-pending-finalizations`.
- Local branch and remote PR branch now agree at `4f6e82878be71b9be289a80c268e98a100ed9285`; worktree is clean.
- Blocker: none.
- Next action: update PR #96 body with accurate current behavior, exact managed Anon7 proof, full Go/race/build/vet gates, and retained literal `Closes #95`; then verify GitHub PR files/commits/check state. Do not mark ready or merge.

## Independent-review concurrency repair

- Review found that explicit global `run_gc()` incorrectly reused `collectSiblingGCRefs`, whose intentional per-task behavior skips running siblings. A global sweep from ID 0 cannot safely ignore a running sibling because it may hold an existing anonymous candidate.
- Added deterministic red proof: create an anonymous object, save it in a suspended sibling VM, mark that saved task `TaskRunning`, remove the persistent root, then invoke `run_gc()` from eval. At head `4f6e828`, the eval succeeded but recycled the object.
- Implemented a dedicated `collectExplicitGlobalGCSiblingRefs` under `s.mu`. It excludes the caller, skips terminal tasks, returns `ok=false` on any other live `TaskRunning`, and otherwise collects anonymous refs from queued/suspended saved VMs.
- The explicit `run_gc` hook returns success without calling the sweep when `ok=false`; the quiescent path still sweeps from `minID=0`. Existing `collectSiblingGCRefs` semantics are unchanged for per-task floor GC.
- Both explicit-GC regressions pass 20 consecutive runs: suspended sibling roots remain live during a quiescent sweep, and a running sibling makes global GC fail closed.
- Full `go test ./... -count=1` passed. Full scheduler race test passed in 248.544 seconds.
- Blocker: none.
- Next action: run build, scoped vet, gofmt/diff checks; audit the two-file delta; commit and push normally to PR #96; update its body and monitor CI without marking ready or merging.

## Physical execution invariant repair

- Second exact-head review found a deeper race: `builtinSuspend` changes logical task state to `TaskSuspended` before the VM returns to `runTask`, while VM mutation, transaction handling, `SetBytecodeVM`, and queue publication continue afterward. Logical state is therefore not a physical execution lease.
- Required design is a scheduler-owned active-execution set under `s.mu`, acquired atomically with the `TaskRunning` transition at `runTask` entry and released by defer only when the entire invocation returns, including every error, panic, retry, suspend-save, and queue-publication path.
- `collectExplicitGlobalGCSiblingRefs` must fail closed on an active sibling; `collectAllGCRefs` must fail closed on any active task; the existing per-task `collectSiblingGCRefs` must skip active siblings without changing its separate floor-GC semantics.
- Deterministic test design avoids production suspend hooks: register a test-only builtin that calls the real `Manager.SuspendTask`, signals a barrier, and blocks before returning `FlowSuspend`. While logical state is suspended but physical execution remains active, concurrent eval `run_gc()` must succeed without sweeping either the VM-only root or a separate orphan. After releasing the barrier and completing VM handoff, a second global GC must retain the task root and collect the orphan.
- Current worktree remains clean at `47aefbb`; no implementation has been made for this second repair yet.
- Blocker: none.
- Next action: implement the scheduler execution set and real-transition regression, capture red/green, then repeat race/full/build/vet/format gates before normal commit/push and PR body update.

## Refcounted execution-lease implementation and proof

- Implemented `Scheduler.executingTasks` as a refcount map under `s.mu`, initialized by the scheduler constructor. `runTask` acquires it atomically with logical `TaskRunning` and defers release across all exits; conflict retries retain the physical lease while restoring logical running state.
- Refcounts are required because open issue #30 permits duplicate physical executions of one task ID: the first finisher must not clear the second lease. Explicit global GC excludes its caller ID only when that ID's count is exactly one; count greater than one fails closed.
- Replaced the manufactured `SetState(TaskRunning)` test with a real resumed-VM transition. A test-only builtin calls the real `Manager.SuspendTask`, signals after logical suspension, and blocks before returning `FlowSuspend`/handoff. The task's previously saved VM is therefore still the actively executing VM.
- Stronger RED with physical checks disabled: the per-task collector scanned the active sibling and returned `map[1:{}]`. Earlier global-boundary RED recycled the VM-only anonymous object during the same handoff class.
- GREEN assertions: per-task collector skips the active VM; deferred GC reports non-quiescent; eval `run_gc()` succeeds without sweeping the VM-only root or a separate orphan; after barrier release the VM is saved suspended, and a second global GC retains the task root while collecting the orphan.
- Added focused refcount assertions: duplicate caller count 2 blocks explicit global and deferred GC; sole caller count 1 may be excluded only by explicit global GC while deferred GC remains blocked; count 0 unblocks deferred GC.
- All three focused tests pass 20 consecutive runs. Full `go test ./... -count=1` passed. Full `go test -race ./scheduler -count=1` passed in 250.003 seconds.
- Existing per-task floor-GC behavior is otherwise unchanged; issue #30 dispatch itself is not repaired here.
- Blocker: none.
- Next action: run build, scoped vet, gofmt/diff checks; audit the three-file diff; commit/push normally; update PR #96 body with current CI breakdown and monitor rerun without marking ready or merging.

## Deferred-GC lifecycle ordering repair

- Final review found `runTask` called `flushDeferredGC` inside its body while its physical lease defer was still active. `collectAllGCRefs` therefore correctly failed closed, but synchronous callers had no guaranteed later scheduler-pass flush and could return with orphan/waif batches unsettled.
- Added direct synchronous RED at head `86e0911`: a task created and dropped anonymous object `#1`; `runTask` returned while the object remained valid.
- Central repair uses one top-level `runTask` lifecycle defer for every caller. After all inner recovery, transaction, escalation-gate, cancellation, suspend-save, queue-publication, and completion work finishes, it releases the physical execution lease first and then calls `flushDeferredGC`. The old in-body flush is removed; the scheduler-pass flush remains a fallback.
- Added panic-path proof: a task defers anonymous GC and then panics in `OnComplete`; existing recovery marks it killed and returns the recovered error, after which the lifecycle defer releases the lease, recycles the orphan, and leaves no pending anonymous batch.
- The direct synchronous and panic-path lifecycle tests plus the suspend-handoff, suspended-sibling, and duplicate-refcount tests pass 20 consecutive runs.
- Full `go test ./... -count=1` passed. Full `go test -race ./scheduler -count=1` passed in 252.282 seconds.
- Current legacy CI baseline at `86e0911`: 86 conformance failures total, comprising 71 `ONLY_32_BITS`, 8 bcrypt, 3 cyclic-GC, and 4 waif failures; summary `11104 passed, 396 skipped`.
- Blocker: none.
- Next action: run build, scoped vet, gofmt/diff checks; audit the four-file delta; commit/push normally; update PR #96 body and current rerun status while retaining draft and `Closes #95`.

## Sweep/start exclusion and direct-VM ownership repair

- Exact-head review at `cc436313` found a root-snapshot TOCTOU: deferred/explicit/inline GC released `s.mu` after capturing roots, so a new VM could acquire VM-only anonymous or waif roots while the sweep was still running.
- Deterministic RED at `cc436313`: `TestDeferredGCSweepBlocksNewVMStartUntilSweepCompletes` blocked inside the actual deferred sweep's first recycle callback after root capture; a new task VM entered and created anonymous object `#2` before the sweep released.
- Implemented scheduler-owned `gcSweepMu` and `vmStartMu` with lock order `gcSweepMu -> vmStartMu -> s.mu -> task locks`. Deferred sweeps hold both from before quiescence/root capture through batch drain and recycle hooks; explicit `run_gc` uses `gcSweepMu.TryLock` so reentrant/concurrent sweeps succeed as no-ops; every new VM execution lease publishes under `vmStartMu`.
- Added scheduler-private context provenance. Sweep-owned recycle contexts propagate the existing barrier to nested standalone verb VMs without reacquiring `vmStartMu`; execution-owned Eval/server-hook contexts propagate their opaque lease ID. Sweep ownership takes precedence while an executing context temporarily owns both.
- Eval and public `CallVerbWithArgstr` now own physical execution leases for their full direct-VM lifecycle. A synchronously nested standalone VM increments the same lease refcount without reacquiring `vmStartMu`; this makes nested `run_gc` fail closed while both outer and inner VMs hold roots and `TaskContext.CallerVM` can name only the inner VM.
- Eval floor GC now runs under both sweep/start barriers and fails closed on any physical sibling lease. If non-quiescent it defers waif/anonymous requests with its own VM roots; its lifecycle releases the lease and retries the deferred flush.
- GREEN x20: the actual-sweep task-start regression, direct Eval-start barrier regression, reentrant `run_gc` recycle-hook assertion, nested Eval verb `run_gc` outer-root regression, and existing global/deferred refcount tests.
- Full scheduler package test is green after the implementation (`go test ./scheduler -count=1`).
- Blocker: none.
- Next action: add/confirm direct server-hook path coverage, run focused race x20 and full race/full/build/vet/format gates, audit, commit/push normally to PR #96, and update the draft PR body without merging.

## Sweep barrier audit refinements in progress

- Deterministic start exclusion now covers ordinary scheduled tasks, Eval's direct VM, and public server-hook `CallVerbWithArgstr`; all remain blocked while a deferred sweep is paused inside a recycle callback after root capture, then enter after barrier release.
- Real registry-hook coverage uses `create()->initialize`: both Eval (nil transaction) and runTask (shared transaction) hold an outer-only anonymous local while the initialize hook calls `run_gc`. Nested standalone VMs increment the same opaque execution lease without reacquiring `vmStartMu`, so global GC fails closed while outer and inner VM roots coexist.
- Store-transaction routing remains first in `SetVerbCaller`. `CallVerbInContext` itself recognizes sweep ownership, preserving shared transaction/caller semantics without reacquiring the start barrier. `TestRunGCSweepRecycleHookRetainsTaskTransaction` proves a recycle-hook write remains visible in the caller transaction and commits as value 42.
- Public/nested standalone verb ownership is published immediately after lightweight task creation, before trace, verb lookup/compile, anonymous target inspection, or argument use; cleanup remains armed before publication.
- Post-audit focused lifecycle/barrier/nested-GC suite passed 20 consecutive runs. Two scheduler race runs passed (250.465s and 248.742s) but were invalidated by later provenance-audit corrections and are not acceptance evidence.
- Current blocker being repaired: `executionOwnedContexts` must be per-context, per-owner refcounted. Duplicate runTask executions can share one TaskContext; one exit must not erase the other claimant. Unknown-owner releases must not remove real claims, unique remaining ownership must recover, and multi-owner ambiguity must make explicit GC a successful no-op.
- A separate possible stale-reachability defect in batched anonymous GC is paused. No `vm/anonymous_gc.go` change is authorized until the required managed WSL Toast oracle proof confirms callback-created persistent anonymous-object behavior.
- Next action: finish per-owner context provenance and its ambiguous-explicit-GC regression, rerun focused x20, scheduler race, full Go/build/vet/format gates, then stop for independent diff review before any commit or push.

## Toast-authorized final sweep snapshot work

- Managed WSL Toast oracle is authoritative GREEN for exact admission plus `builtins/gc.yaml::gc_recycle_hook_created_anonymous_survives_current_sweep`: `2 passed, 42 deselected` in 1.96s. The case releases a self-cycle, invokes `run_gc`, yields with `suspend(0)`, then proves one callback and a callback-created persistently stashed anonymous value of `TYPE_ANON` remains valid.
- Authorized Barn repair now freezes every deferred anonymous-GC request's candidate slice against the one persistent reachability snapshot before any recycle callback. Request order, request context, and cross-request recycled-ID deduplication remain unchanged.
- Added VM regression with two requests: the first recycle callback creates a new anonymous object and stores it persistently; the later request must not discover/recycle it from stale reachability. Expected callbacks are only the two pre-snapshot candidates.
- Replaced 100ms scheduler absence sleeps with a test-only pre-`vmStartMu` acquisition observer. Task, Eval, and public server-hook barrier tests now wait until the real lease path has attempted start, then assert VM entry has not occurred; deadlines remain only deadlock watchdogs.
- Per-context provenance is now per-owner refcounted. Unknown-owner release retains real claims, one remaining owner is uniquely attributable, and multiple owners are explicitly ambiguous. Nested execution under ambiguity installs a reserved blocker lease/claim; its regression pauses inside a real `CallVerbInContext`, removes the other outer claimant, then proves nested `run_gc` still no-ops and blocker cleanup restores unique attribution.
- Added a deterministic retryable MVCC conflict regression. It forces one live-store version change without marking an irreversible mutation, observes provenance on both attempts, and will prove `retryState.restore` replaces the TaskContext, removes the old claim, attributes the replacement, and clears both claims at lifecycle completion.
- Current state: implementation and tests are written but not yet compiled after the latest authorized patch.
- Blocker: none.
- Next action: gofmt and run the new VM, ambiguity, retry, and deterministic barrier tests; repair any fixture errors; then focused x20, scheduler race, full Go/build/vet/gofmt/diff gates, and independent review. Do not commit or push before review.

## Final pre-review gate state

- Focused GREEN x20: `TestRecycleOrphanAnonymousBatchFreezesCandidatesBeforeRecycleHooks` plus the complete scheduler lifecycle/start-barrier/global-GC/context-refcount/ambiguous-nested/retry-provenance set.
- Final scheduler race GREEN: `go test -race ./scheduler -count=1 -timeout=15m`, package duration 258.468s.
- Touched VM regression race GREEN x20: `go test -race ./vm -run TestRecycleOrphanAnonymousBatchFreezesCandidatesBeforeRecycleHooks -count=20 -timeout=5m`, package duration 1.555s.
- Full repository GREEN: `go test ./... -count=1 -timeout=15m`; scheduler package 19.268s and VM package 1.310s within the full run.
- `go build ./...` GREEN. `go vet ./scheduler` GREEN. `go vet ./vm` remains nonzero on the pre-existing unrelated `vm/stack.go:49:15` warning that `ReadByte() byte` should have signature `ReadByte() (byte, error)`; no stack.go change is in this patch.
- Gofmt check over all ten intended files and `git diff --check` are GREEN.
- Local and remote PR branch remain unchanged at `cc436313d892cba6b6e2cb6385849e9a360f0392`. Worktree contains nine modified intended files plus untracked intended `scheduler/execution_provenance_test.go`; nothing is staged, committed, or pushed.
- Blocker: final fresh exact-diff review requested by parent before commit/push.
- Next action: report exact gates/scope to parent, wait for independent review, repair only confirmed findings, then repeat affected gates. Do not commit or push until explicitly cleared.
