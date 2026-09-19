# Fable review: shared execution and commit admission, pre-implementation

Reviewer: Claude Fable (independent provider). Author under review: OpenAI Codex.
Date: 2026-09-18. Repository `C:/Users/Q/code/barn`, HEAD verified `83751cc`
on `fix/mongoose-workload`. Read-only source review. This file is the only
write; nothing was staged, committed, built, or launched. One read-only `grep`
of the WSL Toast source (`/root/src/toaststunt/src/server.cc`) was used for
finding B4.

**Verdict: REVISE.** The policy (weighted service, FIFO shared/exclusive gate,
lease-before-gate) is sound. The proposed *placement* — "top-level
`runTaskSlice`, `Eval`, standalone `CallVerb` acquire before the physical VM
lease" — is not implementable as written: three of those "top-level" entries
are routinely reached *from inside* an admitted slice with no parent context in
their signature, and one entry drives the scheduler while holding its own
slot. Each is a hold-and-wait on the global cap. Six corrections are required
before coding; all are small. They are listed at the end.

Files read in full: the design plan, `engine/runtime.go`, `task_runtime.go`,
`eval.go`, `call_verb.go`, `task_factory.go`, `server/input_processor.go`,
`server/input_login.go`, `engine/internal/scheduler/scheduler.go`. Read in
part: `db/store/store_txn.go` (2360–2780), `store_snapshot.go` (1–180),
`store_core.go` gate sites, `engine/waif_lifecycle.go` (160–340, 436–489),
`server/server.go` (180–225, 285–530), `task/task.go` state/kill/wait methods,
`task/manager.go:96`, `builtins/system.go:774–803`, `builtins/store_reads.go`.
Not audited: every builtin for native waits (a grep for blocking waits found
only SQLite handle serialization and `time.Sleep`; see N5).

---

## 1. Complete VM-entry inventory

"Admit" = owns a reservation. "Borrow" = runs inside a caller's reservation.
"Exempt" = must never wait for admission; accounted post hoc to a system ledger.

| # | Entry (file:line) | Reached from | Today's ownership | Required class | Principal |
|---|---|---|---|---|---|
| E1 | `runTaskSlice` via scheduler worker (`runtime.go:168`, `:524`) | `ProcessReadyBatch` (`input_processor.go:250`), `ProcessReadyTasks` | claim at `runtime.go:512`, lease at `task_runtime.go:76` | Admit — **grant before claim** (B3) | activation programmer |
| E2 | `ExecuteVerbTaskSyncWithStart` → `runTask` (`task_runtime.go:967`) | lane, `input_processor.go:694` | fresh task registered Queued at `:960` | Admit — **before register** (B3) | player |
| E3 | `RunServerVerbTaskWithArgstr` → `runTask` (`task_factory.go:120`) | lane: `do_command` (`input_login.go:167`), user hooks (`:192`); main loop: `server.go:499/508/517/526`; **nested: `task_runtime.go:646`**; **nested via `OnComplete`→`loginPlayer`→`callUserHook`**; **nested via `dump_database`→`checkpoint()`** | none passed; signature has no parent | Admit when independent, **Borrow when nested** (B1) | player / system |
| E4 | `CreateLoginHookTask` → `runTask` (`task_factory.go:191`) | lane, `input_processor.go:604` | fresh task | Admit | shared anonymous |
| E5 | `ResumeReadingTask` → `runTask` (`task_factory.go:408`) | lane, `input_processor.go:399` | `ResumeAndClaim` → Running | Admit; claim currently precedes (B3) | player or anonymous |
| E6 | `Eval` (`eval.go:64`) | lane `;` fallback (`input_processor.go:735`), dbtool (`dbtool.go:241/246`) | own task + lease `eval.go:116` | Admit, **release across its suspend loop** (B2) | player |
| E7 | `callVerbWithArgstr(vmOwnershipNone)` (`call_verb.go:183`, `:232`) | lane: OOB `:387`, phantom login `:444`, `do_login_command` fallback `input_login.go:66`, `do_blank_command` `:135`; **nested: `callTaskTimeoutHook` `task_runtime.go:869`**; `host.VerbCaller` fallthrough `runtime.go:121` | task ID **0** lease, `call_verb.go:225–236` | Admit when independent, **Borrow at `:869`** (B1) | player / anonymous / system |
| E8 | `callVerbWithArgstr(vmOwnershipExecution)` (`runtime.go:119`) | builtin callback from a claimed DirectTxn context (Eval) | inherited lease, `call_verb.go:237–240` | Borrow — needs parent **context**, only `ownerTaskID` is passed today (B5) | caller's |
| E9 | `callVerbWithArgstr(vmOwnershipSweep)` (`runtime.go:113`) | `:recycle` hooks under `flushDeferredGC`/`run_gc`/Eval inline sweep | `SweepMu`+`VMStartMu` held | **Exempt**, not Borrow (B6) | system ledger |
| E10 | `CallVerbInContext` (`call_verb.go:25`) | every txn-backed builtin callback: `:initialize`, `:recycle`, `exitfunc`, `accept`/`enterfunc`, `#0:bf_*` (`builtins/objects.go:273/430/484/495`, `objects_hierarchy.go:728`, `objects_movement.go:100/121/131`, `registry.go:266`) | shares parent ctx/txn | Borrow (automatic if the reservation rides on the ctx) | caller's |
| E11 | `callWaifRecycle` own VM (`waif_lifecycle.go:439–489`) | `flushDeferredGC:294`, `finalizePendingWaifs` | sweep-owned, **no StoreTxn, direct writes** | **Exempt** | system ledger |
| E12 | `vm.RecycleOrphanAnonymousBatch` / `AutoRecycleOrphanAnonymousSince` → `session.CallVerb` → E9/E10 | `waif_lifecycle.go:329`, `runtime.go:164`, `eval.go:277` | sweep-owned | Exempt / Borrow per branch | — |
| E13 | VM-internal continuations (move/recycle lifecycle, `eval()` builtin frames) | same VM | none needed | n/a (same slice) | — |
| — | `drainForks` resumes (`task_runtime.go:907`, `call_verb.go:161/344`, `eval.go:176`) | same VM | same invocation | n/a | — |

Non-VM top-level mutation outside both mechanisms: `.program` commit,
`input_processor.go:775` (`DirectTxn().SetVerbCode`). Not a VM entry; it is an
unfenced direct write and stays outside the gate's isolation envelope (N3).

`host.TaskYielder` is wired (`server.go:162`) but no builtin consumes it (only
`host.go:78` validation). If anything ever calls it from a builtin it becomes a
B2-class entry. Delete it or classify it now.

---

## 2. Blocking issues

### B1. Three "top-level" entries are nested inside an admitted slice and cannot tell

Evidence, all executed on the parent's goroutine *before* the parent's deferred
release at `task_runtime.go:78–92`:

- `task_runtime.go:571/573` → `callTaskTimeoutHook` → `s.CallVerb` (`:869`) → E7.
- `task_runtime.go:646` → `s.RunServerVerbTask(0, "handle_uncaught_error", …)` → E3 → `runTaskSlice`.
- `task_runtime.go:710–712` `cb(result)` → `input_processor.go:590` `onComplete`
  → `loginPlayer` → `callUserHook` (`input_login.go:190–192`) → E3. `loginPlayer`
  is *also* called top-level from a lane (`input_processor.go:574`), and
  `callUserHook` is top-level at `:368` and `:502`. Same function, both roles.
- `builtins/system.go:797` `dump()` → `server.checkpoint()` →
  `RunServerVerbTask(checkpoint_started/finished)` (`server.go:508/517`) from
  inside a running VM builtin; and `task_runtime.go:251` `runDeferredCheckpoint`
  does the same mid-slice after `releaseEscalation`.

None of these signatures carries a parent context, task or reservation, and Go
has no goroutine-local state, so "am I nested?" is not inferable. An
implementation that follows the proposal literally admits again.

**Deadlock timeline (global cap = N, any N ≥ 1).**

1. N slices are admitted (slots N/N). Mongoose raises uncaught errors
   routinely (`task_runtime.go:591–594` notes each is a `$wiz_utils` write).
2. Each slice reaches `:646` while holding its slot and its lease.
3. Each calls `RunServerVerbTask` → `Admit` → queued; no slot is free.
4. No holder can finish until its child is admitted. No child is admitted
   until a holder finishes. Permanent. With cap 1 a single uncaught error
   suffices. The same shape holds for a fork timeout (`:571`), every
   successful login (`:710`), and every non-deferred `dump_database()`.

**Correction.** Make nesting explicit, single-level, and unforgeable:

- Internal variants take the owner's reservation:
  `runTask(t, res)`, `runServerVerbTask(parent *reservation, …)`,
  `callVerbWithArgstr(…, parent *kernel.TaskContext)`. Exported wrappers pass
  nil = independent = Admit. `:646` and `:869` pass the slice's reservation.
  A borrowed `runTaskSlice` never calls `Finish`; only the owner settles.
- `OnComplete`: do not thread a token through `task.SetOnComplete`. Return the
  callback from `runTaskSlice` and fire it in `runTask` *after* the slice has
  settled and released. `loginPlayer`'s hooks then are genuinely independent
  entries on the same goroutine, lane ordering is unchanged (still synchronous
  before the next input line), and the login task's lease no longer pins GC
  while `user_connected` runs. Its only argument is the result value; no
  anon/waif root is lost.
- Checkpoint: see B4.

### B2. `Eval` drives the scheduler while holding its own slot

`eval.go:116` takes the lease; `eval.go:201/214/224` call
`s.ProcessReadyTasks()` inside the suspend loop, which runs E1 slices through
`scheduler.Run` and **joins** them (`scheduler.go` `Run`). With admission at
`Eval` entry:

1. cap = 1. `;suspend(0)` (or `;fork … suspend`) on the `;` fallback or dbtool.
2. Eval holds slot 1/1, enters the `seconds == 0` loop, calls
   `ProcessReadyTasks`.
3. The worker's `runTaskSlice` calls `Admit`; it waits for Eval's slot.
4. `Run` never returns, so Eval never resumes, so the slot is never released.

cap = N deadlocks with N concurrent suspended evals. Production Mongoose
routes `;` through a verb, so this mainly kills Test.db conformance and the
dbtool — which is exactly where a cap-1 deterministic test would be run.

**Correction.** `Eval` releases its reservation when it enters the suspend
loop (`eval.go:175`) and re-admits before `bcVM.Resume()` (`:243`). This is the
design's own rule ("true MOO suspension releases it", plan line 258). Settle
service per admitted segment. The physical lease stays held as today.

### B3. Admission must come before claim and before registration

The proposal puts `Admit` at the top of `runTaskSlice`. By then:

- **Background (E1):** `runReadyTasks` has already claimed the task
  (`runtime.go:512`, `TryClaimQueued` → `TaskRunning`). Consequences of an
  unbounded wait in that state:
  - every GC collector fails closed on `TaskRunning`
    (`runtime.go:578`, `:611`, `:651`) → deferred GC is pinned for the whole
    admission wait, which the proposal wanted to avoid by admitting before the
    lease;
  - the task is no longer in `queued_tasks()` yet has not started; the design's
    "unselected ready tasks remain visible and killable" is violated;
  - `Task.Kill` (`task.go:737–750`) never calls `CancelFunc`, and `CancelFunc`
    is only installed at `task_runtime.go:284`, after admission. A kill during
    the wait sets `TaskKilled`; then `acquireTaskExecution` unconditionally
    executes `t.SetState(task.TaskRunning)` (`runtime.go:208`) and the killed
    task **runs anyway**. That window exists today (the `VMStartMu` wait) but
    is microseconds; admission makes it unbounded.
- **Fresh foreground (E2/E3/E4):** the task is registered `TaskQueued`
  (`task_runtime.go:960–961`, `task_factory.go:114–115`, `:184–185`) before
  `runTask`. `GetQueuedTasks` (`task/manager.go:111`) lists every `TaskQueued`
  task, so a command waiting for admission becomes visible in `queued_tasks()`
  and killable-then-resurrected as above. I did not verify Toast's
  `bf_queued_tasks`, but pending *input* tasks are not forked/suspended tasks;
  treat new visibility as a conformance risk and avoid it.
- **Shutdown:** a cancelled, never-started foreground task left `TaskQueued`
  is persisted by `TaskSnapshots` (`task_factory.go:476–477`) into the final
  checkpoint and would execute at next boot with no connection.

**Correction.**

- Foreground: `Admit` in the entry function *before* `newTaskID()`/
  `RegisterTask`; pass the reservation down. On cancel, nothing was registered.
- `ResumeReadingTask`: `Admit` before `ResumeAndClaim` (`task_factory.go:405`).
  The task stays Suspended-on-read and findable; the lane is serial so no
  second line can race it.
- Background: the worker calls `Admit` with the task **unclaimed**, then
  `TryClaimQueued`; if the claim fails (killed, or taken by Eval's nested
  pass), `Reservation.Cancel()` and drop it. Move the claim out of
  `runReadyTasks`. The saved VM of an unclaimed Queued task is immutable and
  walkable, so GC is not pinned.
- Independently: make `acquireTaskExecution` refuse to overwrite `TaskKilled`
  (return a bool; caller cancels and returns), closing the existing window too.

### B4. Checkpoint from `dump_database()` is a nested admission *and* a nested gate request

`builtins/system.go:786–800`: only a gate-exempt attempt defers; every other
caller runs `server.checkpoint()` inline inside the VM builtin — two nested
`RunServerVerbTask` admissions (B1) plus an exclusive gate request from a
goroutine whose own slice will later need a shared commit. The deferred variant
(`task_runtime.go:244–252`) runs at `:495`, i.e. **before** the suspend
hand-off publishes the VM at `:551`; a checkpoint taken there snapshots this
task as Suspended with no saved VM (`TaskSnapshots`, `task_factory.go:468–479`).
I did not execute this; it follows from line order and should be confirmed.

Toast does neither. `server.cc:2617–2625`:

```c
bf_dump_database(...) { ... checkpoint_requested = CHKPT_FUNC; return no_var_pack(); }
```

The main loop performs it later (`server.cc:809–832`).

**Correction.** `dump_database()` always calls the existing non-blocking
`requestCheckpoint()` (`server.go:335–347`), letting `mainLoop` (`:307`) run it
as an independent system-principal entry. This deletes `DeferredCheckpoint`,
`runDeferredCheckpoint`, the `IsCommitGateExempt` special case, the nested
admissions, and the mid-handoff snapshot. The `"CHECKPOINTING"` log line stays
at builtin time, so `assert_log` is untouched. It is a Barn behaviour change
toward Toast: prove `server/dump_database.yaml` on the managed oracle first,
then Barn, per the convergence loop. If that is refused, the fallback is to set
`DeferredCheckpoint` unconditionally and run it in `runTask` after settlement.

### B5. Task ID 0 cannot key a reservation or a borrow

`callVerbWithArgstr` builds `&task.Task{}` with ID 0 (`call_verb.go:225–231`);
`acquireTaskExecution` then does `ExecutingTasks[0]++` and
`leasedTasks[0] = t` (last writer wins, `runtime.go:202–206`). All concurrent
standalone hooks share that key. `executionContextClaim` therefore returns
owner 0 for every hook context, and the inherited path receives only
`ownerTaskID` (`call_verb.go:186`, `runtime.go:119`). A reservation map keyed
by task ID merges every concurrent hook into one owner; the first `Finish`
frees a slot that others still occupy.

**Correction.** The reservation is a pointer carried on `kernel.TaskContext`
(next to `StoreTxn`), never looked up by ID. `CallVerbInContext` borrows for
free (same ctx). Change the inherited path to pass the parent `*TaskContext`
and copy the pointer into the new ctx at `call_verb.go:284–303`.
Set it in the per-attempt block (`task_runtime.go:161–177`), **not** once before
`captureTaskRetryState` (`:109`): `retryState.restore` installs a clone of the
context captured at slice start (`:823–827`), so a pointer set only on the
first ctx is absent on attempts ≥ 1 and nested borrows would then re-admit.

### B6. Sweep-owned execution must be *exempt*; "borrow" has no lender

The proposal says sweep-owned calls borrow. Frequently there is nothing to
borrow from: `flushDeferredGC` runs from the dispatcher tail
(`runtime.go:483`, `:492`) with no reservation at all, and from slice/hook/eval
epilogues (`task_runtime.go:91`, `call_verb.go:217`, `eval.go:87`) after the
owner has finished. The contexts it uses are stale ones stored in
`lifecycle.PendingWaifs/PendingAnonGC` (`waif_lifecycle.go:294`, `:313–329`);
a reservation pointer on them is already released.

If a sweep hook ever waits for admission:

1. cap = N. `flushDeferredGC` sees `ExecutingTasks` empty (`runtime.go:566`),
   holds `SweepMu` + `VMStartMu` (`waif_lifecycle.go:232–235`).
2. N tasks are admitted and block in `acquireTaskExecution` on `VMStartMu`
   (`runtime.go:198`), each holding a slot. They were invisible to the
   quiescence check because admission precedes the lease.
3. The sweep's `:recycle` hook asks for a slot. None. Deadlock.

**Correction.** Classification is by ownership, checked first:
`isSweepOwnedContext` / `DeferredGC` ⇒ never call `Admit`, never assert a live
parent reservation, record occupancy to a system ledger after the fact.
E9 also performs an ordinary shared `Commit` (`call_verb.go:293`, `:348`) while
holding `VMStartMu`; under the FIFO gate that wait stalls every VM start for
the length of a checkpoint walk. That exists today with `RWMutex`; record the
wait (section 4) rather than hide it.

### B7. Shutdown ordering vs "close admission" and the finalization-producer count

- `beginFinalizationProducer` is the first thing each entry does
  (`task_runtime.go:70`, `call_verb.go:187`, `eval.go:65`). The shutdown
  `ready` channel closes only when `ActiveFinalizationProducers == 0`
  (`runtime.go:405–408`), and `shutdown()` from a task waits on it
  (`server.go:194–206`). If `Admit` is placed after that line, every admission
  waiter is an active producer: `ready` cannot close until all of them are
  admitted and finish. If admission has been closed, it never closes.
  **`Admit` must precede `beginFinalizationProducer`.** (B3's placement in the
  entry functions already guarantees this.)
- `server.shutdown()` runs `shutdown_started` (`server.go:421`), then
  `s.input.Stop()` (`:428`, joins lanes via `p.wg.Wait()`), then the final
  checkpoint with its two hooks (`:432`), then `s.runtime.Stop()` (`:439`).
  "Shutdown closes new admission" (plan line 373) would refuse Barn's own
  shutdown hooks if applied at `BeginShutdown`. And a lane blocked in `Admit`
  is not watching `p.ctx`, so `input.Stop()` hangs on it.

**Correction.** Two-stage close. `Runtime.CloseAdmission(classInput)` is called
by `InputProcessor.Stop()` before `wg.Wait()`: it cancels queued *input*
requests (nothing registered, per B3) and rejects new ones. System-principal
and background requests stay open until `Runtime.Stop()`, where `Admit` observes
`s.ctx` so `scheduler.Stop()`'s `wg.Wait()` can join workers blocked in it.
Never close at `BeginShutdown`.

---

## 3. FIFO gate replacement: what the source requires

Production gate sites are exactly: `store_core.go:126`, `:280–285`;
`store_txn.go:2649–2652`; `store_snapshot.go:71–72`; engine users at
`task_runtime.go:122/134/203/249/422`; exemption at `store_txn.go:2373–2392`,
carried by `CommitAndRenew` at `:2617–2622`; read at `builtins/system.go:787`.
Tests touch the raw mutex directly (`store_snapshot_atomic_test.go:17–31`,
`store_txn_test.go:1990–2014`) and must move to the new API.

1. **No nested non-exempt commit under an exclusive owner — verified.**
   Under `runTaskSlice`, `ctx.StoreTxn` is never direct, so `host.VerbCaller`
   always takes `CallVerbInContext` (`runtime.go:105–107`) and shares the
   exempt txn. The post-slice hooks (`:571`, `:646`), the suspend commit
   (`:527`) and the completion commit (`:692`) all follow `releaseEscalation`
   (`:495`). Defer order is LIFO: the gate backstop (`:120–124`) runs before the
   recover (`:95`) and before lease release + `flushDeferredGC` (`:78–92`), so a
   panic releases the gate before any sweep hook commits. Preserve this order
   when `escalated bool` becomes a grant; keep the grant a slice-level variable
   because the promotion closure is rebuilt on each `goto retryAttempt`.
2. **The `:134` wait is unmeasured.** Only the promotion measures
   (`:202–205`). The entry acquisition happens after `Admit` under the proposal,
   so it sits inside the service window and must be timed (section 4).
3. **Cancellation scope.** `Commit()` has no context and returns an
   `ErrorCode`. Do not make shared acquisition cancellable in this increment:
   a cancelled commit needs a new MOO-invisible outcome and a discard path.
   Make only the entry exclusive request (`:134`) cancellable — nothing has run,
   so cancel = `Reservation.Cancel()` + return. Keep promotion (`:203`) and
   checkpoint non-cancellable; they are bounded by holders finishing, as today.
   "Release must not unlock a live owner" is then satisfied by construction
   for everything except the one request that owns nothing yet.
4. **Replay ownership: plan and code disagree.** Plan lines 317–323 say retain
   the exclusive capability across a validation-loss replay.
   `task_runtime.go:409–423` deliberately releases it, with a comment claiming
   that holding it "would deadlock any ordinary commit the task body issues".
   I could not find such a commit on a reachable path (the retried txn is
   re-exempted at `:168–174`), so the comment may be stale — but this increment
   should not decide it. Keep current behaviour, and note that under FIFO the
   released owner rejoins at the tail; attempt 63 remains the backstop.
5. **`x/sync/semaphore`** is not in `go.mod`. Its FIFO and cancel/grant race
   handling are right, and `TryAcquire` respects waiters. The wrapper must set
   exclusive weight == size exactly; shared participants beyond size merely
   queue. It gives no owner identity, reason, or wait time, all of which are
   required. A ~100-line in-package gate mirroring `scripts/model-scheduling-gate.py`
   is no larger than the wrapper and avoids a dependency. Either is acceptable.
6. **`Server.Panic`** (`server.go:457–483`) runs `checkpoint_started` and the
   exclusive snapshot. From `shutdown(…, panic)` inside a task it is already
   moved to a goroutine after `ready` (`server.go:195–206`), so it is
   independent. Give it the system principal and do not subject a panic dump
   to a closed admission stage.

---

## 4. Where to account gate waits without touching MOO quota clocks

Two clocks, two sinks, one source of truth:

- **Source:** the gate's `Acquire` returns the measured wait. That is the only
  place an ordinary commit's wait is observable; `Commit()` hides it behind a
  deferred `RUnlock` today (`store_txn.go:2649–2652`).
- **Sink 1 — service ledger (per invocation, not per txn).** Accumulate into
  the *reservation* (`atomic` nanos), reached through the ctx pointer from B5.
  A txn-local counter under-subtracts: one invocation uses many txns —
  per-attempt `BeginReadOnly` (`task_runtime.go:167`), `CommitAndRenew`
  successors (`store_txn.go:2619`), the refresh at `:492`/`:543`, and *separate*
  txns in borrowed hooks (`call_verb.go:293`). Give `StoreTxn` a
  `gateWaitSink *atomic.Int64` that `CommitAndRenew` copies to `next` alongside
  `gateExempt`, and that borrowed entries set from the borrowed reservation.
- **Sink 2 — MOO seconds budget.** Call `t.ExcludeExecutionWait` only for waits
  that occur while the VM is running with the deadline armed: the promotion
  (`:203`, already done), `flushStagedBeforeCoarse`'s `CommitAndRenew`
  (`builtins/store_reads.go:83`), and `run_gc`'s renewals (`runtime.go:148`).
  The last two are **not excluded today** — an existing quota leak under gate
  contention that FIFO neither fixes nor worsens. Post-VM commits
  (`:435`, `:527`, `:692`) feed sink 1 only. `ExcludeExecutionWait` is a no-op
  when no deadline is set (`task.go:527`), which is correct for ID-0 hooks.
- **Service window.** Start the occupancy clock *after* `acquireTaskExecution`
  returns, or subtract the lease wait as well: between `Admit` and the lease a
  task can sit on `VMStartMu` for the length of someone else's sweep
  (`waif_lifecycle.go:232–235`), and "settle over the invocation" would charge
  that to the victim principal. Call `Finish` **before** the epilogue's
  `flushDeferredGC` (`task_runtime.go:91`) for the same reason: the sweep
  collects other principals' garbage.
- Sweep-hook gate waits (E9) go to the system ledger with the `VMStartMu`-held
  flag, so "VM starts stalled behind checkpoint" is diagnosable.

---

## 5. Scope claim that the retained batch rules make untrue

The proposal keeps "current retry-safe/solo background batch rules".
`ReadyBatch` (`scheduler.go`) returns one batch; production fork/resume work is
non-retryable (`taskIsConflictRetryable`, `task_runtime.go:742`), so the batch
is **one task: the global FIFO head**, and `processRuntimeTick` keeps one batch
in flight and joins it (`input_processor.go:246–252`). The weighted policy
therefore only ever sees *one* background candidate. "Many forks from one
programmer against another" (plan acceptance list) cannot be arbitrated by
min-normalized-service; the single global background FIFO decides. What this
increment delivers is foreground-principal fairness plus a 3:1 share between
all foreground and the one background head.

Either state that limit in the plan, or have the scheduler expose one ready
head per programmer and let admission pick the principal, keeping "at most one
non-retryable background slice in flight" as an *eligibility* predicate rather
than a batch shape. The second is a larger change; the first is honest and
sufficient for this step.

Two eligibility traps if heads are ever submitted as queued requests:

- **Held workers.** `readyLocked` parks tasks whose lease is still physically
  held (`ReadyDeadline` zero via `executionActive`, `task.go:290–292`). Such a
  task must not be submitted: it would be granted, fail `TryClaimQueued`
  (`task.go:326`), cancel, and be resubmitted — a grant/cancel spin until
  `releaseTaskExecution` fires.
- **Lost wakeup.** `releaseTaskExecution` notifies the selector
  (`runtime.go:222`); a reservation `Finish` does not exist yet. If any selector
  path uses a non-blocking `TryAdmit`, `Finish` must also call
  `NotifyScheduleChange`, or an empty-handed selector sleeps on `changed`
  (`input_processor.go:218`) with a free slot and ready work.

---

## 6. Recommended smallest sound implementation

In this order; each step is independently testable.

1. **Pre-work, no policy yet.** (a) `dump_database()` → `requestCheckpoint()`
   after the Toast-first proof (B4). (b) Fire `OnComplete` from `runTask` after
   the slice returns (B1). (c) `acquireTaskExecution` refuses `TaskKilled` (B3).
   (d) Delete or classify `TaskYielder`. These remove three of the four nested
   entries before admission exists.
2. **Gate.** `store.Gate` with `Acquire(ctx, mode, reason) (Grant, wait)`,
   owner-only idempotent `Release`, FIFO cohorts. Migrate the six production
   sites and the two tests. `BindExclusiveGrant` replaces
   `ExemptFromCommitGate`. Add `gateWaitSink`. Cancellable only at `:134`.
3. **Reservation plumbing with cap = ∞.** Pointer on `TaskContext`; explicit
   parent parameters at `:646`, `:869`, and the inherited `callVerbWithArgstr`;
   sweep/`DeferredGC` exempt; `Eval` release/re-admit around its loop; `Admit`
   before register/claim/producer. With an infinite cap this is observation
   only and must be conformance-neutral. Add an invariant counter: "`Admit`
   called while this goroutine's ctx already carries a live reservation" must
   stay zero across the full conformance suite and the 1p/16p workload.
4. **Turn the policy on.** Ledgers, EWMA reservation, 3:1, watermark clamp,
   finite caps, two-stage close. Only now can behaviour change.
5. No performance statement until the identical live 1/16-client matrix runs,
   as the prompt already says.

---

## 7. Focused deterministic tests

All use `newRuntimeWithWorkerCount` and an in-memory store; no sockets, no
sleeps as synchronization (block on channels from test builtins/hooks).

| Test | Setup | Must hold |
|---|---|---|
| `TestAdmissionUncaughtHandlerBorrows` | cap 1; verb raises; `#0:handle_uncaught_error` defined | completes; nested-admit counter 0; one settlement |
| `TestAdmissionTimeoutHookBorrows` | cap 1; forked task exhausts ticks; `handle_task_timeout` defined | completes |
| `TestAdmissionLoginCompletionHooks` | cap 1; `CreateLoginHookTask` returns a player; `user_connected` defined | hook runs after the login slice settled; two settlements; lane order kept |
| `TestAdmissionEvalSuspendDrivesScheduler` | cap 1; `Eval("fork (0) … endfork suspend(0);")` | returns; fork ran |
| `TestAdmissionSweepHookExempt` | cap 1; holder parked in a test builtin; dispatcher-tail flush with a `:recycle` that commits | sweep completes without a reservation; system ledger charged |
| `TestAdmissionSweepVsVMStartCap` | cap 2; sweep parked inside `:recycle`; two tasks admitted and blocked on `VMStartMu` | sweep finishes; both tasks then run |
| `TestAdmissionKillWhileWaitingBackground` | cap 1 held; queued fork; `kill_task` | fork never executes; stays listed until killed; reservation cancelled; GC flush not failed-closed meanwhile |
| `TestAdmissionKillWhileWaitingForeground` | cap 1 held; second lane command | not in `queued_tasks()`; cancel leaves no catalog entry |
| `TestAdmissionRetryKeepsReservation` | force one validation loss, then a builtin callback on attempt 1 | callback borrows; counter 0 |
| `TestAdmissionConcurrentIDZeroHooks` | two standalone `CallVerb`s in flight, one with a nested DirectTxn callback | two reservations; first `Finish` leaves one occupied |
| `TestAdmissionShutdownReadyNotBlockedByWaiters` | cap 1 held by a task calling `shutdown()`; one lane waiting | `ready` closes; waiter cancelled; nothing `TaskQueued` in `TaskSnapshots` |
| `TestAdmissionShutdownHooksStillAdmitted` | after `CloseAdmission(classInput)` | `shutdown_started`, `checkpoint_started/finished` run |
| `TestGateFIFOCohorts` | S S X S, explicit tickets | first two overlap; X after both; last S after X |
| `TestGateCancelQueuedExclusive` | X queued behind a holder, cancelled | later S requests are admitted with the holder's cohort rules; no leaked ticket |
| `TestGateReleaseOwnerOnlyIdempotent` | double `Release` | one unlock, invariant counter 1, no panic |
| `TestGatePanicReleasesBeforeSweepCommit` | panic under exclusive grant with pending waif whose `:recycle` commits | no self-deadlock |
| `TestGateWaitAccounting` | hold X; commit at `:435`, coarse builtin renew, nested hook commit | all three waits in the reservation ledger; only the mid-VM one extends the deadline |
| `TestServiceExcludesLeaseWait` | sweep parked holding `VMStartMu`; admitted task waits 50 ms (controlled clock) | charged service excludes the wait |
| `TestDumpDatabaseDeferredToMainLoop` | `dump_database()` from a gate-holding and a plain slice | returns 0 immediately; checkpoint runs after the slice; suspended caller is snapshotted *with* its VM |

Managed conformance (unchanged expectations, Toast first): `server/dump_database.yaml`,
`audit/background_zero_suspend.yaml`, `server/exec_recent_regressions.yaml`, and
the full suite at step 3 with cap = ∞ and again at step 4 with cap = 1 — a cap-1
full-suite run is the cheapest deadlock detector available.

---

## 8. Non-blocking observations

- **N1. `force_input` back-pressure cycle.** `ForceInput` does a blocking send
  on the 256-slot `inputQueue` from inside a slot-holding builtin
  (`input_processor.go:431`; also via pending effects, `builtins/network.go:1285`).
  The run loop blocks in `dispatch` when a lane's 64-slot channel is full
  (`:270–273`). Once lanes can wait for admission: slot holders → full
  `inputQueue` → run loop stuck on a full lane → that lane waiting for a slot.
  Needs ≥ 320 queued lines for one connection, so it is a flood scenario, but it
  is a true cycle and it also stops background dispatch. Make the forced send
  non-blocking into a per-connection overflow list, or bound it with an error.
- **N2. Principal snapshot.** `input.Player` is sampled at read time
  (`input_processor.go:126–129`); `processCommand` re-reads `conn.GetPlayer()`
  (`:631`). Resolve the principal in the lane at `Admit` time.
- **N3.** `.program` writes verb code through `DirectTxn` (`:775`) outside the
  gate. Pre-existing; list it in the plan's isolation-envelope limits.
- **N4. Two admissions per command line** (`do_command` then the verb,
  `input_processor.go:660` and `:690`). Correct, but the EWMA "slice kind"
  should distinguish them or the cheap hook drags the estimate down.
- **N5. Native waits.** SQLite handle serialization
  (`builtins/sqlite.go:91–96`) waits for another task's operation, but that
  operation runs on its own goroutine via `runSQLiteAsync` with the MOO task
  suspended and slotless, so it is not a same-pool dependency. I found no
  builtin that blocks a slot-holder on another *admitted* task besides `Eval`.
  This was a grep, not a full audit.
- **N6. Standalone `CallVerb` argument roots.** `call_verb.go:221–224` takes the
  lease early precisely so anon/waif args are rooted. If `Admit` waits before
  it, args passed through the `runtime.go:121` fallthrough are unrooted for an
  unbounded time. All current independent callers pass strings/objects, and the
  one caller with arbitrary values (`:646`) is nested under the parent's lease.
  Keep it that way: do not turn `:646` into an independent queued request
  without first making `VerbArgsValues` a GC root.

---

## REVISE — corrections required before implementation

1. **Explicit nesting (B1, B5).** Reservation pointer on `TaskContext`, set per
   attempt; parent parameters at `task_runtime.go:646`, `:869` and the inherited
   `callVerbWithArgstr`; borrowed slices never settle; `OnComplete` fires after
   settlement. No ID-keyed ownership anywhere.
2. **`Eval` releases its reservation across its suspend loop (B2).**
3. **Admit before register, claim and `beginFinalizationProducer` (B3, B7);**
   `acquireTaskExecution` must not resurrect `TaskKilled`.
4. **`dump_database()` requests a main-loop checkpoint like Toast (B4),** proven
   on the managed oracle first; remove the in-slice checkpoint paths.
5. **Sweep-owned and `DeferredGC` execution is exempt, not borrowed (B6).**
6. **Accounting and shutdown (sections 4, 7):** gate wait measured in the gate,
   accumulated per reservation through a sink that survives `CommitAndRenew` and
   nested txns; service clock starts after the lease and stops before
   `flushDeferredGC`; two-stage admission close that still admits shutdown and
   checkpoint hooks.

Also amend the plan to state that, with the retained solo-batch rule, this
increment does not provide fairness *between background principals* (section 5),
and to record that the replay-ownership rule in the plan contradicts
`task_runtime.go:409–423` and is deliberately left as is.

With those six corrections the design is implementable in the order given in
section 6, and a cap-1 full conformance run becomes a meaningful gate.
