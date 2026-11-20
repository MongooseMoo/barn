# PR 96 pending-roots repair checkpoint

## Current findings

- Rejected head under correction: `6f1022f1c6131a7e5ef6197499f211c3c67e0723` in `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair`.
- Red-first contracts reproduced all requested blockers: ordinary popped WAIF recycle suppression, deferred shutdown transfer loss, nested ANON flattening under WAIF, cancellation incorrectly implying shutdown, noncanonical task-local owners, panic checkpoint ordering, and numbered anonymous storage acceptance.
- The correction now separates direct shutdown roots from ordinary recursive liveness, atomically chooses deferred-GC versus checkpoint ownership, transfers preexisting WAIF/ANON requests on `BeginShutdown`, and publishes shutdown before panic checkpointing.
- `task_local` and `set_task_local` now require `*task.Task`; VM task-local root collection uses the same canonical owner.
- VM anonymous fixtures now use `CreateObject(..., true)`; `Store.Add` rejects anonymous objects and anonymous reachability scans only `anonObjects`.

## Verification state

- Focused affected packages: green.
- Focused correction contracts: green for 20 repeats.
- Focused race contracts: green, including 10 repeats after the ownership linearization review.
- Full `go test ./...`: green after the final lock-scope correction.
- `go build ./...`: green.
- `go vet ./...`: nonzero only for the preexisting `vm\stack.go:49:15` `ReadByte` signature warning; no task files overlap it.
- Managed Anon1/Anon2/Anon6 run `20260805_051422`: 3 passed, 11576 deselected, but it preceded the final lock-scope refinement and will be rerun.

## Blocker

- No implementation blocker. The repository-wide vet baseline remains nonzero on the unrelated existing warning.

## Next action

- Rerun documented managed Anon1/Anon2/Anon6 on the final code, audit diff/status, stage only the 16 intended source/test files, create a new local correction commit, and report exact head/files/provenance without pushing.

## 2026-08-05 PR45 fixture correction

### Current findings

- The subsequently rejected local head is `8b774ce65dae7567e254a367040402713bfd3863`; its condition-variable shutdown barrier self-deadlocks when `shutdown()` runs inside a deferred WAIF `:recycle` hook.
- Exact conformance PR45 commit `1cd9dd26ba9bca9287149b97ded3a01de4ab158d` establishes three contracts: recycle-hook shutdown returns and produces zero pending roots; panic with a suspended WAIF plus anonymous object produces zero pending roots and one suspended task; a pending outer WAIF containing an inner WAIF remains one top-level root.
- `Store.normalizePendingFinalizationsLocked` recursively promoted nested WAIFs. The worktree now has a direct-WAIF-boundary collector and a regression preserving the nested topology under one outer root.
- Suspended task values were not roots of the anonymous serialization plan, so task-owned anonymous objects could not be emitted/rebound without promotion. The worktree now has task snapshot root enumeration, serialization-only snapshot roots, task-value rewrite support in the database writer, and deterministic task snapshot ordering.

### Current state

- Modified but uncommitted worktree files: `db/format/writer.go`, `db/store/store_reachability.go`, `db/store/store_snapshot.go`, `db/store/store_test.go`, `scheduler/task_factory.go`, `server/server.go`, and `task/snapshot.go`.
- `reports/runs/` remains untracked and excluded. Both notes files in the original checkout remain untracked and must not be staged.
- A focused package test was started before the missing `sort` import was corrected; its result still needs to be collected/rerun.

### Blocker

- No external blocker. The reentrant shutdown publication model and lifecycle handoff are not implemented yet, so the PR45 recycle-shutdown contract still fails at this checkpoint.

### Next action

- Replace the blocking shutdown condition with a single asynchronous publisher and completion channel, mark deferred-GC contexts, defer server lifecycle finalization when shutdown is requested by the GC owner, then add concurrent flush and sink-arrival regressions before running focused/race/full/managed verification.

## 2026-08-05 implementation and verification checkpoint

### Current findings

- `BeginShutdown` now returns a completion channel. A non-GC caller publishes synchronously; a deferred-GC owner returns without waiting on itself, and the collector publishes only after the recycle hook and sweep complete.
- Server lifecycle callbacks recognize deferred-GC contexts and wait in a tracked background goroutine before clean shutdown or panic checkpointing. Normal lifecycle callers still wait synchronously for the boundary.
- The publisher drains the sink outside the GC mutex, then rechecks under the producer lock. A regression caught and led to correction of a missing unlock on the multi-pass path.
- Suspended-task anonymous requests are marked task-owned and excluded from shutdown-root promotion. Task snapshots enumerate serialization roots, `Store.SnapshotWithRoots` emits anonymous objects reachable from those values without making them pending, and the writer rewrites only task-owned values through that snapshot plan.
- Pending WAIF normalization now stops at the outer WAIF boundary, preserving a nested WAIF within one top-level pending root.

### Verification state

- New shutdown regressions pass for 10 repeats: claimed-GC boundary return, deferred WAIF recycle shutdown return/publication, roots arriving during a blocked sink, and concurrent flush single-claim.
- Focused race regressions pass for 5 repeats across scheduler/store/format/server.
- Suspended task checkpoint round trip passes with zero pending roots, one suspended task, and one serialized anonymous object.
- All modified packages pass, full `go test ./...` passes, and `go build ./...` passes.
- Full `go test -race ./... -count=1 -timeout=300s` is still running in scheduler. A separate older scheduler test process from the pre-fix combined run remains stuck on the already-corrected missing-unlock bug and should be terminated by exact PID without touching the current race process.

### Current state and blocker

- Thirteen intended source/test paths are modified/untracked in the repair worktree; `reports/runs/` remains untracked and excluded.
- No code blocker is known. Remaining verification is full-race completion and the managed PR45 three-fixture run.
- Exact PR45 conformance worktree already exists clean at `C:\Users\Q\code\.worktrees\moo-conformance-shutdown-gc-edges`, head `1cd9dd26ba9bca9287149b97ded3a01de4ab158d`.

### Next action

- Terminate only the stale pre-fix scheduler test process, allow the current full-race run to finish, run the documented managed conformance script with `PYTHONPATH` and startup DB directory pinned to the exact PR45 worktree, then audit and explicitly stage/commit only intended paths.

## 2026-08-05 managed PR45 checkpoint

### Current findings

- Managed run `20260805_063638` pinned `PYTHONPATH` and `--server-db-dir` to the clean exact PR45 worktree and collected exactly 3 selected tests from 11,590 cases.
- All three errored before test execution in PR45's session capability snapshot: Barn returned integer `1` for `server_version("options.OUTBOUND_NETWORK")`, while that older harness accepted only `ON`, `OFF`, or `#-1`.
- This is a harness/setup incompatibility, not a result for any of the three lifecycle fixtures. The managed server started and was reachable.
- The current conformance main harness accepts numeric option values, but the three PR45 YAML files are not present in its packaged suite. The dirty main conformance checkout must not be modified to transplant them.
- Exact PR45 exposes manifest gating, but using an invented target/oracle profile solely to bypass the failed probe would be an ad hoc workaround and has not been attempted.

### Verification state

- Full race remains running in the current scheduler package; the stale pre-fix scheduler process and its parent were stopped only after exact PID/command verification.
- Exact PR45 managed invocation and failure artifacts are preserved under `reports/runs/20260805_063638`.

### Blocker

- Exact-ref managed execution is blocked by the old PR45 capability probe's incompatibility with Barn's current numeric option response. Project rules require reporting the managed harness failure before any alternative path; no manual server or Toast process has been launched.

### Next action

- Determine whether the clean current conformance code can select the exact PR45 YAML files without copying or altering either checkout. If not, report the managed limitation, finish full-race verification, audit the diff, and commit only the intended local repair files.

## 2026-08-05 final pre-commit checkpoint

### Current findings

- Current conformance main cannot select YAML outside its packaged `_tests` directory and does not contain PR45's three fixtures. Mixing current harness code with copied PR45 fixtures would no longer be an exact-head run and would modify or synthesize harness state, so it was not attempted.
- Added a direct server panic integration regression using real MOO code that creates a WAIF plus anonymous object and suspends. Emergency checkpoint reload confirms zero pending roots, one suspended task, and one anonymous object.
- Refined task serialization roots to match values actually written for queued, suspended, and interrupted tasks, avoiding serialization-only retention of stale call-stack values that are not persisted.
- Added deterministic task snapshot ordering coverage; 20 race-enabled repeats pass.

### Verification state

- Full `go test ./...` and `go build ./...` pass after the direct server panic regression and task-root refinement.
- First full `go test -race ./...` passed, including scheduler in 222.825 seconds.
- A second full race run containing the final production changes is in progress; all packages through profile are green and scheduler is the remaining long-running package.
- Exact managed PR45 remains setup-blocked as documented in run `20260805_063638`; no fixture actually ran, and no manual server/oracle path was used.

### Current state and next action

- Intended local change set is 15 files: 14 modified paths plus new `db/format/task_owned_roots_test.go`. `reports/runs/` and both original-checkout notes remain excluded.
- Completed: second full race, final tests/build/diff audit, explicit staging of 15 intended paths, and one local correction commit without pushing.

## 2026-08-05 completion checkpoint

- Second full `go test -race ./... -count=1 -timeout=300s` passed; scheduler completed in 204.805 seconds.
- Final `go test ./...`, `go build ./...`, and `git diff --check` passed after deterministic-order coverage was added.
- Explicitly staged the 15 intended paths only. `reports/runs/` remained untracked; neither original-checkout notes file was staged.
- Created local commit `dc34264` (`Correct reentrant shutdown finalization ownership`) on `agent/pr96-pending-roots-repair`.
- No push, pull request mutation, merge, manual Barn launch, or manual Toast launch was performed.

## 2026-08-05 post-review repair checkpoint

### Current findings and implementation

- Review rejected `dc34264` because shutdown publication did not account for task producers, suspend state preceded scheduler-owned VM/root publication, running resumed VMs were snapshot-readable, frame-pop WAIF discovery recursively promoted nested WAIFs, and loaded pending roots had no startup executor.
- The scheduler now admits each task execution slice as a finalization producer. Shutdown publication waits for every admitted producer and any resulting sink pass before closing its ready boundary.
- Running task IDs are recorded under the scheduler mutex. `TaskSnapshots` skips them before calling `PersistenceSnapshot`, so a resumed VM cannot be copied while executing.
- `suspend()` now records intent on a running task. The scheduler publishes task-owned anonymous-GC state, the saved VM, and queued/suspended state while holding both finalization and scheduler ownership. Forked `suspend(0)` requests are consumed as internal yields without exposing a queued state.
- Actual `VM.Return` frame-pop collection now stops at an outer WAIF boundary, leaving nested WAIFs inside that single direct root.
- Deterministic regressions cover a late producer blocked in the finalization sink and actual `VM.Return`; both pass. New suspension/checkpoint interleaving tests are written and compiling after correcting a test-only snapshot field name.

### Verification state and blocker

- Focused `task` and `vm` packages pass. Scheduler focused shutdown/frame tests and the corrected forked zero-delay test pass repeatedly.
- The first scheduler package run exposed the forked zero-delay regression described above; the implementation has been corrected and its focused regression passes 20 times.
- No current code blocker is known. The exact PR45 managed run remains setup-blocked by its old capability parser rejecting Barn's numeric option response; no alternate/manual workflow is authorized.

### Next action

- Finish the deterministic suspension/checkpoint regressions, implement startup execution of loaded pending finalization roots through registered scheduler tasks, add the clean shutdown-in-recycle startup integration test, then run focused/race/full test and build gates before an explicit-path correction commit. Do not push.

## 2026-08-05 startup executor implementation checkpoint

### Current findings and implementation

- Deterministic suspension/checkpoint interleaving tests now pass repeatedly. Before publication, `TaskSnapshots` returns no task; task-owned anonymous GC, saved VM, and suspended state are then published under both scheduler/finalization locks; afterward exactly one complete suspended snapshot owns the anonymous root.
- A resumed task blocked inside a builtin remains in `runningTasks`; 100 concurrent-style checkpoint samples return no queued/suspended copy, and the next suspension produces exactly one complete VM snapshot.
- The store now atomically transfers its loaded pending-finalization queue to a startup executor.
- The startup executor holds an outer finalization-producer admission across the whole ordered root batch and runs each root as a registered scheduler task. WAIF roots execute their real `:recycle`; anonymous roots run `recycle(this)` with the actual anonymous receiver and owner permissions.
- Startup task frames carry the actual WAIF/anonymous `this` value. Their context marks the exact root being finalized, and completion filtering prevents that identity from being re-promoted by frame-pop discovery while preserving any other roots.
- `Server.Start` invokes the executor before starting input or binding listeners. If a recycle hook requests shutdown, startup enters the normal main-loop shutdown path without opening service.

### Verification state and blocker

- `go test ./scheduler ./server` passes after the startup executor implementation; prior focused suspension, zero-delay, shutdown, and frame-pop repeats remain green.
- No current code blocker is known. Direct startup-executor and end-to-end server checkpoint/log regressions still need to be added.
- The exact PR45 managed run remains setup-blocked by the old capability parser. No manual or synthesized alternative has been used.

### Next action

- Add store transfer coverage, ordered WAIF/anonymous startup executor coverage, and a server integration fixture whose loaded WAIF logs entry, calls `shutdown()`, logs return, then exits cleanly with a checkpoint containing zero pending roots. Run focused race repeats before full gates.

## 2026-08-05 startup regression and race checkpoint

### Findings and corrections

- The first end-to-end startup test proved that the hook ran, `shutdown()` returned, the post-shutdown log ran, and the server exited cleanly, but its final checkpoint still contained the active WAIF root.
- Root cause: `BeginShutdown` snapshots the caller VM immediately, before task-completion filtering. The loaded WAIF was therefore handed to shutdown from the active recycle frame even though completion later excluded it.
- `BeginShutdown` now excludes the exact `TaskContext.FinalizingValue` identity from caller-VM capture. This preserves any other roots and prevents only the already-executing persisted root from being requeued.
- The corrected end-to-end startup regression passes 20 repetitions with ordered `ORACLE WAIF RECYCLE ENTERED` / `ORACLE WAIF RECYCLE RETURNED` logs and zero pending roots in the final checkpoint.
- Scheduler coverage verifies ordered WAIF then anonymous execution, actual receiver identity, registration in both scheduler and global task manager during each hook, and anonymous destruction.

### Verification state and blocker

- Store transfer, scheduler startup execution, atomic suspension publication, resumed-VM snapshot exclusion, late-producer shutdown, real frame-pop, and server startup shutdown regressions pass under `-race` for 10 repetitions.
- `git diff --check` passes. Intended worktree changes currently span 17 tracked/untracked source and test paths; `reports/runs/` remains untracked and excluded.
- No code blocker is known. Exact PR45 managed execution remains the previously documented capability-parser setup blocker.

### Next action

- Complete the production diff audit, run all modified packages normally and under race, then run full `go test ./...`, full `go test -race ./...`, and `go build ./...`. Explicitly stage only intended paths and create a local correction commit; do not push.

## 2026-08-05 final correction verification checkpoint

### Final corrections after audit

- Shutdown publication now rechecks active producers after the sink returns. A deterministic regression admits a producer during the blocked sink and proves ready remains open until that producer publishes its root through a second sink pass.
- Task panic recovery now settles the owned VM through the same finalization boundary before its producer finishes. Actual returning and panicking task regressions create an anonymous root, block, race shutdown, then prove the root reaches the sink before ready closes.
- `suspend(0)` clears any stale timed wake deadline before the scheduler stamps the new yield moment.

### Verification state

- The focused late-producer and panic-root tests pass 50 normal repetitions and 20 race repetitions.
- The complete focused lifecycle/startup/snapshot/frame-pop set passes 10 race repetitions.
- Full `go test ./...` passes.
- Full `go build ./...` passes.
- Full `go test -race ./... -count=1 -timeout=300s` passes; scheduler completed in 207.809 seconds.
- `git diff --check` passes.
- Exact PR45 remains setup-blocked by its old capability parser; run `20260805_063638` is unchanged, and no manual or improvised conformance path was used.

### Next action

- Explicitly stage the 17 intended source/test paths, verify cached names/stat and that `reports/runs/` plus original-checkout notes remain excluded, then create one local correction commit. Do not push.

## 2026-08-05 second post-review checkpoint

### Rejection and current findings

- Commit `6aec871` is CODE NO-MERGE. Three architectural blockers remain: suspension callback linearization, shutdown admission closure at request time, and Toast-exact startup ordering/type grouping.
- Current `RequestSuspend` records only a boolean/seconds while leaving `TaskRunning`. `Resume`, `CompleteExec`, and `Kill` therefore cannot distinguish an in-flight suspension from an ordinary running task; early callbacks are rejected or can be overwritten when suspension publishes.
- Current producer admission rejects only after `shuttingDown` publication. A task can start after `shutdownRequested`, extend the boundary repeatedly, and execute outside the literal request-time admission cut.
- Current Barn startup executes pending roots before disconnection hooks and `#0:server_started`, and preserves cross-type file order.
- Current Toast source at `/root/src/toaststunt/src/server.cc` explicitly performs checkpointed `user_disconnected` notifications second, `#0:server_started` third, and later calls `recycle_anonymous_objects()` followed by `recycle_waifs()` at lines 851-852. Source inspection was read-only; no manual server/oracle was launched.

### State, blocker, and next action

- Worktree head remains `6aec871`; only `reports/runs/` is untracked. No new code edits or push occurred in this correction turn yet.
- Exact PR45 managed execution remains setup-blocked by its old capability parser; no workaround is authorized.
- Next: inspect exact Toast queue construction/order and Barn async callback sites, define a task-owned suspension transition model with buffered completion events and kill dominance, then write deterministic red tests for before/during/after publication and request-time admission closure.

## 2026-08-05 suspension model implementation checkpoint

### Current findings

- Toast current source commit is `aecc51e9449c6e7c95272f0f044b5ba38948459e`. `main_loop()` reads pending values into separate structures, runs checkpointed disconnections, runs `#0:server_started`, and only inside the main loop calls anonymous recycling before WAIF recycling.
- Toast anonymous queue insertion is at the list head, so anonymous roots execute in reverse encounter order. WAIF roots are tracked separately in `destroyed_waifs` (`std::unordered_map`); the architecture must not preserve Barn's cross-type file order.
- Async Barn callbacks include `Resume` from SQLite, HTTP, process stdin, and input delivery, plus `CompleteExec` from exec/background operations. The exec paths currently set task fields directly before requesting suspension.
- Kill after a published task-owned suspension also requires shutdown handling: killed tasks are omitted from task snapshots, while task-owned anonymous requests were previously skipped unconditionally during shutdown transfer.

### Implementation state

- Task suspension now has explicit idle/requested ownership, one buffered completion kind/value, atomic exec-suspension request support, terminal-state protection, and `BeginRun` kill dominance.
- `Resume` and `CompleteExec` buffer only while a matching requested transition exists; ordinary Running callbacks remain rejected. Duplicate early callbacks are rejected.
- Scheduler suspension publication is being converted to install VM, publish state, and establish task-owned roots while holding finalization, scheduler, and task ownership. It exposes before/during/after deterministic observer stages.
- This code is mid-edit and has not been formatted or compiled yet.

### Blocker and next action

- No external blocker beyond the unchanged exact-PR45 capability parser issue.
- Next: finish compilation changes in async builtins and scheduler, add the callback timing matrix, close request-time execution/scheduling admission, then replace Barn's startup executor ordering and tests with Toast-derived grouping/order coverage.
## 2026-08-05 correction implementation checkpoint

- Current repair worktree remains `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair`; no push and no correction commit yet.
- Suspension now establishes task-locked requested ownership before async callbacks. Resume and CompleteExec buffer exactly once before VM publication, publication installs the VM and publishes state atomically, ordinary Running callbacks are rejected, and Kill remains terminal. Added task and scheduler before/during/after immediate-callback matrices plus explicit suspend(0) queue semantics.
- Shutdown producer admission now rejects on `shutdownRequested`, not only final publication. `runTask`, immediate registration, `QueueTask`, and `ProcessReadyTasks` honor the closed boundary. A pre-admitted startup-finalization batch uses the internal admitted execution path so reentrant `shutdown()` from `:recycle` can return and settle. Added continuous rejected-activity/no-starvation proof and late execution/root rejection.
- Loaded pending roots transfer into separate startup queues. Startup binds listeners, reports checkpointed disconnections, calls `#0:server_started`, then drains anonymous roots LIFO followed by WAIF roots in encounter order. Added server log-order integration and scheduler per-type order test.
- Killed suspended task-owned anonymous roots are promoted to the shutdown checkpoint boundary from the saved VM; live suspended roots remain task-owned.
- Focused `go test ./task ./builtins ./scheduler`, `go test ./scheduler ./server`, and targeted new regression selections are green. `git diff --check` is green.
- Existing untracked `reports/runs/` remains untouched. This notes file must remain uncommitted.
- Next: audit the expanded diff for locking/lifecycle edge cases, run repeat and race suites, then full test/race/build gates; commit locally only if all pass.
## 2026-08-05 final gate checkpoint

- Final focused repeat: relevant task/scheduler/server regressions green at `-count=20`.
- Final focused race: relevant task/scheduler/server regressions green at `-race -count=10`.
- Full `go test ./...` green after the final startup/shutdown ownership tightening.
- Full `go test -race ./...` green after that tightening; scheduler completed in 202.597s.
- Full `go build ./...` green. `git diff --check` and `git diff --cached --check` green.
- Final audit closed a subtle boundary window: queued startup roots are now transferred by `BeginShutdown` before `shutdownReady` closes, while `RunStartupPendingFinalizations` acquires its batch producer admission atomically with taking those roots.
- Explicit intended paths are staged in the repair worktree. Pre-existing `reports/runs/` remains untracked and unstaged. This notes file remains outside the repair commit.
- PR45 exact setup remains blocked as previously reported; no manual Barn/Toast process was launched.
- Next: create one local correction commit, verify its exact head and preserved untracked state, and report to the parent. Do not push.
