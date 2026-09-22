# Fable review: cooperative transaction scheduling design

Date: 2026-09-18. Reviewer: Claude Fable. Baseline `e86ecd0`.
Reviewed: `plans/barn-transaction-scheduling-design.md` (full),
`reports/research-barn-cooperative-mvcc.md`,
`reports/research-barn-transaction-contention.md`.
Method: one bounded read-only pass. Source was opened only to settle a concrete
design question (files and lines cited inline). No tests, no runtime, no
subagents. Toast attribution was checked by reading
`/root/src/toaststunt/src/tasks.cc` in WSL, not by running the oracle.

## Decisive recommendation

**Accept the direction; do not accept the document as an implementation
contract yet.** Keeping OCC/MVCC, scheduling at existing cooperative
boundaries, serial irrevocability, and *not* importing an STM is the right
call, and the document is unusually honest about what it does not prove.

Split the verdict by stage:

| Stage | Verdict |
| --- | --- |
| 1. Persistent ready membership, select-after-each-completion, event wakeups | **Build now.** This is the only stage aimed at the measured symptom (the 10 s trace is *pre-dispatch* delay, by the design's own account). |
| 2. Gate as explicit semaphore with capabilities, lease→gate order retained | **Build, but re-justify.** It buys cancellation, ownership tokens and observability. It does **not** buy starvation freedom — the current `sync.RWMutex` already has that (F6). |
| 3. Pre-start gate admission, provisional lease, offer/claim, maintenance handoff | **Do not build as written.** This is the one genuinely invented protocol in the design, it has three concrete defects (F2, F3, F4), and no measurement shows lease-holding gate waiters are the bottleneck. Gate it on evidence and apply the corrections below. |
| 4. Whole-server admission with "one active slice per principal" | **Blocked on F1.** As specified it serializes the 16-client login path that is the project's perf gate. |
| 5. Rollback / fine-grained irrevocability | Correctly deferred. Nothing in stages 1–4 should be described as progress toward it. |

## Ranked findings

### F1 (serious) — Principal definition plus "one active slice per principal" collapses the server into a few serial lanes

Pointers: design §Runnable policy ("Default principal is the top activation's
programmer at admission/fork/resume, matching the inspected Toast
attribution"; step 4 "Keep one active slice per principal"; "Pre-auth
connections share a server-configured anonymous principal").

Source facts:

- Toast attributes **input** to the *player's* queue and only **background**
  work to the programmer: `tasks.cc:987-989` `new_task_queue(player, …)` →
  `find_tqueue(player, 1)`; `tasks.cc:1186-1190` `enqueue_waiting` uses
  `forked.a.progr` / `progr_of_cur_verb(suspended.the_vm)`. The design's
  "matching Toast" sentence is true for background only.
- In Barn the top activation's programmer is the **verb owner**
  (`engine/call_verb.go:274,286`; `engine/task_runtime.go:334`). In
  LambdaCore-family databases, including Mongoose, command verbs and
  `#0:do_login_command` are wizard-owned.
- For input, the programmer is not even known at admission: it is discovered
  by command parsing and verb lookup, which is itself transactional execution.

Timeline (literal reading, 16 clients logging in, the perf-gate scenario):

```text
t0   16 connections deliver their first line. All are pre-auth → one shared
     anonymous principal; or, by programmer, all map to the wizard owning
     #0:do_login_command.
t0   Policy admits 1 slice (one active slice per principal). 15 wait.
t0+L slice 1 completes; slice 2 admitted. ...
t0+16L last login starts.  Today: 16 connection goroutines run concurrently
     (server/input_processor.go:29-34).
```

If one of those slices parks in a native builtin, or is a known-exclusive
slice holding the principal slot while it waits in the gate queue (the design
reserves "its principal admission slot" *before* the ticket), every other
slice of that principal — including interactive input — waits for the whole
gate queue ahead of it. The 3:1 input/background ratio governs selection
frequency, not latency; it cannot help while the single slot is held.

Toast can afford programmer-keyed queues because it is single-threaded anyway.
Barn would be paying Toast's serialization *and* MVCC's overhead.

Correction: separate the **fairness ledger key** from the **concurrency cap**.

- Ledger key: input → the connection's player (pre-auth: the connection,
  charged against a shared anonymous *budget*, which is a weight, not a
  concurrency limit of one). Background → programmer at fork/suspend, as Toast.
- Concurrency: input is already limited to one slice per connection by
  ordering. For background, the stated reason for the cap is "unbounded native
  continuations" — so cap *parked native continuations per principal*, not
  slices. Make the per-principal slice cap a tunable ≥ 1 with a default above 1.
- With more than one in-flight slice per principal, "charge at completion" is
  no longer enough: k slices start before any debt lands. Charge a provisional
  advance at dispatch (running mean slice cost) and reconcile at completion.

### F2 (serious) — Reserving a pool executor before every known-exclusive ticket re-creates the worker occupancy it was meant to remove

Pointers: design §Commit admission ("Before enqueueing a gate ticket they
reserve an executor slot and its principal admission slot"); research
cooperative-MVCC §3 (Calvin: "a known resource wait should not occupy a scarce
execution worker").

Every resumed saved VM is nonrestartable (`engine/task_runtime.go:133`,
`!retryState.canRetry`), so "known-exclusive" is the *common* background case,
not a rarity.

```text
workers = 4
t0  four resumed tasks R1..R4 (four principals) become ready.
    Each reserves a pool slot, then takes tickets 1..4.
t0  R1 claims, runs 2 s under the gate (synchronous I/O builtin).
t0+ε player P types a command: optimistic, needs no gate until commit.
    No executor slot free — R2..R4 hold three while doing nothing.
t2s R1 releases … P starts only after the ticket queue drains a slot.
```

That is today's behaviour (gate waiters occupying workers) minus the GC lease.
The time is also uncharged ("Exclude gate waiting"), so principals whose work
is mostly exclusive look cheap to min-service selection and are preferred —
a mild positive feedback into the gate queue.

The inversion the reservation exists to prevent is real: all pool executors
held by mid-builtin promotion waiters queued behind a known-exclusive head H
that cannot get an executor. But exactly one exclusive runs at a time, so
**one dedicated exclusive-start slot outside the optimistic pool** is
sufficient. Known-exclusive work takes a ticket at selection without a pool
slot; the head uses the dedicated slot. Physical concurrency becomes
workers + 1, where the +1 only ever runs under exclusive ownership.

### F3 (serious) — Maintenance handoff: internally contradictory, can stall the whole server, and is unsafe for in-task entrypoints

Pointers: design §Maintenance handoff; `engine/waif_lifecycle.go:220-255`;
`builtins/base_descriptors.go:249` (`run_gc`, `Effect: Irreversible`).

(a) **Contradiction.** The text orders: publish pending → *enqueue exclusive
maintenance ticket* → stop admissions → let active attempts drain → claim. It
then says both "no new requests barge ahead of maintenance" and "gate requests
needed to finish already-active attempts must remain runnable". An active
optimistic attempt has not yet issued its shared-commit or promotion ticket;
when it does, that ticket is *younger* than maintenance.

```text
t0  attempt A running (holds lease, no ticket yet).
t1  maintenance M publishes pending, takes ticket 20.
t2  A finishes executing, requests SharedCommit → ticket 21, behind M.
t3  M is head: claims, takes SweepMu→VMStartMu, quiescence fails (A's lease).
    "release barriers and gate and retry later" → M re-queues as ticket 22.
t4  A commits. M eventually succeeds — after a wasted exclusive turn that
    drained the shared cohort for nothing, and only because M gave up its
    FIFO position, which the text says cannot happen.
```

Not a deadlock, but the protocol as written cannot be implemented without
violating one of its own sentences. Fix: **drain-then-claim**. Maintenance
takes no ticket while anything holds a lease or a reservation; it waits for the
in-flight population to reach zero with admissions paused, *then* takes a
ticket (the queue is then empty except for checkpoints) and the barriers.

(b) **Global stall.** The admission pause lasts until the slowest active
attempt unwinds. A slice parked in a native builtin holds its lease
(design: "One active slice includes a slice parked inside a builtin").

```text
t0   task X enters a synchronous network/exec builtin; parks ~10 s, lease held.
t1   a waif batch becomes pending → maintenance_pending → admissions stop.
t1…t10  no new input, fork, or completion is admitted server-wide.
```

Today `flushDeferredGC` fails closed and returns
(`waif_lifecycle.go:250-255`): GC is delayed, the server is not. The design
converts a GC-latency problem into a whole-server latency problem. Required:
a drain budget. If the population is not zero within the budget, withdraw
`maintenance_pending`, reopen admissions, back off. Consequence to state
honestly: **"maintenance eventually receives a turn" (acceptance, step 3) is
not guaranteed** while a native builtin can park indefinitely. You can have
"no global stall" or "guaranteed maintenance turn", not both, until those
builtins are genuinely asynchronous.

(c) **In-task entrypoints.** "All independent sweep entrypoints must use this
handoff." `run_gc` is `Irreversible`, so its caller already crossed
`BeforeIrreversibleEffect` and holds the gate exclusively *and* a lease.

```text
t0  B: optimistic attempt, holds lease.
t1  A: calls run_gc() → already exclusive owner. Handoff says: wait for
    active attempts to drain.
t2  B finishes executing, requests SharedCommit → queued behind A.
    A waits for B's lease; B waits for A's gate.  Deadlock.
```

In-task sweep entrypoints must **borrow** the caller's capability and stay
**try-only** (today's fail-closed no-op). Only the lease-free post-slice flush
may use the draining handoff.

Side observation: sweep-run `:recycle` hooks execute with a context that has
no `StoreTxn` and write the live store directly (`waif_lifecycle.go:457-467`,
`gcRecycleContext` at `:201-211`). They are gate *non-participants* today, so
"maintenance may need to enter the gate from a sweep-owned hook" describes a
future state, not the baseline. Running the sweep under exclusive maintenance
ownership would be a real improvement (it would exclude checkpoint capture,
`db/store/store_snapshot.go:71`) — I did not verify whether checkpoint capture
and a sweep can overlap today; list it in the inventory.

### F4 (medium) — Ownership state machine: a missing transition, a kill window, and unspecified double release

Pointers: design §Ownership and cancellation state machine; §Commit admission
steps 2–4; `engine/task_runtime.go:120-124,244-252,283-285`.

1. *Missing transition.* Step 3: on failed claim "retain the slot reservation
   and ticket, and await a new notification". The diagram has no
   `offered → queued`. With FIFO, no barging, and a reserved executor, a claim
   can fail only through cancellation (the "resource availability" recheck has
   nothing left to check). Either delete the retain-and-rewait path, or add the
   transition with a named cause. I recommend deleting it: fewer states, and
   the lost-wakeup/generation machinery for re-subscription disappears.
2. *Stale claimant / kill window.* Between successful claim and
   `t.SetCancelFunc` (`task_runtime.go:283-284`) the task is neither queued nor
   cancellable-as-running.

   ```text
   t0  ticket offered; claimant registers provisional lease.
   t1  claim succeeds (state = claimed). Task still reads as queued to MOO
       ("must not mark the MOO task running until claim succeeds").
   t2  kill_task(id) sees a queued task → removes it, frees the saved VM.
   t3  claimant begins executing a killed task on a freed VM.
   ```

   Contract: kill/timeout linearizes against the *semaphore* state, not task
   state. `queued|offered` → cancelled (claim then fails). `claimed` → set
   `cancel_requested`; the owner must check it before the first instruction
   and again when installing the cancel func. The task-state flip to running
   happens inside the same critical section as the claim.
3. *Identity.* Leases are refcounted by task ID (`runtime.go:201,208-212`). A
   stale claimant for generation 1 and a fresh one for generation 2 of the same
   task are indistinguishable there. Harmless for the refcount, harmful if the
   failure path "releases both the reservation and ticket" by task ID.
   Reservation, provisional lease, and ticket must all be owned by the
   per-request capability.
4. *Release twice.* Today two paths release (`releaseEscalation` and the
   deferred backstop) and are safe only because one goroutine shares one
   `escalated` bool. "Release consumes it exactly once" must define the second
   call: a generation-checked no-op that logs at ERROR and bumps a counter.
   Never a panic (it runs in deferred unwinds) and never an unlock of the
   successor's grant — the classic double-`Unlock` failure.
5. *Failed-claim cleanup must re-trigger GC.* A sweep that failed closed
   because of a provisional lease relies on "that VM's lifecycle release will
   retry the flush" (`waif_lifecycle.go:250-251`). Removing a provisional lease
   must call the flush, outside the semaphore mutex.

Answers to Q4 directly: cancellation cannot release an active holder *as
specified*; a stale claimant **can** start unless item 2 is added; double
release is prevented only if item 4 is specified.

### F5 (medium) — Replay after a failed promotion must keep ownership

Pointer: design §Commit admission, last paragraph; `task_runtime.go:133,
198-226` (on validation loss the attempt reruns with `escalated` still true).

The design says validation failure "may replay … a restartable attempt" but
not whether the capability survives the replay. If it is released and
re-requested at the tail:

```text
loop: run prefix optimistically → wait full FIFO queue → validate → a commit
      landed during the wait → fail → release → replay …
```

Under steady writes this need never terminate; FIFO's finite-predecessor
argument does not apply because each round is a *new* request. Keep today's
behaviour: retain ownership across the replay. State the cost (the prefix
re-executes under global exclusion) and bound it to one replay.

### F6 (calibration) — The FIFO cohort rule is sound, but it is not what fixes starvation

Q1 answer. Post-ticket, new traffic cannot starve an older request:

```text
t0  S1,S2 hold shared.   t1  checkpoint C takes ticket 10.
t2… S3,S4,… take 11,12,… and queue behind C (an earlier exclusive intervenes).
t3  S1,S2 release → C admitted → C releases → S3… admitted as one cohort.
```

Assumptions actually needed:

- A1. Shared holds are finite: true today, `Commit` holds the gate only over
  validate+apply (`store_txn.go:2645-2652`), no MOO code inside.
- A2. Exclusive holds are finite: **not guaranteed.** Tick/second limits bound
  VM work; a parked synchronous builtin is bounded by nothing.
- A3. An offered head is claimed or cancelled in finite time: depends on
  `VMStartMu`, which a sweep holds across every hook (300k ticks each,
  `waif_lifecycle.go:472`).
- A4. No bypass: write-free commits return before the gate (harmless, no hold);
  direct transactions and sweep hooks do not participate at all.
- A5. Pre-ticket age is governed by the runnable policy, not by FIFO. "Oldest
  request" means oldest *ticket*; a background task starved of selection has no
  ticket to be old with.

Calibration the design should add: Go's `sync.RWMutex` already blocks new
readers once a writer waits and releases waiting readers after each writer —
it is already starvation-free for both classes, roughly phase-fair. "Foreground
flood against an old background gate waiter" is therefore not a *current* gate
failure; it is a pre-dispatch failure (stage 1). The semaphore's real value is
cancellable waits, reasons/ledgers, capabilities, and later lease-free waiting.
Sell it as that.

### F7 (minor) — Accounting ambiguities

- "Exclude gate waiting and external suspension": say explicitly that time
  parked inside a native builtin **is** charged (it holds slot and lease); only
  true MOO suspension is not.
- Mid-builtin promotion waiters hold a pool slot while uncharged. Fine, but
  count them in the in-flight cap and in a separate blocked-slot ledger.
- Fairness wording that is too strong: "all shared commits and checkpoints
  participate in one finite-predecessor gate order" (excludes exempt nested
  commits, write-free commits, direct paths, sweep hooks); "maintenance
  eventually receives a turn" (F3b). The same-principal guarantee is only:
  each nonempty class is *selected* infinitely often given finite slices — no
  latency statement follows.

## Q5 — honesty of the MVCC boundary, and missing integration contracts

The design is honest where it matters: it states that write-free `Commit`
skips gate and validation (`store_txn.go:2636`, confirmed), disclaims opacity
and whole-server serializability, names direct eval, anonymous objects and
shared WAIF payloads as existing behaviours, and keeps wider speculation
disabled until they have contracts. "Promotion validates all prior reads
before an external effect" is accurate (`store_txn.go:2471`, `validateReads`
runs before anything else). I found no claim that overstates preserved
behaviour as new safety, apart from the wording in F7.

Contracts missing before implementation:

1. **Capability carrier.** "Task-ID equality is not authorization" — then the
   capability must travel in `TaskContext`. Today exemption rides on
   `StoreTxn.gateExempt` and survives only through `CommitAndRenew`
   (`store_txn.go:2617-2622`). `callVerbWithArgstr` begins a fresh transaction
   (`call_verb.go:293`); I did not see exemption propagated in the lines read.
   Every site that constructs a new `TaskContext` (`call_verb.go:284`,
   `waif_lifecycle.go:201,457`) must be classified: borrows, non-participant,
   or independent.
2. **Deferred checkpoint.** `dump_database` under the gate is postponed to
   `runDeferredCheckpoint` after release, still inside the slice, still holding
   lease and slot (`task_runtime.go:244-252`). In the new model that is a
   promotion-like waiter that then runs hook tasks; it needs a row in the
   in-flight accounting and an explicit "never requested while owning" rule.
3. **Kill/timeout linearization** (F4.2) and `queued_tasks()` visibility of
   ticketed-but-unclaimed work.
4. **Budget semantics.** Gate waits extend the VM deadline today
   (`ExcludeExecutionWait`, `task_runtime.go:205`). Pre-start waits must not
   consume the budget either; say where the anchor is taken.
5. **Executor meaning before stage 4.** Connection goroutines are not pool
   workers; "reserved executor" is undefined for them until they are routed
   through admission.
6. **Shutdown** with queued, offered, and provisional-lease states.
7. **Sweep hooks as gate non-participants** (F3 side note) in the inventory.

## Q6 — researched design or invented algorithm?

Mostly the former. Stride-style accounting, task-fair RW admission, serial
irrevocability and admit-before-worker are correctly sourced and correctly
de-scoped; the refusal to import PPoPP'09 priorities or DSN'11 deadlines is
right. The invented part is stage 3 (reservation + provisional lease +
offer/claim + maintenance handoff). That is where every serious defect sits,
and it is the part with no supporting measurement. Smallest corrections:

1. Ship stage 1 alone and measure against the real gate
   (`TestMongooseRealWorkload` 1p and 16p, plus the CI-style conformance
   suite). It targets the measured 10 s.
2. Stage 2 keeps lease→gate order; describe it as ownership, cancellation and
   observability.
3. Stage 3 only on evidence that lease-holding gate waiters materially delay
   GC or occupy workers. If built: one dedicated exclusive-start slot;
   drain-then-claim maintenance with a drain budget; in-task sweeps borrow and
   stay try-only; no `offered → queued`.
4. Stage 4: input principal = player/connection, background = programmer;
   replace "one active slice per principal" with a parked-native cap plus a
   tunable slice cap; provisional charge at dispatch.

## Corrected contracts (replacement text)

**Principal.** Input work is charged to the connection's player (pre-auth: a
shared anonymous weight). Background work is charged to the programmer of the
current verb at fork/suspend. The ledger key never limits concurrency by
itself.

**Concurrency.** One in-flight input slice per connection. Per principal: at
most N in-flight background slices (tunable, default > 1) and at most M parked
native continuations. Dispatch charges a provisional advance; completion
reconciles. Parked native time is charged; gate-queue time is not.

**Exclusive start.** Known-exclusive work takes a ticket at selection holding
no pool executor and no lease. One dedicated exclusive-start executor serves
the head. Order: notification → `VMStartMu` + provisional lease (owned by the
capability) → claim and task-state flip in one semaphore critical section →
snapshot → run. A claim fails only if cancelled; failure removes the lease,
re-triggers the deferred-GC flush, and ends the request.

**States.** `queued → offered → claimed → released`; `queued|offered →
cancelled`; `claimed → claimed(cancel_requested) → released` by the owner only.
Kill and timeout act on semaphore state. Second `Release` is a logged no-op
checked by generation.

**Promotion.** Wait holding lease and Go stack, no store/slot mutex. After
claim: validate, then effect. On validation loss with the effect still ahead
and the attempt restartable: replay once **under the same capability**.

**Maintenance.** Lease-free post-slice flush only: publish pending → pause new
admissions → wait, within a drain budget, for leases and reservations to reach
zero → take an exclusive ticket → `SweepMu → VMStartMu` → verify quiescence →
sweep with hooks borrowing maintenance ownership → release in reverse. Budget
exceeded or not quiescent: withdraw, reopen admissions, back off. In-task
entrypoints (`run_gc`) borrow the caller's capability and never wait for drain.
No guarantee of an eventual maintenance turn is made while synchronous native
builtins can park.

**Progress statement (the only one to make).** A ticketed request is served
after finitely many predecessors *if* every predecessor's hold and claim
handoff terminates and the Go scheduler is fair. No elapsed-time bound, no
exactly-once external delivery, no whole-server serializability.

## First fair-admission implementation versus later work

*First implementation (stages 1–2, then corrected 4, then 3 if measured):*
selection, wakeups, ledgers, capability-based gate, whole-server admission.
Storage semantics unchanged; exclusive slices stay exclusive; no new
commit/yield boundary visible to MOO; conformance expectations untouched.

*Later, separately reviewed:* `AttemptSnapshot` rollback for resumed slices
(must cover WAIF payload identity, recycle-guard re-registration, forks and
pending effects as one unit), asynchronous builtins verified against Toast
suspension behaviour, and only then any protected-read / concurrent
irrevocability protocol — with the fixed-snapshot counterexample from the
contention research as its first adversarial history.

## Not verified in this pass

Whether checkpoint capture can overlap a sweep today; whether nested
standalone calls under an exclusive owner receive the gate exemption
(`call_verb.go:293` onward); `engine/eval.go` admission details;
`engine/runtime.go:465` beyond the scheduler `Plan`/`Run` code read. Go
`sync.RWMutex` fairness in F6 is from its documented behaviour, not from a
Barn experiment.
