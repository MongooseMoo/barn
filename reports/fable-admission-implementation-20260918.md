# Fable review: shared admission and FIFO commit gate — implementation

Reviewer: Claude Fable (external, independent). Date: 2026-09-18.
Tree: `C:/Users/Q/code/barn`, HEAD `83751cc` plus the uncommitted working tree.
Reviewed against `reports/fable-shared-admission-20260918.md` (B1–B7) and
`plans/barn-transaction-scheduling-design.md`. `AGENTS.md` read.

Read-only. This file is my only write. No implementation edits, no staging,
no commits, no network. **I ran no tests, builds, vet, or conformance** (the
parent is running them); every statement below is from reading source, and
none is a verification claim about runtime behaviour. The tree changed while I
read it: `engine/admission_test.go` appeared, `runTask` gained a
`context.Canceled` branch, and `CallVerbWithArgstr` gained its own input-keyed
admission. Findings are pinned to the last state I read; line numbers are from
that state and may drift.

## Verdict: ACCEPT, with two small fixes recommended before commit (F1, F2)

All seven blocking findings from the earlier review are corrected in the
source, and I found no remaining cap-one hold-and-wait. Nothing below is a
deadlock or a data-loss bug. F1 and F2 are narrow correctness/hygiene defects
with few-line fixes; the rest are limitations to record.

---

## 1. Prior B1–B7: verified in source

| # | Earlier finding | What the code does now | Status |
|---|---|---|---|
| B1 | nested "top-level" hooks re-admit | `handle_uncaught_error` → `runServerVerbTask(…, scope, true)` (`task_runtime.go:724`); `ownsScope := scope == nil` is false, so `runTaskAdmitted(t, scope, false)` borrows and never `Finish`es. Timeout hook → `callVerbWithArgstr(…, vmOwnershipExecution, t.ID, t.ContextValue().Admission)` (`:920`), inherited lease, borrowed scope. `OnComplete` moved out of the slice into `runTaskAdmitted` (`:78–82`), after the slice's lease-release defer has run `scope.Finish()` (`:136–138`), so `loginPlayer`→`callUserHook` is a genuinely independent entry | fixed |
| B2 | `Eval` drives the scheduler holding its slot | `scope.Finish()` on every `FlowSuspend` iteration (`eval.go` suspend branch), `scope.ResumeBackground(…)` + `scope.Start()` before `bcVM.Resume()` (`eval.go:257`) | fixed (see F3) |
| B3 | claim/registration before admission; kill resurrection | Foreground entries call `enterInput` before `newTaskID`/`RegisterTask` (`ExecuteVerbTaskSyncWithStart`, `runServerVerbTask`, `CreateLoginHookTask`); `ResumeReadingTask` admits before `ResumeAndClaim`. Background: `ReserveAdmission` sets `admissionPending` (task stays `TaskQueued`, VM walkable, excluded from reselection via `ReadyDeadline`), worker `Acquire`s, then `TryClaimQueued`. `StartExecution` refuses `TaskKilled`; `acquireTaskExecution` returns false before the lease defer is registered (`task_runtime.go:116–118`), so no refcount underflow. `Kill` and `SetCancelFunc` now cancel | fixed |
| B4 | in-slice checkpoint | `host.Checkpoint = requestCheckpoint` (`server.go:167`); `DeferredCheckpoint`, `runDeferredCheckpoint`, and the `IsCommitGateExempt` branch are deleted. Conformance `server/boolean_dump_persistence.yaml` already waits 1500 ms after `dump_database()` because the oracle is asynchronous, so the suite is written for this behaviour | fixed (see L4) |
| B5 | ID-0 cannot key ownership; retry clone loses pointer | `kernel.TaskContext.Admission *admission.Scope`, assigned in the per-attempt block (`ctx.Admission = scope`, after `ctx.TaskID`), so retry clones carry it. `host.VerbCaller` passes `tc.Admission` on the inherited path. No ID-keyed lookup anywhere | fixed |
| B6 | sweep hooks must be exempt | `vmOwnershipSweep` passes `scope == nil`; `ownsScope` is false; no `Enter`, no root pin; `SetCommitWaitObserver` is guarded by `scope != nil`. `callWaifRecycle` is untouched and unadmitted | fixed |
| B7 | producer count and shutdown close | Every entry admits before `beginFinalizationProducer`. `InputProcessor.Stop` calls `CloseInputAdmission()` before `cancel`/`wg.Wait`; lifecycle hooks use `s.ctx` via the System key, so `shutdown_started` and the final checkpoint hooks are still admitted. Cancelled input requests were never registered, so nothing `TaskQueued` leaks into `TaskSnapshots` | fixed |

Other items from that report:

- **N1 (force_input back-pressure cycle): fixed.** Lanes are now an unbounded
  slice plus a 1-slot `ready` signal; `dispatch` never blocks
  (`input_processor.go:297–318`), so the run loop always drains `inputQueue`.
  Phantom forced logins are routed through a lane (`:380`, `:499`) instead of
  running inline inside the caller's builtin, removing another nested entry.
- **N6 (argument roots): fixed** by `pinAdmissionRoots`
  (`engine/admission.go:48–63`), pinned under `VMStartMu` + `s.mu` and merged
  into all three collectors (`runtime.go` `collectAdmissionRoots` calls).
- **Gate wait sink across renewals: done.** `SetCommitWaitObserver` is set on
  every `BeginReadOnly` in the slice (three sites) and copied by
  `CommitAndRenew` together with the exclusive grant
  (`store_txn.go` `CommitAndRenew`). The entry exclusive wait is now timed
  (`task_runtime.go:196–199`). Service starts after the lease (`scope.Start()`)
  and is settled before `flushDeferredGC`.

### Gate (`internal/commitgate/gate.go`)

Strict head-of-queue dispatch gives FIFO cohorts: a shared request joins only
if no exclusive precedes it; an exclusive head waits for `readers == 0`.
Cancellation racing a grant is handled on both `select` arms (`r.active` check
under `mu`; post-grant `ctx.Err()` releases). `Release` is idempotent and
owner-held. `BindExclusiveGrant` panics on a dead or foreign grant, and an
exempt `Commit` re-checks `Owns` — a stale exemption now fails loudly instead
of silently skipping the gate. Shared acquisition in `Commit` and the
checkpoint/promotion acquisitions use `context.Background()`; only the entry
escalation is cancellable (`taskCtx`). That matches the recommendation.
Defer order in `runTaskSliceAdmitted` still releases the gate before the
recover and before lease release. `EscalationLock/Unlock` have no production
callers left (grep: definitions only); `legacyGrant` is test-compat.

### Cap-one audit (source trace, not executed)

Traced with `Limit = 1`: uncaught-error hook (borrows), timeout hook (borrows),
login completion hooks (run after `Finish`), `Eval` + fork/suspend (scope
released across the loop; its own task is held out of selection by
`executionActive`), `dump_database` (request only), sweep `:recycle` hooks
(exempt, including from the dispatcher-tail flush), admitted tasks parked on
`VMStartMu` behind a sweep (sweep never needs a slot), exclusive-gate waiter
holding the slot while a main-loop checkpoint is queued behind it (checkpoint
hooks run *before* the gate request, so no gate holder waits for admission).
I found no cycle.

---

## 2. Findings

### F1 — Medium. Killing a task now surfaces `context canceled` as an error

`Task.Kill` now calls `CancelFunc` (`task/task.go`, `Kill`). Before this change
it never did, so the `<-taskCtx.Done()` branch (`task_runtime.go:570–574`) fired
only at runtime shutdown. Now it fires for any task killed **while running**:
the slice finishes its VM, commits, then returns `taskCtx.Err()`.

- Foreground: `ExecuteVerbTaskSyncWithStart` logs `slog.Error("task error")`
  (`:1021`) and returns the error; `executeCommandMatch` sends it to the
  player (`input_processor.go:773`). A wizard `kill_task`ing a player's
  running command makes the victim's connection print `context canceled`.
- Background: `runTaskBatch` logs ERROR `task error` (`runtime.go:556`).
- `barn_logs -level error` exits 1 on any ERROR record, and the plan's
  acceptance says "audit logs for panic/error", so a legitimate `kill_task`
  now reads as a server failure.

The queued-kill half was patched during my review (`runTask` returns nil on
`context.Canceled`). That patch has its own flaw: `errors.Is(err,
context.Canceled)` is also true when `s.ctx` is cancelled by `Runtime.Stop`,
so every background task waiting for admission at shutdown is marked
`TaskKilled`. `server.shutdown` checkpoints before `runtime.Stop`, so nothing
is persisted wrongly today, but shutdown is not a kill.

Reproduce (engine test, no sockets): park a foreground verb in a test builtin;
from another goroutine `KillTask` it; unpark. Expect
`ExecuteVerbTaskSyncWithStart` to return nil and no ERROR record; observe
`context.Canceled`. Second case: hold the only slot, reserve a fork, call
`s.Stop()`; the fork should stay `TaskQueued`, not `TaskKilled`.

Minimum fix: decide by task state, not by error identity. In `runTask` and in
the `Done` branch, `if t.GetState() == task.TaskKilled { return nil }`; on
runtime cancellation leave the state alone and return the error.

### F2 — Medium-low. Lifecycle and player work share one System principal; one path misses the input close

Partly fixed during review: exported `CallVerbWithArgstr` now admits with
`enterInput(player, false)` (`call_verb.go:183–191`), so
`do_out_of_band_command`, `do_blank_command` and the login fallback are
charged to the player/anonymous key and are cancelled by
`CloseInputAdmission`. Remaining:

- `RunServerVerbTask` hard-codes `system = true` (`task_factory.go`,
  `runServerVerbTask(…, nil, true)`). That covers `server_started`,
  checkpoint and shutdown hooks (correct) **and** `callUserHook`
  (`user_connected`, `user_created`, `user_reconnected`,
  `user_disconnected`, `user_client_disconnected`), which are per-player
  work. All of it lands on one `System`/`Background` principal with one
  ledger. A login storm raises that ledger and pushes `checkpoint_started`
  behind every player principal; conversely every player's `user_connected`
  is served at background class weight. The plan says input is attributed to
  its connection/player and only server-owned hooks use the system key.
- The internal fallback in `callVerbWithArgstr` (`ownsScope && scope == nil`,
  `:196–205`) is still System-keyed on `s.ctx`. It is reached by
  `host.VerbCaller`'s unclaimed fallthrough (`runtime.go:129`) — rare, fine.

Minimum fix: give `callUserHook` a player-keyed entry (`enterInput(player,
false)`), keep System for the four server lifecycle hooks.

### F3 — Low. `Scope.ResumeBackground` discards the group

`scope.go:38–41` rebuilds the key as `Key{Principal, Class: Background}`, i.e.
`Group: User`. Harmless for `Eval` today (always a real player), wrong if it
is ever reused for an anonymous or system scope. Carry `s.key.Group` over.

### F4 — Low (performance risk, measure). Global-mutex O(n) work per slice

- `Reservation.Finish` walks **every** principal ever seen to compute the
  watermark (`controller.go:255–261`), under the one controller mutex, once
  per slice. `principals` is never evicted. At the recorded ~550 slices/s and
  a few thousand distinct players/programmers this is millions of map
  iterations per second inside the hottest lock in the server.
- `dispatchLocked` is O(queue) per grant; `Request.Cancel` is O(queue).
- `scheduler.ReadyBatch` now `sort.SliceStable`s the ready list with a
  comparator that, per comparison, copies two call stacks
  (`backgroundAdmissionKey` → `GetCallStack`, which allocates) and takes the
  controller mutex, all while holding the scheduler mutex.

None is a correctness bug. Track min-settled incrementally (or evict idle
principals at the watermark) and precompute one key per ready task before
sorting. The real-workload 1p/16p gate is the judge.

### F5 — Low. Lane lifetime and bounds

- A forced phantom login uses `ConnID = int64(player)` (negative). That lane's
  goroutine and map entry only retire on `IsDisconnect` or shutdown, and no
  disconnect is ever sent for a phantom id, so each distinct phantom id leaks
  one goroutine.
- Lane queues are unbounded. Socket readers self-limit (they await `Done`),
  but `force_input` in a MOO loop can now grow a lane without bound where it
  previously blocked at 64 + 256.
- `CloseInputAdmission` is one-way on the `Runtime`; an `InputProcessor`
  cannot be stopped and restarted on the same runtime. Only matters to tests.

### F6 — Low. `pinAdmissionRoots` side effects

It fires `ExecutionStartObserver` and takes `VMStartMu` before admission
(`admission.go:50–54`). Tests that count observer calls as "VM starts" will
see an extra, earlier call per independent entry, and every such entry now
queues behind an in-progress sweep twice (pin, then lease). It also roots
`types.NewAnon(objID)` unconditionally; harmless if anonymous and numbered ids
share one allocator (`anonGCFloor := s.store.NextID()` suggests they do), but
worth a comment.

---

## 3. Remaining scope and limitations (not defects)

- **L1. The 3:1 class weight is intra-principal only** (`dispatchLocked`
  compares class ledgers only when both requests have the same identity).
  This follows the plan literally. In Mongoose nearly all background work runs
  as a wizard programmer while input is per player, so in practice background
  competes as one principal of weight 1 against each player, not at 3:1. If a
  global input:background share was intended, it is not what is built.
- **L2. Default `Limit = workerCount` (GOMAXPROCS) now also caps foreground
  lanes**, which were unbounded, and gate waiters hold slots by design (plan
  line 44). During a long checkpoint walk, `GOMAXPROCS` writers parked on the
  gate will queue read-only commands that previously ran (a write-free
  `Commit` returns before the gate). Tunable via `ADMISSION_LIMIT`; needs the
  1p/16p numbers before a default is defended.
- **L3. Mid-VM shared-commit waits are still not excluded from the MOO
  seconds budget** (`flushStagedBeforeCoarse`'s `CommitAndRenew`, `run_gc`
  renewals). They now reach the service ledger through the observer, but only
  the promotion calls `ExcludeExecutionWait`. Pre-existing; unchanged.
- **L4. `dump_database()` is now asynchronous.** The deleted comment said it
  "does not report success until the requested checkpoint is durable and
  available for managed restart adoption". The conformance suite tolerates
  this (explicit waits), and the harness stops Barn with
  `process.terminate()`, which on Windows is a hard kill. Two residual risks:
  `mainLoop`'s `select` can choose `ctx.Done` over a pending `checkpointChan`
  when both are ready, dropping the request when the final checkpoint is
  disabled; and `scripts/benchmark-mongoose.ps1` ends with `;dump_database()`
  and may now stop the server mid-dump. Draining `checkpointChan` once in the
  `ctx.Done` arm closes the first. The parent's managed run of
  `server/dump_database.yaml`, `server/boolean_dump_persistence.yaml`,
  `server/checkpoint_operational.yaml` and
  `audit/checkpoint_lifecycle_toast_oracle.yaml` is the evidence that matters.
- **L5. Replay ownership** still releases the grant on a promotion-time
  validation loss, contradicting plan lines 317–323. Deliberately unchanged;
  the plan should say so.
- **L6. Behaviour shift:** a suspend-time commit failure now fires
  `OnComplete` (result is an exception, `retErr == nil`), where the old code
  returned before the callback. For a login task this clears the login-task id
  and prints the traceback — arguably better, but it is a change.
- **L7.** Stale `ctx.Admission` pointers survive on suspended tasks' and
  deferred-GC contexts. Only `Waited` (atomic) can be reached through them,
  and the next slice overwrites the pointer; accounting for such waits is
  silently dropped, nothing worse.

## 4. Test coverage seen

`engine/admission_test.go` covers cap-one Eval, completion hook, error-hook
borrow, queued kill, root pinning, and lifecycle hooks after input close.
`internal/admission` and `internal/commitgate` have unit tests for monopoly,
cancellation races, principal cap, gate wait, class weights, FIFO cohorts and
owner-safe cancel. Not yet covered, from the earlier list: timeout-hook
borrow, sweep-hook exemption with a committing `:recycle`, admitted tasks
parked on `VMStartMu`, retry attempt ≥ 1 borrowing, concurrent ID-0 hooks,
kill of a *running* task (F1), shutdown leaving queued tasks queued (F1),
gate-wait accounting across `CommitAndRenew`, and a cap-one full conformance
run.

## 5. Minimum fixes

1. **F1:** treat kill as a normal outcome keyed on `TaskKilled` state; do not
   mark tasks killed on runtime-context cancellation; no ERROR record and no
   player-visible `context canceled`.
2. **F2:** player-key `callUserHook`; reserve the System key for
   `server_started`, `checkpoint_started/finished`, `shutdown_started`.
3. Optional, cheap: F3 (keep group), L4 drain of `checkpointChan` on shutdown.

With 1 and 2 applied — and the parent's test, race and managed conformance
runs green, which I have not seen — this is ready to commit as the
admission/gate increment. Performance claims remain out of scope until the
identical live 1/16-client matrix is run.
