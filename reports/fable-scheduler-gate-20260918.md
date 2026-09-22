# Barn scheduling with the commit gate inside the algorithm

Reviewer: Claude Fable (external design partner). Date: 2026-09-18.
Barn `fix/mongoose-workload` @ `ef5fcb0`. Toast
`/root/src/toaststunt-mongoose-login-20260917` @ `72e3c7f` (WSL `Debian`, verified
with `wsl --list --quiet` and `git rev-parse HEAD`).

Method: source reading only. No runtime change, no test run, no server started,
no conformance run, no timing reproduced. Every statement below is tagged:

- **[FACT]** — read directly in source at the cited line.
- **[INFERENCE]** — follows from cited facts but was not executed or tested.
- **[PROPOSAL]** — design recommendation; nothing is implemented or measured.

## 0. Bottom line

1. The gate is not a rare escalation path. **Every resumed slice — every
   `suspend()`, `read()`, and `exec()` continuation — takes the commit gate
   exclusively before its first instruction and holds it for the whole slice**
   (`engine/task_runtime.go:133-137`, released at `:495`). On a suspend-heavy
   core this is the dominant exclusive use, and it is where login continuations
   queue.
2. Exclusive waiters park on a raw `sync.RWMutex` (`db/store/store_core.go:126,
   280-283`). Order is decided by the Go runtime, not by policy; the wait is
   uncancellable; the scheduler cannot see it. No ready-queue policy, however
   fair, has any authority over this queue. The user's correction is right.
3. The "worker pool" is not where the contention lives. In production every
   scheduler-dispatched task is a solo batch, so the scheduler is serial. The
   real gate population is **one scheduler task + one goroutine per connection +
   checkpoint**. "Reduce the batch size" addresses a batch that is already 1.
4. Recommended smallest cohesive change: put a **policy-ordered arbiter in front
   of the exclusive side only**, make gate need a **dispatch-time eligibility
   condition** for tasks whose need is known in advance (all resumed slices),
   and turn the mid-slice boundary into **try-acquire, else abort-and-rerun
   under a pre-granted gate** — which the existing retry machinery already
   supports without VM changes. Invariant: *no goroutine ever waits for the gate
   while holding an execution lease, a VM, or a pool slot.*
5. Honest ceiling: nothing here preempts a holder. Foreground gate wait becomes
   "one residual hold + foreground holds ahead" instead of "every hold queued
   ahead in mutex order". The residual hold is bounded only by the task's
   tick/seconds budget **plus any synchronous I/O performed under the gate** —
   and `curl()` does exactly that today (`builtins/curl.go:50-51`).

## 1. Proven source facts

### 1.1 The gate

- **F1 [FACT]** The gate is `Store.commitGate sync.RWMutex`
  (`db/store/store_core.go:120-126`). Documented lock order: commitGate, then
  `store.mu` (`db/store/store_txn.go:2645-2648`, `db/store/store_snapshot.go:70`).
- **F2 [FACT]** Shared side: an ordinary `StoreTxn.Commit` with staged writes holds
  `RLock` for its whole validate+apply window (`store_txn.go:2649-2652`).
- **F3 [FACT]** Never touches the gate: `BeginReadOnly` (only `store.mu.RLock` plus
  read-timestamp registration, `store_txn.go:260-292`); every read; and a
  write-free commit, which returns at `store_txn.go:2636-2638` *before* the gate.
- **F4 [FACT]** Exclusive acquirers:
  - (a) slice start when the slice cannot be re-executed or the loss budget is
    spent: `(attempt >= escalateAfterAttempts || !retryState.canRetry)`
    (`task_runtime.go:133-137`, threshold 63 at `:44`);
  - (b) the first irreversible effect of a retryable attempt, via
    `ctx.BeforeIrreversibleEffect` (`task_runtime.go:198-235`), invoked by the
    `Irreversible` descriptor wrapper (`builtins/registry.go:102-117`) and by
    every coarse builtin through `beforeCoarse` (`builtins/store_reads.go:55-63`);
  - (c) the checkpoint object walk, `SnapshotWithRoots`
    (`store_snapshot.go:66-72`). The file write and fsync are outside the gate
    (`db/format/checkpoint.go:55-81`).
- **F5 [FACT]** `canRetry` is false for any task with a saved yielded VM:
  `taskIsConflictRetryable` requires `BytecodeVMValue() == nil`
  (`task_runtime.go:742-744`); the fork rebuilder declines when
  `saved.IsYielded()` (`:757-760`). A fork's *first* run is retryable
  (`:788-791`). Therefore class (a) = every resumed slice + checkpoint-loaded
  VMs + attempt ≥ 63.
- **F6 [FACT]** Hold extent: from acquisition to `releaseEscalation()` at
  `task_runtime.go:495` — all remaining bytecode of the slice and its commit.
  Suspension releases it ("Every suspension yields the commit gate", `:494`).
  A deferred backstop releases on early return or panic (`:120-124`); defers run
  LIFO so the gate is released before the lease and before `flushDeferredGC`
  (`:78-92`).
- **F7 [FACT]** Class (b) acquirers are, by construction, retryable attempts that
  have performed no irreversible effect and published nothing: the hook returns
  immediately when already escalated (`:199-201`), and being un-escalated at that
  point implies `canRetry` and `attempt < 63` (`:133`). The wrapper runs the hook
  *before* the builtin body (`registry.go:111-116`).
- **F8 [FACT]** At the boundary the hook validates all reads and **publishes the
  slice's writes so far** (`CommitAndRenewCarryingReads`,
  `store_txn.go:2461-2589`; comment at `task_runtime.go:194-197`). On validation
  loss with the effect still ahead it requests a re-run (`:221-224`); the re-run
  path releases the gate first, discards forks and pending effects, and loops
  (`:409-430`).
- **F9 [FACT]** Builtins that take the gate (`builtins/base_descriptors.go`): `exec`
  `:232`, `server_log` `:233`, `curl` `:164`, `set_connection_option` `:138`,
  `open_network_connection` `:142`, `read_http` `:143`, `flush_input`/`force_input`
  `:144-145`, `kill_task` `:252`, `resume` `:255`, `run_gc` `:249`, `listen`/`unlisten`
  `:127-128`, file mutators and readers `:170-194`, `sqlite_open/close/limit/
  interrupt` `:199-207`, `reseed_random` `:53`, `reset_max_object` `:247`,
  `read_stdin` `:214`. `dump_database` and `shutdown` are `CommitGateReentrant`
  (`:212,217`). `notify` is `Transactional` (`:125`): buffered, no gate.
- **F10 [FACT]** Some holders block on external I/O **while holding the gate**:
  `curl` performs a synchronous `client.Do` with a 5 s default, caller-extendable
  timeout (`builtins/curl.go:29-36,50-51`); `open_network_connection` dials
  synchronously (`builtins/connection.go:356`). `exec` does not: it suspends
  (`builtins/system.go:258,284`), so the gate drops at `task_runtime.go:495` and
  the subprocess runs ungated (`system.go:261-281`).
- **F11 [FACT]** Toast's `curl` is a suspending builtin:
  `return background_thread(curl_thread_callback, &arglist)` (`src/curl.cc:91`).
- **F12 [FACT]** Gate-wait accounting: the boundary wait is excluded from the task's
  seconds budget (`task_runtime.go:202-205`, `task/task.go:475-481`) and logged
  as `gate_wait` (`:211-216`). The slice-start wait (`:134`) happens before the
  deadline is set (`:275-280`) so it is outside the budget, but it is **not
  measured or logged anywhere**. `ready_to_vm` (`:298`) is sampled after the
  gate; `queue_wait` (`:297`) before it. Shared-side `RLock` wait inside `Commit`
  is not measured at all.

### 1.2 Execution slots and lanes

- **F13 [FACT]** `QueueTask` callers in non-test code: forks
  (`engine/task_factory.go:322`), checkpoint-loaded tasks
  (`engine/task_load.go:82,162`), and `Create{Foreground,Background}Task`
  (`task_factory.go:55,204`), which have no non-test callers. Forks and resumed
  VMs fail the `Plan` predicate, so each gets a solo batch
  (`engine/internal/scheduler/scheduler.go:159-163`); `Run` joins its batch
  (`:173-188`); `runReadyTasks` walks every batch of one snapshot
  (`engine/runtime.go:476-491`); one pass is in flight at a time, ticked every
  10 ms (`server/input_processor.go:182,196-231`).
  **[INFERENCE]** In production the scheduler executes one task at a time; the
  `GOMAXPROCS` pool (`runtime.go:54`) is essentially idle.
- **F14 [FACT]** Foreground bypasses the scheduler: command verbs
  (`task_runtime.go:967`), server hooks (`task_factory.go:117`), login hooks
  (`:188`) and `read()` resumption (`:394-416`) all call `runTask` on the caller's
  goroutine. Each connection has its own lane goroutine
  (`input_processor.go:237-274`), unbounded in number, one line at a time
  (`:167-175`). Lanes bypass the pool and the ready queue but **not the gate**: a
  `read()` continuation on a lane is a resumed slice and takes the gate
  exclusively at `:134`.
- **F15 [FACT]** An `exec()` continuation leaves the lane: `CompleteExec` sets
  `TaskQueued` (`task/task.go:642-658`); it is then found by the catalog scan in
  `Ready` (`scheduler.go:109-122`) on a later 10 ms pass and sorted ahead of other
  ready work (`:124-145`).

### 1.3 Lifecycle and GC barriers

- **F16 [FACT]** The execution lease is taken *before* the gate
  (`task_runtime.go:76` vs `:134`). `acquireTaskExecution` passes through
  `VMStartMu` (`runtime.go:193-203`).
- **F17 [FACT]** `flushDeferredGC` holds `SweepMu` then `VMStartMu` for the entire
  sweep including every `:recycle` hook (`engine/waif_lifecycle.go:229-235`), and
  fails closed if any lease exists (`:250-255`, `runtime.go:532-537`).
  **[INFERENCE]** (i) A sweep blocks *all* new task starts, foreground included,
  for its duration. (ii) A task parked on the gate holds a lease, so sustained
  gate contention postpones deferred GC indefinitely.
- **F18 [FACT]** Sweep recycle hooks run on a context with no transaction and no
  boundary hook (`waif_lifecycle.go:201-212`), i.e. direct live-store writes that
  bypass the gate (`store_core.go:277-279`). Mutual exclusion with gate holders
  comes from lease quiescence, not from the gate.
- **F19 [FACT]** `dump_database()` under an exclusive hold is deferred until release
  (`builtins/system.go:787-795`, `task_runtime.go:244-252,798-812`). The periodic
  checkpoint runs `checkpoint_started`/`finished` as ordinary synchronous tasks
  (`server/server.go:362-382,503-517`).

### 1.4 Cancellation

- **F20 [FACT]** `Task.Kill` flips state and cancels an exec subprocess
  (`task/task.go:684-696`). `Task.CancelFunc` is published (`:425-428`,
  `task_runtime.go:283-285`) but I found no non-test caller, and no VM poll of
  kill state (`grep` over `vm/` for `TaskKilled|GetState|Done()` is empty). The
  kill check happens after the VM returns (`task_runtime.go:498-505`), and
  `SetState` assigns unconditionally (`task/task.go:260-269`), so normal
  completion overwrites `Killed` at `task_runtime.go:676`.
  **[INFERENCE]** Killing a running holder does not shorten its hold, and its
  writes still commit. A goroutine parked in `commitGate.Lock()` cannot be
  cancelled by any means.

### 1.5 Toast

- **F21 [FACT]** One task per `run_ready_tasks` call (`src/tasks.cc:1648-1652`,
  `did_one`); queue reinserted after equal-usage peers (`<=`, `:399`); new queue
  adopts the head's usage (`:408-411`); input tried before background within the
  chosen queue (`:1675` then `dequeue_bg_task`).
- **F22 [FACT]** `usage += end - start` where both are `time(nullptr)`
  (`:1651,1804-1806`) — whole seconds.
  **[INFERENCE]** For sub-second tasks the delta is 0, so Toast's cross-queue
  policy is **round-robin among active queues**; usage only discriminates once a
  single execution crosses a second boundary.
- **F23 [FACT]** Queues share one Objid key space: input by player/connection
  (`:989,2893,2911`), background by the fork's `a.progr` or
  `progr_of_cur_verb` of the suspended VM (`:1637-1640,1189-1190,1327-1328`).
- **F24 [FACT]** At login the connection's queue is relabelled to the player and
  keeps its usage; background tasks owned by the negative id move to their own
  queue; the player's previous queue's background tasks are merged in
  (`:921-945`).

## 2. Resource and state graph (Q1)

```
 connection lane (1 goroutine / conn, unbounded)          10 ms ticker
   fresh command / hook / login  ──┐                   one pass in flight
   read() resume (G1)            ──┤                           │
                                   │                 Ready() snapshot, solo batches
                                   ▼                           ▼
                        runTaskSlice(t)  ◄───────── 1 pool worker at a time
                                   │
   R1 lease: VMStartMu (transient) → ExecutingTasks[t]++            F16
                                   │        ▲ blocked for a whole GC sweep  F17
                                   ▼
   R4 gate, EXCLUSIVE at slice start if G1                           F4a
                                   │
                         VM runs on a snapshot (R5 store.mu.RLock per read)
                                   │
   first irreversible/coarse builtin ─► R4 EXCLUSIVE + validate + publish   F4b,F8
                                   │
                                   ▼
   commit: write-free → no gate (F3) │ writes → R4 SHARED (F2), or exempt if holder
           then store.mu RLock + slot mutexes, or store.mu Lock (coarse)
                                   │
   release gate (:495) → release lease → flushDeferredGC (SweepMu → VMStartMu)

   checkpoint goroutine: hooks as tasks → R4 EXCLUSIVE for object walk (F4c)
```

Gate states for one task, stated precisely:

| State | Meaning | Who is affected | Exit |
|---|---|---|---|
| **speculative** | running on a snapshot, no gate | nobody | boundary, commit, suspend |
| **acquiring-exclusive** | parked in `RWMutex.Lock()` | Go's RWMutex gives a pending writer preference over *new* readers, so every new ordinary write-commit also stalls behind this waiter, even before it holds | grant only; not cancellable (F20) |
| **holding-running** | executing bytecode under the gate | all write-commits and all other exclusive requesters; read-only tasks unaffected (F3) | commit or suspend (`:495`) |
| **holding-blocked** | inside a synchronous builtin under the gate (F10) | same, for the duration of the I/O | I/O completion or timeout |
| **acquiring-shared** | in `RLock()` inside `Commit` | only this committer | any exclusive hold ending |
| **released** | `:495` or backstop `:120` | — | — |

**Safe handoff points** — where the gate can change hands with no MOO-visible
consequence beyond what exists today:

1. *Before acquisition*, for G1 tasks at slice start: no VM instruction has run,
   no snapshot is open. Deferring is pure queueing.
2. *Before acquisition*, at the boundary (F7): nothing published, no effect
   performed. The attempt can be abandoned and re-run (F8) — that path exists.
3. *After release* (`:495`).

There is **no** safe handoff inside a hold. Releasing mid-hold forfeits the
cannot-lose guarantee after an irreversible effect (the issue-#296 phantom
`E_INVARG`, `task_runtime.go:127-132,178-192`); parking mid-hold freezes every
write-commit in the server.

## 3. Adversarial timelines (Q2)

Notation: A = background task, B = a login, H = holder.

**T1 — A holds after an irreversible effect; B's login arrives.**
A (fork, first run) calls `server_log` → takes gate (F4b), keeps running.
B's `do_login_command` is a fresh retryable task on B's lane (F14): it runs
speculatively, unblocked. Then one of three things:
- B's slice is write-free → completes and flushes output with **zero** gate
  interaction (F3). Not inverted.
- B's slice stages writes → parks in `RLock` at commit until A releases; if A's
  second half touched B's reads, B loses validation and re-runs.
- B calls `set_connection_option` (F9) → parks in `Lock()` behind A.
Then B suspends in `read()`. **Every later line B types resumes a G1 slice that
takes the gate exclusively at `:134`** — this, not the first slice, is where a
login repeatedly meets the gate.

**T2 — "all workers block acquiring it".** As a pool scenario this cannot occur
today: one scheduler task is in flight (F13). The real form: N lanes each with a
`read()`/boundary acquisition + the single scheduler task + possibly the
checkpoint, all parked on one mutex, each holding a lease (F16), in an order no
policy controls. Pool exhaustion becomes real the moment parallel dispatch is
introduced — so the no-exhaustion rule must ship *with* that change, not after.

**T3 — B read-only vs B needs commit.** Read-only: never waits (F3).
Needs shared commit: waits at most one exclusive hold — `RWMutex.Unlock` releases
blocked readers before the next writer proceeds **[INFERENCE from Go's
`sync.RWMutex` semantics; not re-verified against the toolchain in use]**.
Needs exclusive: waits for every exclusive request ahead of it.
Caveat **[INFERENCE]**: a reader whose snapshot begins between H's boundary
publish (F8) and H's final commit observes H's first half only, and if
write-free it completes and emits output on that view (F3). The source comment
"none of them can commit on that view until this attempt has"
(`task_runtime.go:196-197`) holds for writers only. Pre-existing, not caused by
scheduling, but it means "readers proceed safely" needs the qualifier
"crash-safe and snapshot-consistent, not slice-atomic with respect to a
boundary-crossing holder".

**T4 — repeated optimistic conflicts trigger escalation.** A fresh task loses 63
times (`:44`), then takes the gate at slice start and re-executes from the top
under it. Each lost attempt is full wasted execution, unaccounted to anyone. The
tuning note (`:38-43`) records that escalating early (8) serialized the server;
that is existing evidence that gate holds are expensive at 16p, not a
measurement I reproduced.

**T5 — holder awaits an external result.** `exec`: not a problem (F10).
`curl`: H holds the gate across an HTTP round trip with a caller-chosen timeout.
For that interval no task in the server can commit a write and no resumed slice
can start. No queue or arbiter policy can bound this; only removing the I/O from
under the gate can. Toast suspends here (F11).

**T6 — heavy foreground starves background.** Today, accidentally bounded: lanes
and the scheduler are separate goroutines, so background always gets CPU; and
RWMutex writer ordering is roughly arrival-ordered. The moment an arbiter prefers
foreground, starvation becomes possible by design and needs an explicit bound
(§7). Background *commits* additionally lose validation more often under a
foreground write stream — a fairness cost no gate policy sees unless retries are
accounted.

**T7 — a checkpoint or GC waits.** Checkpoint: parks in `Lock()` like anyone;
while parked, writer preference stalls new shared commits; once granted, holds
for an O(objects) walk (F4c). GC sweep: needs zero leases (F17); under sustained
load, and especially while gate waiters hold leases, the flush keeps failing
closed; when it does run it blocks every new task start. Neither is visible to
the scheduler today.

**T8 — pre-login transitions owner.** The pre-login lane uses player `-connID`
(`input_processor.go:126-129`); the login task's owner is that negative id,
its programmer the verb owner (`task_factory.go:155-162`). Login completes in
`OnComplete` *after* gate release (`task_runtime.go:495,710-712`), so no gate
request of the login task spans the transition. A welcome-hook `exec`
continuation does span it — but its principal is a programmer, not the
connection. **[PROPOSAL]** Key *service* by programmer, key *interactive lineage*
by connection id (stable across login); see §6.4.

**T9 — cancellation kills a waiter or a holder.** Waiter: impossible today
(F20) — a disconnected client's lane stays parked, holding a lease, and then
runs its slice under the gate for nobody. Holder: the kill is not observed until
the slice ends; the writes commit; state is overwritten to Completed (F20).

## 4. Policy comparison (Q3)

| Approach | Fixes | Cannot fix |
|---|---|---|
| **Queue-only fairness** (per-principal FIFOs, reconsider after each task) | snapshot-drain delay for `exec` continuations (F15); cross-principal fork floods | anything parked on the mutex; lanes (they never enter the queue, F14); order among exclusive waiters; T5, T7, T9 |
| **Gate-aware admission** (do not dispatch a G1 task unless it can have the gate) | pool slots parked on the gate; GC blocked by waiter leases; makes waiters visible and killable | mid-slice acquisitions; lanes unless they participate; residual hold |
| **Priority inheritance / donation** | nothing material: a Go goroutine has no priority to donate to, and a holder is already running at full speed. Its one useful consequence is an invariant — *a holder must never be in a non-running state* (granted-but-unscheduled, waiting for a lease, blocked on I/O) | residual hold |
| **Unified execution/gate arbiter** | all of the above in one ordering; cancellable waits; bounded foreground preference that also covers checkpoint and sweep | residual hold; `holding-blocked` builtins (needs F10 fixed separately) |
| **Optional serialized mode** (every task G1) | gives a deterministic, Toast-shaped reference schedule for differential tests; an operator fallback | throughput (`:38-43`); read-only lanes would queue too |

What helps **without new MOO-visible interleavings**: reordering *who gets the
gate next* (order among concurrent gate waiters is already nondeterministic);
deferring the *start* of a G1 slice (pure queueing, provided same-principal FIFO
is preserved — F21); abandoning a pre-boundary attempt (F7/F8, existing
behaviour); cancelling a wait that has not started its slice.

What **cannot be bounded** without a semantic change: the length of one hold
(task seconds limit + synchronous I/O under the gate); the checkpoint walk; the
sweep; the number of optimistic losses before attempt 63.

## 5. Parking an effectful task before the gate (Q4)

**Yes, and it needs no VM work — but not by literally parking the VM.**

[FACT] The VM is resumable only at builtin-returned flows; the boundary hook is
called on the Go stack inside the builtin wrapper (`registry.go:104-116`). There
is no "re-execute this call on resume" mode, so freezing the VM *at* the builtin
would be new VM machinery.

[FACT] It is unnecessary. By F7 every boundary acquirer is a retryable attempt
with nothing published and nothing external done. The existing
`FlowAbortAttempt` unwind (`store_reads.go:46-50`, `vm/op_misc.go:92`) plus the
`ConflictRetryRequested` path (`task_runtime.go:409-430`) already discards forks
and pending effects and re-runs from the top.

[PROPOSAL] At the boundary: `TryExclusive`. If granted → today's fast path,
zero extra cost. If not → request a re-run exactly as a validation loss does,
release the lease, return the goroutine, queue an arbiter request, and re-run
the task **from the top as a G1 slice with the gate pre-granted**.

- *State lost:* the attempt's prefix — by design; it is rebuilt by re-execution.
- *Effects duplicated:* none; the effect had not run, pending effects are
  discarded (`:426`).
- *Revalidation:* none; the re-run reads under the gate.
- *Cost:* at most one wasted prefix per task (today a conflicted task may waste
  up to 63), and the re-run holds the gate for the whole slice rather than its
  tail. This is the real trade and must be measured (§9).
- *Not available for:* resumed slices — but they never reach the boundary
  un-escalated (F7), so the case does not arise.

**Materially different from yielding after taking the gate: yes.** After the
grant the hook has already published the first half (F8) and the effect is
imminent. Parking there either freezes all commits server-wide or, if the gate
is released, reopens #296. Before the grant there is nothing to protect.

The larger lever, *not* in the smallest change: make resumed slices re-runnable
by snapshotting the saved VM at resume. That would delete class (a) for resumed
tasks — the dominant exclusive use — and let read-only continuations run
ungated. A lazy "take the gate at first write" for resumed slices **without**
that snapshot is unsafe: a validation loss at that point leaves a VM whose locals
were computed from stale reads and cannot be rebuilt — #296 again.

## 6. Recommended design (Q5) — all [PROPOSAL]

### 6.1 Shape

1. **Arbiter in front of the exclusive side only.** Keep `commitGate` as the
   mechanism. Ordinary commits keep their bare `RLock` — the commit-dominated hot
   path (`store_core.go:67-74` records how sensitive it is) gains nothing. All
   exclusive requesters (`task_runtime.go:134,203`, `store_snapshot.go:71`) go
   through `Arbiter.Request`; only the single granted requester calls
   `commitGate.Lock()`, where it waits solely for in-flight shared commits to
   drain.
2. **Gate need is a dispatch-eligibility condition.** `needsGate(t)` =
   `!canRetry || attempt >= escalateAfter || requeuedFromBoundary`, computable
   before dispatch (F5). A G1 task is dispatched only with a grant in hand;
   `runTaskSlice` receives the grant instead of executing `:134`.
3. **Boundary = try, else abort-and-rerun as G1** (§5).
4. **Lanes participate** in the same arbiter, with a cancellable wait taken
   *before* `runTask` (so before the lease, fixing F16/F17-ii).
5. **Checkpoint walk and deferred-GC sweep become `maintenance` requests** to the
   same arbiter, so no task is ever granted the gate and then stalls on
   `VMStartMu` behind a sweep.

### 6.2 State machine

```
            ┌────────── kill / disconnect ──────────┐
            ▼                                       │
 READY(G0) ──dispatch──► RUNNING-SPEC ──boundary, TryExclusive ok──► RUNNING-HELD
    ▲                        │  │                                        │
    │                        │  └─boundary, busy─► abort attempt ─► READY(G1)
    │                        │                                           │
    │                  commit/suspend                         Request → GATE-WAIT
    │                        │                                 (no lease, no VM,
    │                        ▼                                  no slot; killable)
    └──── wake ◄── SUSPENDED / DONE ◄── commit/suspend ◄── RUNNING-HELD ◄─grant+dispatch
```

`GATE-WAIT` is a queue state, visible to `queued_tasks()` and `kill_task`, not a
blocked goroutine.

### 6.3 Pseudocode

```go
type class int // foreground > continuation(interactive lineage) > background; maintenance ages in

type request struct {
    class     class
    principal ObjID      // programmer; see 6.4
    connID    int64      // interactive lineage, 0 if none
    enqueued  time.Time
    seq       uint64
    granted   chan grant // buffered(1)
}

// Arbiter orders exclusive requesters; it never touches the shared path.
func (a *Arbiter) TryExclusive(r *request) (grant, bool) {
    a.mu.Lock(); defer a.mu.Unlock()
    if a.held || a.outranked(r) { return grant{}, false }
    a.held = true
    return a.newGrant(r), true       // caller then does store.commitGate.Lock()
}
func (a *Arbiter) Request(r *request)              { /* push; grantNextLocked() */ }
func (a *Arbiter) Cancel(r *request)               { /* remove; if already granted → Release */ }
func (a *Arbiter) Release(g grant, held time.Duration) {
    a.mu.Lock()
    a.held = false
    a.ledger.charge(g.principal, held)             // 6.4
    a.fgStreak = nextStreak(a.fgStreak, g.class)
    a.grantNextLocked()
    a.mu.Unlock()
    a.wakeDispatcher()
}
func (a *Arbiter) pickLocked() *request {
    if w := a.oldest(background, maintenance);
        w != nil && (a.fgStreak >= foregroundBurst || age(w) >= backgroundMaxWait) {
        return w                                   // bounded preference
    }
    return a.best()  // class, then least-service principal, then enqueue order
}

// Dispatcher: event-driven (task ready | task finished | gate released | tick).
func (d *Dispatcher) step() {
    d.admitDue()                                   // into per-principal FIFOs
    for d.inflight < d.slots {                     // slots = 1 until parallel dispatch
        t := d.pick()                              // head-of-FIFO of the best principal
        if t == nil { return }                     //   whose head is not gate-blocked
        if needsGate(t) {
            g, ok := d.arb.TryExclusive(t.req)
            if !ok { d.arb.Request(t.req); d.blockPrincipal(t); continue }
            t.grant = g
        }
        if t.TryClaimQueued() { d.handoff(t) }     // never blocks: a slot is reserved
        else if t.grant.valid() { d.arb.Release(t.grant, 0) }
    }
}

// Lane, for a read() resume (G1) or a boundary re-run:
select {
case g := <-req.granted: run(t, g)
case <-conn.Done():      arb.Cancel(req); killAndRemove(t)
}
```

Only the *head* of a principal's FIFO is eligible, so a gate-blocked head blocks
its own principal and nobody else. That preserves Toast's same-queue FIFO (F21)
while staying work-conserving across principals.

### 6.4 Owner, lineage and accounting policy

- **Principal = the programmer Toast would use** (F23): fork → programmer at the
  fork statement (already what `task_factory.go:266-269` records); suspended task
  → programmer of the *top activation at suspension*. `Task.Programmer` is the
  root verb's owner and is not a substitute; capture at the suspend site.
  Unauthenticated input has no programmer principal of its own: use the
  connection id *and* an aggregate "unauthenticated" bucket, because a newly
  created principal starts at the current minimum (F21) and a connection flood
  would otherwise mint unlimited fresh share. Toast has the same exposure.
- **Interactive lineage is keyed by connection id**, survives login (T8), is
  inherited across `read()`/`exec()` by the same task, is **not** inherited by
  forks (Toast treats forks and resumed tasks as background — see the comment at
  `task_runtime.go:254-258`), and expires after `interactiveBudget` of service so
  a `suspend(0)` loop started from a command cannot hold foreground class
  forever. This is an explicit Barn enhancement over Toast, not parity.
- **Charged:** execution occupancy (wall time with a VM running — not CPU time),
  including wasted attempts, because the ledger records resource consumption, not
  blame; and gate-hold time *again*, since a held second denies commit to the
  whole server. Wasted-attempt time is also exported as its own metric.
- **Never charged:** queue wait, gate wait, lease wait. They feed aging only, and
  stay outside the MOO seconds budget as today (F12).
- **Rejoin credit:** Toast's rule — a reactivated principal adopts the current
  minimum (F21). Sleeping must not bank credit.
- **Caveat [INFERENCE]:** on a LambdaCore-shaped database most verbs are owned by
  a few wizard characters, so programmer principals are coarse. The three
  distinct Mongoose owners in the audit make fairness relevant to *that* path;
  they do not show that principals are well-distributed across the workload. A
  per-principal cap keyed on a wizard-owned programmer could throttle the whole
  MOO; any cap must apply to background class only.

### 6.5 Lock-order invariants

Total order (outermost first):

```
Arbiter grant → store.commitGate → SweepMu → VMStartMu → Runtime.mu / lifecycle.Mu
             → store.mu → per-slot mutex → task.mu
```

- I1. Arbiter's own mutex is a leaf: never held across any other acquisition or
  across a channel send that can block.
- I2. **No goroutine waits for a grant while holding a lease, a VM, or a pool
  slot.** (Boundary contention abandons the attempt first.)
- I3. A grant is issued only to a requester that can run immediately: dispatcher
  grants are attached at handoff with a slot reserved; a grant whose task died is
  released at once. This is the useful residue of priority inheritance.
- I4. The sweep takes a `maintenance` grant *before* `SweepMu`/`VMStartMu`.
  `run_gc()` is already Irreversible (F9), so its caller holds the gate before
  `SweepMu.TryLock` (`runtime.go:126`) — consistent with the order above.
- I5. The shared path never consults the arbiter.

**No-worker-exhaustion rule:** *a goroutine drawn from a bounded pool may hold the
gate only if it was granted before handoff; it may never block on it.* With I2
this holds for lanes too, which is what keeps leases from pinning GC.

### 6.6 What stays unchanged

MOO yield/transaction boundaries; the boundary's validate-and-publish semantics;
the #296 cannot-lose guarantee; retry caps; solo execution of non-retryable
tasks; the shared commit path; `queued_tasks()` visibility (it improves: gate
waiters become ordinary queued tasks).

## 7. Operator knobs and honest guarantees (Q6) — [PROPOSAL], defaults unmeasured

| Knob | Meaning |
|---|---|
| `foreground_burst` | max consecutive foreground-class grants/dispatches while lower-class work waits |
| `background_max_wait` | age at which a background or maintenance request outranks foreground |
| `interactive_budget` | service a lineage may consume at foreground class before demotion |
| `serialized` (diagnostic, off) | every task is G1 — a deterministic reference schedule |

Deliberately **not** a knob yet: per-principal in-flight cap. With one scheduler
slot and one lane per connection it is inert; it becomes meaningful only with
parallel dispatch, and should arrive with it. Principal weights likewise: start
with equal weights and add the table only when a workload needs it.

**Can promise (once implemented and validated):**
- Foreground exclusive wait ≤ one residual hold + the foreground holds ahead.
- Background receives at least one grant per `foreground_burst + 1` grants, and
  waits no longer than `background_max_wait` + one residual hold.
- Gate waiters are killable and hold no lease, VM, or slot.
- Same-principal FIFO is preserved.

**Cannot promise:**
- Any absolute latency. The residual hold is bounded by the task's tick/seconds
  limits *plus* synchronous I/O under the gate (F10) — for `curl`, by a timeout
  the MOO programmer chooses.
- Anything about the checkpoint walk or a GC sweep beyond *when* they start.
- Read-side latency: reads never touch the gate, so they are already unaffected
  and gain nothing.
- That preference helps when the foreground itself is the contended class — N
  simultaneous logins still serialize their `read()` continuations.

## 8. Claims in our reports that are wrong or too strong (Q7)

1. **Audit, opening: "Toast's protection … is per-queue service accounting."**
   Too strong. Usage is whole seconds (F22); for the sub-second tasks that
   dominate, the mechanism is round-robin between queues + one task per loop +
   per-queue FIFO. Weighted service accounting is a Barn *enhancement*. The audit
   mentions the granularity but headlines the accounting.
2. **Audit §"Where Barn diverges" and the research summary imply a batching
   problem.** Batches are already size 1 in production (F13). What hurts is the
   *snapshot length* (no reconsideration until the whole snapshot drains) and the
   10 ms pass cadence — not batch width. **Reducing batch size therefore changes
   nothing**, and even dispatch-one-then-reconsider fixes only ready-queue delay:
   it has no authority over lanes (F14), over order among gate waiters, or over a
   held gate. If the 39 s login is mostly gate wait rather than queue wait, a
   perfect queue leaves it intact. **Which it is has not been measured** — and
   cannot be from current logs, because slice-start gate wait is unlogged (F12).
   A partial discriminator already exists: `ready_to_vm − queue_wait` at
   `task_runtime.go:296-298` is lease wait + slice-start gate wait for `exec`
   continuations.
3. **Audit step 4, "Preserve the bounded MVCC worker pool"; research, "many
   speculative background tasks can exhaust workers."** The pool is effectively
   unused (F13). Exhaustion is a risk of the *proposed* parallelism, not of the
   current system.
4. **Audit, "Barn holds an exclusive commit gate after certain irreversible
   effects (`task_runtime.go:185`)."** Understated. It also holds it for every
   resumed slice from its first instruction (F4a/F5), for coarse builtins, and
   for the checkpoint walk.
5. **Research open question, "Does the execution gate serialize all useful
   foreground work?"** Answerable from source: read-only work is entirely ungated
   (F3); disjoint writers commit concurrently under the shared side — *unless*
   any exclusive holder or waiter exists, in which case all writers stall at
   commit, not during execution.
6. **Candidate: "per-principal in-flight cap."** Inert today (§7).
7. **Candidate: "separate interactive latency preference."** Unbounded by
   default; and exec continuations being background is Toast's behaviour, so
   foreground lineage for them is a deliberate divergence in *scheduling order*
   (not in MOO semantics) and should be labelled as such.
8. Source comments worth fixing alongside, since designs get built on them:
   `task_runtime.go:24-28` ("conflicts only arise between tasks committing inside
   the same optimistic batch") predates concurrent lanes; `:196-197` holds for
   writers only (T3).

## 9. Trace points and deterministic schedules (Q8)

**Trace points** (all additive attrs; none of the conformance-pinned message
text changes):

- `gate.request / grant / release` with `mode` = slice_start | boundary |
  escalation | checkpoint | sweep, `class`, `principal`, `conn_id`, `wait`,
  `hold`, `holder_task`. *Slice-start wait is the single most important missing
  number* (F12).
- `commit.shared_wait` — time inside `RLock` (currently invisible).
- `boundary.abort_requeue` with wasted prefix duration.
- `attempt.wasted` — duration of each lost attempt, by principal.
- `lease.wait` — time in `VMStartMu`; `sweep.begin/end`; `flush.failed_closed`.
- `dispatch.pick` — chosen principal, runner-up, reason (class | service | age |
  gate_blocked).
- For external completions: completed → ready-visible → picked → granted → VM.
  The first gap is the 10 ms catalog scan (F15); the third is the gate.

**Deterministic schedules.** Use test builtins as barriers, in the style of
`bumpReadValueLiveOnce` and `assertCommitGateReleased`
(`engine/irreversible_effect_boundary_test.go:42-75`). Each schedule separates
policies that the others cannot:

| # | Schedule | Queue-only | Arbiter |
|---|---|---|---|
| S1 | bg holder parked on a barrier; enqueue 3 bg G1 then 1 fg G1; open barrier | fg runs 4th | fg runs 1st |
| S2 | S1 with a continuous fg stream | — | a bg grant within `foreground_burst + 1` |
| S3 | G1 waiter, then kill it, then open barrier | cannot be expressed (uncancellable) | slice never runs; no lease was held |
| S4 | holder parked; fork flood from principal P; one fork from Q | Q behind the flood (global snapshot) | Q runs after ≤ 1 P task |
| S5 | fresh task reaches boundary while gate held | goroutine parks holding a lease; a concurrent `flushDeferredGC` fails closed | attempt aborted; flush succeeds; re-run once under grant; effect executed exactly once |
| S6 | same principal: resumed task (G1) then fork (G0), gate held | — | fork does **not** overtake (same-principal FIFO) |
| S7 | checkpoint requested during fg stream | unordered | starts within `background_max_wait`; never granted mid-sweep |
| S8 | write-free fg slice while gate held | completes without waiting | identical — guards against regressing F3 |
| S9 | `serialized` on vs off, same corpus | — | identical MOO-visible results; differential oracle |

S3, S5 and S7 discriminate *arbiter* from *gate-aware admission only*; S1/S2/S4
discriminate both from queue-only.

## 10. Rejected alternatives

- **Smaller batches / more frequent passes alone** — batches are already 1; no
  authority over the gate or lanes (§8.2).
- **Priority inheritance** — nothing to donate to (§4). Its invariant is kept as I3.
- **Freezing the VM at the builtin** — new VM machinery for something the retry
  path already provides (§5).
- **Yielding after the grant** — freezes all commits or reopens #296 (§5).
- **Lazy gate for resumed slices without a VM snapshot** — unrecoverable
  validation loss (§5).
- **Replacing `commitGate` wholesale with an arbiter lock** — puts a mutex on the
  commit hot path for no ordering benefit (I5).
- **Instruction-level preemption** — agreed with both reports: a separate
  semantic project.
- **A faithful EEVDF** — needs slice lengths we cannot bound.
- **Serialized mode as the fix** — existing tuning data says it is expensive
  (`task_runtime.go:38-43`); keep it as a diagnostic.

## 11. Recommended phases

0. **Measure first.** Add the §9 trace points only. Decompose the login delay
   into queue wait / lease wait / slice-start gate wait / shared-commit wait /
   execution / external. *If gate wait is small, Phase 1 alone may suffice and
   Phase 2 should be re-justified.*
1. **Queue.** Event-driven dispatcher, per-principal FIFOs, reconsider after each
   task, wake on `CompleteExec` instead of the 10 ms scan. Replace the internal
   contract in `engine/scheduler_fairness_test.go:13-32` (an implementation
   assertion, not a MOO requirement).
2. **Gate.** Arbiter on the exclusive side; G1 eligibility at dispatch; boundary
   try-else-rerun; lanes and checkpoint as clients; cancellable waits. S1–S8.
3. **Hold hygiene.** Move synchronous I/O out from under the gate, `curl` first.
   Conformance-first: a Toast-passing test showing other tasks run during
   `curl()` belongs in `moo-conformance-tests` before any Barn change.
4. **Sweep as a maintenance client** (I4), then bounded preference knobs.
5. **Optional, separately justified:** re-runnable resumed slices (VM snapshot at
   resume); parallel dispatch of first-run forks with the in-flight cap.

## 12. Unresolved choices (for Q)

1. Charge gate-hold time double, or keep a separate gate ledger?
2. Should an `exec` continuation with interactive lineage outrank a fresh
   foreground command, equal it, or sit between foreground and background?
3. Boundary contention: always abort-and-rerun, or allow a short bounded inline
   wait first? (Trades wasted CPU against an I2 exception. I recommend no
   exception.)
4. Is a whole-slice hold on the re-run acceptable for login verbs with long
   prefixes before `set_connection_option`? Needs Phase 0 data.
5. Unauthenticated aggregate bucket: one bucket, or per source address?
6. Should kill actually reach a running task (F20)? Out of scope here, but it
   determines whether "kill the holder" is ever a remedy.
7. Is threading on by default in the pinned oracle build, so that Toast's `curl`
   suspension (F11) is the behaviour a conformance test would observe? Not checked.

## 13. Not verified

- Nothing was executed. All behavioural statements about Barn are from source;
  the T3 half-slice visibility and the RWMutex reader-release ordering are
  inferences.
- I did not read `commitDecentralized` (`db/store/store_cow.go:358`) or the VM's
  suspend/resume internals; §5's claim that no "re-execute the call" resume mode
  exists rests on the builtin-wrapper call shape, not on a full VM read.
- I did not confirm that `ctx.Programmer` tracks the top activation at
  suspension; §6.4 states the requirement, not that Barn meets it.
- F20's "no production caller of `CancelFunc`" is a grep result over non-test,
  non-worktree Go files.
- No timing in this report is a measurement of mine.
