# Pending finalization audit checkpoint

- Scope: read-only analyst audit of exact local head dc342649225a871277b36f9dd5cc63bb436f322d in C:/Users/Q/code/.worktrees/barn-pr96-pending-roots-repair.
- Current state: audit starting; no code findings yet.
- Required checks: shutdown-in-recycle reentrancy and ordering; concurrent flush and roots during sink; panic and suspended task ownership/serialization; nested WAIF direct-root identity; ordinary cancellation cleanup; ownership boundaries; canonical *task.Task and anonObjects; races, lost wakeups, snapshots, duplicates.
- Known admission blocker: managed PR45 setup currently fails because master has numeric OUTBOUND_NETWORK; do not improvise or manually launch.
- Next action: read gauntlet instructions, inspect exact diff/current code and live issue #95/PR #96/Toast PR #45, then run focused, repeated, race, and full local gates.

## Current findings

- Exact local HEAD verified as dc342649225a871277b36f9dd5cc63bb436f322d; PR #96 remote still points to 0450cf22c84d0c3ebdb96a9172b6072db1a7f56a, so this audit is intentionally local-head-only.
- Live issue #95 requires preserving the cyclic pending anonymous value. Live Toast PR #45 pins three contracts: shutdown returns inside pending WAIF recycle and clean exit writes zero pending roots; panic with one suspended task-owned WAIF/anon graph writes zero pending roots plus one suspended task; nested WAIF topology remains one direct pending root.
- Concrete race: publishShutdown closes shutdownReady after observing producer queues empty, but deferPendingWaifs, deferAnonGC, and settleCompletedTaskFinalizations append directly to the store sink after unlocking when shuttingDown is already true. A waiter can begin checkpoint immediately after shutdownReady closes and acquire the store lock before that late append. In a panic with concurrently finishing sibling tasks, roots can therefore miss the emergency checkpoint. Existing roots-during-sink coverage exercises arrival before publication, not after ready closes.
- Architecture observations so far: canonical *task.Task checks and anonObjects-only lookup are present; task snapshot RootValues covers every value serialized by queued and suspended task writers; snapshot assigns deterministic anon ids and WAIF rewrite identity is shared within one serialization plan.
- Protocol path C:/Users/Q/.Codex/AGENTS.md.d/gauntlet.md does not exist; the installed gauntlet skill was read and followed instead.
- Known admission blocker remains the managed PR45 setup failure on master numeric OUTBOUND_NETWORK; no manual server path will be used.
- Next action: prove/reproduce the late-producer publication race with existing tests or a read-only scheduling analysis, inspect all failure-mode tests, then run focused repeats, race packages, full go test/build/vet, and diff checks.

## Final analyst evidence

- BLOCKER: terminal task lifecycle is not linearized with shutdown publication. runTask marks a task Completed or Killed before settleCompletedTaskFinalizations publishes its roots. publishShutdown can close shutdownReady in that window; Panic immediately checkpoints; the late producer then appends outside the boundary and can miss the dump.
- BLOCKER: suspend lifecycle has the same gap. suspend() changes Task state before runTask enqueues a TaskOwned request, commits, and stores BytecodeVM under scheduler.mu. Concurrent Panic can clear the TaskOwned request and snapshot the task while its VM is nil, producing an invalid suspended task or losing roots.
- BLOCKER: TaskSnapshots does not exclude TaskRunning. A resumed task retains its BytecodeVM while runTask mutates it after releasing scheduler.mu; concurrent TaskSnapshots calls PersistenceVMSnapshot on the live VM and classifies it as suspended. This permits a data race, torn state, and duplicate running-task serialization. The GC snapshot path already rejects TaskRunning; checkpoint snapshotting does not.
- BLOCKER: actual popped-frame nested WAIF topology is flattened. collectPendingWaifsFromFrame recursively adds outer and nested WAIFs to VM.PendingWaifs. TakePendingFinalizationValues scans PendingWaifs before TakePendingWaifs and treats each entry as direct, so both become top-level pending roots. Store-only tests inject only the outer root and miss this integration path.
- BLOCKER against live Toast PR #45: Barn has no non-test consumer that executes loaded database PendingFinalizations at startup. LoadDatabase only installs them into Store. The current PendingWaifRecycleShutdown fixture puts the required ENTERED/RETURNED log calls and shutdown() solely in #9::recycle, so exact head cannot satisfy that case.
- Canonical ownership checks passed review: task_local access requires exact *task.Task; anonymous objects are created/loaded only in anonObjects; server is the sole runtime sink wiring point; Store is the sole pending candidate owner.
- Existing focused tests passed 30 repetitions; focused race tests passed 5 repetitions. Full go test ./... and go build ./... passed. Full race for scheduler, server, vm, db/store, and db/format passed, with scheduler taking 208 seconds. git diff --check passed.
- Full go vet is not green because base vm/stack.go ReadByte triggers the standard method-signature diagnostic. This is the separate known ReadByte issue, not introduced by dc34264.
- Managed Toast PR #45 cannot be admitted currently because its managed setup is blocked by the known numeric OUTBOUND_NETWORK master behavior; no manual server or oracle path was used.
- Verdict: exact-head CODE NO-MERGE. Required next action is a single scheduler lifecycle linearization that quiesces running tasks or atomically transfers their task-owned roots before ready closes, plus direct frame-pop nested-WAIF coverage and startup pending-finalization execution; then repeat this audit and the managed PR45 run once admission is repaired.

## Corrected-head re-audit

- Scope moved to exact local head 6aec87102acbdf0816bdd1a26cab27dd43cc0c49 in the same repair worktree; prior dc34264 findings are being treated as the regression checklist.
- Current observations: the repair adds finalization-producer admission around runTask, delays visible suspension until the resumable VM is installed under scheduler locks, tracks running task IDs for snapshots, switches frame-pop collection to direct WAIF roots, and adds an ordered startup pending-finalization executor invoked before listener/input service.
- Startup lifecycle review is still open. The executor drains Store pending roots up front, executes each as a registered task, and restores the failed and unattempted suffix on explicit error. It deliberately filters the currently finalizing root from shutdown rediscovery.
- Potential concern requiring proof: successful no-hook/invalid pending roots are consumed silently; startup hook error semantics and shutdown reentrancy must be compared against current code and the confirmed Toast PR45 contracts. No verdict yet.
- Managed Toast PR45 remains inadmissible because its managed setup is blocked by numeric OUTBOUND_NETWORK; no manual launch or improvised oracle path will be used.
- Next action: finish targeted source review of producer admission, snapshot locking, root topology, startup executor error/reentrancy semantics, then run repeated focused, race, and full gates and issue an exact-head CODE verdict.

## Corrected-head final findings

- BLOCKER: deferred suspension publication loses asynchronous completions and can resurrect killed tasks. RequestSuspend leaves a running task in TaskRunning until scheduler publication. Resume and CompleteExec reject any non-TaskSuspended state, so fast sqlite/exec/read-style completions in that interval are dropped. Kill can set TaskKilled in the same interval, after which PublishRequestedSuspension blindly applies the pending suspension and overwrites TaskKilled.
- BLOCKER: task admission is not closed at shutdown. beginFinalizationProducer rejects only after shuttingDown is published, not when shutdownRequested is set, so new work can continually extend the shutdown wait. Worse, runTask continues even when admission returns false, so work started after shutdownReady closes runs outside the finalization boundary and can produce roots after a panic/final checkpoint is allowed to start.
- BLOCKER / oracle mismatch: Toast startup queues loaded anonymous roots and WAIFs into separate finalization owners, runs checkpointed-disconnection hooks, then #0:server_started, and only in its loop runs recycle_anonymous_objects followed by recycle_waifs. Barn drains and executes roots synchronously in exact persisted cross-type order before input/listeners/server_started; its ordered startup test therefore locks in Barn-specific behavior rather than the Toast oracle.
- Prior dc34264 blockers are structurally repaired: admitted producer quiescence covers late completion/panic settlement, suspension installs VM/state under scheduler ownership, running VMs are excluded from task snapshots, actual frame-pop uses direct WAIF roots, and a startup executor exists.
- Verification: focused lifecycle/root/startup tests passed 20 repetitions; focused race tests passed 5 repetitions; fresh go test ./... -count=1 passed; go build ./... passed; broad race gate for scheduler/server/vm/db-store/db-format/builtins passed (scheduler 206s); git diff --check passed.
- go vet ./... remains blocked only by the known base diagnostic vm/stack.go:49 ReadByte() byte signature.
- Managed Toast PR45 remains blocked at managed setup by numeric OUTBOUND_NETWORK; no manual server/oracle launch was used. Read-only Toast source inspection was used for startup ordering.
- Exact-head verdict: CODE NO-MERGE at 6aec87102acbdf0816bdd1a26cab27dd43cc0c49.

## Exact-head 6aeadc32 re-audit

- Scope: fresh analyst review of 6aeadc322c5f3c99f478729edebe69ecb440ffee only; no historical Markdown and no product edits.
- The previous three blockers have direct repairs: transient suspend completion buffering plus task-locked publication; shutdown producer rejection from shutdownRequested onward with runTask fail-closed; Toast-style startup lifecycle placement and separate anonymous/WAIF queues.
- Suspension review so far: RequestSuspend/RequestExecSuspend establish one requested phase; early Resume/CompleteExec is buffered exactly once; Kill clears the phase; publication installs the VM, applies the suspension and buffered completion under one task lock, then publishes scheduler-owned roots before unlock. The scheduler separately settles roots when publication observes TaskKilled.
- Shutdown review so far: ordinary QueueTask/register/runTask reject after shutdown request. Startup and host lifecycle work use explicit already-admitted paths so recycle/shutdown_started/checkpoint hooks can complete inside the established boundary.
- Startup review so far: LoadDatabase transfers Store roots into separate startup queues; anonymous roots are prepended for Toast LIFO, WAIFs retain encounter order; Start runs checkpointed disconnection hooks and server_started before the two collectors, then starts input/acceptance only if shutdown was not requested.
- Open probes: exact callback/kill matrix under repeated race; task cleanup for rejected/finished lifecycle tasks; queued startup roots if shutdown occurs before Start; lifecycle task roots after shutdown publication; possible lock-order deadlocks; startup partial-failure restoration; snapshot and direct nested-root regressions.
- Next action: finish targeted source/test inspection, run focused repeated and race tests, then fresh full tests/build/race/vet and report exact-head CODE verdict.

## Exact-head 6aeadc32 final findings

- BLOCKER: real read-resume discovery cannot reach the new transient completion buffer. Manager.FindReadingTask and Manager.ResumeTask still require visible TaskSuspended, so input arriving after RequestSuspend but before publication sees TaskRunning and is not delivered to Task.Resume. Direct-pointer callback tests bypass this production lookup path.
- BLOCKER: Kill racing suspension publication can leave a killed task queued with its VM and roots retained. PublishRequestedSuspension returns the pre-unlock queued state; Kill can win immediately after unlock, but task_runtime uses the stale return to enqueue rather than settle and clear the VM. The new kill matrix checks only final TaskKilled state.
- BLOCKER: killed suspended tasks holding only WAIF roots have no shutdown handoff. Deferred-root traversal reaches a killed task VM only through a TaskOwned pending-anon marker; TaskSnapshots excludes killed tasks. The added handoff test creates an anonymous object, so it does not cover WAIF-only locals.
- BLOCKER: loaded startup pending roots are removed from Store into scheduler-only queues, but ordinary checkpoints before startup finalization snapshot neither queue. A dump_database call from user_disconnected or server_started can therefore persist zero pending roots even though Toast keeps those roots in its pending-finalization owners.
- Additional error-path hazard: checkpoint_started/checkpoint_finished always bypass ordinary producer admission even for non-shutdown checkpoints. If such a hook initiates panic, shutdownReady may publish while the lifecycle task still runs and the emergency checkpoint can recursively invoke the same hook; this path has no reentrancy or concurrency guard.
- The prior direct suspension callback, shutdown-request admission, and Toast startup-order blockers are repaired. Direct nested roots, running-task snapshot exclusion, queued-before-ready startup roots, and the currently covered killed-anon handoff remain green.
- Verification: focused tests passed 50 repetitions; focused race tests passed 10 repetitions; fresh go test ./... and go build ./... passed; broad uncached race gate for task/scheduler/server/vm/db-store/db-format/builtins passed; git diff --check passed. go vet ./... reports only the known base vm/stack.go ReadByte signature diagnostic.
- Managed Toast PR45 remains blocked at managed setup by numeric OUTBOUND_NETWORK. No manual Barn or Toast launch was used; Toast ordering evidence came from read-only source inspection.
- Exact-head verdict: CODE NO-MERGE at 6aeadc322c5f3c99f478729edebe69ecb440ffee.
- Next action: report the exact code locations and required regression coverage to the parent; do not modify the product tree.
