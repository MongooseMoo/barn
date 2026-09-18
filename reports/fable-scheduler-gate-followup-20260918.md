# Gate-aware scheduling, round two: traps, retractions, one first slice

Reviewer: Claude Fable. 2026-09-18. Barn `ef5fcb0`. Source reading only; nothing
executed. Builds on `reports/fable-scheduler-gate-20260918.md` (R1; its facts are
cited as F-numbers). Tags: **[FACT]** read at the cited line, **[INFERENCE]**,
**[PROPOSAL]**.

## Retractions from R1

Three R1 recommendations do not survive the traps below. I withdraw:

1. **Boundary abort-and-rerun as the default (R1 §5, I2).** Under contention it
   turns a *tail* hold into a *whole-slice* hold and wastes the prefix: busier
   gate → more aborts → longer holds. It amplifies the convoy it targeted.
2. **The dispatcher holding a grant until a slot frees (R1 §6.3)** — a gate held
   by a non-running task, which R1's own I3 forbids.
3. **"The shared path never consults the arbiter" (R1 I5)** — right for
   ordering, wrong as a migration boundary (§4).

## 1. Two priority orders

**[PROPOSAL] One rule removes the second order: only an entity that already owns
its execution capacity may request the gate, and the task is bound at grant
time, not at request time.**

Every real exclusive requester already owns a goroutine: a connection lane
(F14), the checkpoint goroutine, and — because production scheduling is serial
(F13) — the single scheduler lane. So:

- **Dispatch decision:** owned by whoever owns the goroutine. The scheduler lane
  picks its next task; a connection lane has exactly one.
- **Exclusive grant:** owned by the arbiter, ordering *lanes*, not tasks. The
  scheduler lane's request carries the priority of its best current G1
  candidate, recomputed as candidates arrive or die; on grant it binds whichever
  is best *then*. A killed candidate simply leaves the queue.
- A grant can never go to something that cannot run, because a lane requests
  only while idle.

**Does the holder always retain a way to finish?** It needs its goroutine (has
it), a lease, and the store locks. No MOO primitive makes a holder wait on
another task: `suspend`/`read`/`exec` end the slice and release at
`task_runtime.go:495`; the uncaught-error and timeout hooks run after release
(`:646`, `:869`). One real trap **[FACT]**: the standalone nested call path opens
its own transaction and commits through the ordinary shared gate
(`engine/call_verb.go:293,348`). Entered by a goroutine that holds the gate
exclusively, that `RLock` self-deadlocks. Today it is avoided by construction —
a holder's non-direct transaction routes to `CallVerbInContext`
(`engine/runtime.go:104-106`) and `dump_database` is deferred (F19). The arbiter
must make it a rule: **an inherited lease (`call_verb.go:237-239`) inherits the
grant**, i.e. the nested transaction is marked gate-exempt when the arbiter's
holder is `ownerTaskID`.

Schedule (S = scheduler lane, L1/L2 = connection lanes, C = a shared committer):

| t | Event | Arbiter | Gate |
|---|---|---|---|
| 0 | S idle, best candidate X (bg, resumed). Requests. | grants S; S binds X | S drains shared, holds |
| 1 | L1 resumes a `read()` (G1): requests fg, **before** taking a lease | queue: L1 | S |
| 2 | C (fresh command on L2) finishes executing, enters `Shared()` | — | C blocked, counted as fg shared waiter |
| 3 | Checkpoint requests (maintenance) | queue: L1, ckpt | S |
| 4 | X suspends; S releases | C admitted first (RWMutex releases blocked readers); grants L1 | L1 waits only for C's short commit |
| 5 | S idle again, requests for Y (bg) | queue: ckpt, S | L1 |
| 6 | L1 done | ckpt aged past bound → granted, else S | — |

While the checkpoint is queued *in the arbiter* it is not inside `Lock()`, so it
exerts no writer preference and shared commits keep flowing — better than today
(R1 T7).

## 2. Early exclusive admission for resumed slices

**Keep it. It is the right first implementation.** Removing it is unsafe:

Resumed slice S runs ungated on snapshot ts₁ and reads `x = 1` into a local.
C commits `x = 2`.

- **Future write:** S writes `y = x + 1`. Commit validation fails on `x`. S cannot
  re-run: `canRetry` is false for a yielded VM (F5) and its pre-suspend half was
  already committed at the suspend (`task_runtime.go:526-544`). Result: the
  issue-#296 phantom `E_INVARG`, pending output discarded (`:485-486`).
- **Irreversible effect:** the hook takes the gate and validates; validation
  fails; `canRerun` is false (`:209`), so the hook returns "proceed" (`:225`). The
  effect runs on stale reads, then the final commit fails. Effect done, writes
  lost — torn.
- **Read-only slice:** safe only in hindsight; unknowable before the first opcode.

No retry snapshot exists, so there is nothing to fall back to. The defect today
is not *that* resumed slices are admitted exclusively; it is that admission is
unordered, invisible, uncancellable, and taken **after** the lease (F16). Fix
those. Lazy acquisition waits for a continuation-snapshot design (phase 4).

## 3. Parking at the effect boundary

Literal parking-by-suspension is not available. **[FACT]** Operands are already
consumed — `args` aliases the VM stack above `SP` (`vm/op_misc.go:61-64`) or is a
Go-heap copy in splice mode (`:52-55`) — and `FlowSuspend` pushes a return value
and resumes *after* the call (`:115-119`). A suspended-at-boundary continuation
would need a new "re-execute this call" resume mode with re-materialised
operands: representation work.

**Smallest actually-safe option: park the goroutine, retain the lease.**

- *Continuation state:* the Go stack. Nothing to represent.
- *Exactly-once:* the wrapper runs the hook before the body
  (`builtins/registry.go:111-116`); the effect has not happened.
- *Revalidation:* unchanged — `CommitAndRenewCarryingReads` validates on grant
  (F8); a loss re-runs the attempt.
- *GC roots:* the consumed operands are invisible to a VM walker, so the VM is
  **not** a complete root set while parked. The lease must be retained so GC
  fails closed (F17). Releasing it here would be a use-after-collect.

**Does it prevent GC indefinitely?** **[INFERENCE]** It can — but so can today's
system: `flushDeferredGC` does not wait for quiescence, it returns if any lease
exists (`waif_lifecycle.go:250-255`), so overlapping lanes alone can starve it.
Parking widens lease intervals from *run* to *wait + run*. It is a finalization
*liveness* cost, never a safety one. Defensible as a bounded intermediate because
(a) only boundary waits hold a lease — G1 waits move before the lease and hold
nothing; (b) each wait is bounded by the policy queue; (c) it is now measurable.
Slice 1 must export pending-batch length and age so the cost is visible.

Required companion fix **[FACT]**: the re-run path (`task_runtime.go:409-430`) has
no kill check before `goto retryAttempt`; a task cancelled while parked would be
re-executed.

## 4. Migration boundary

R1's "exclusive-only arbiter" invites a half-migration: if `SnapshotWithRoots`
still called `commitGate.Lock()` directly, the arbiter would believe the gate
free, grant it, and the grantee would park inside `Lock()` behind the checkpoint.

**[PROPOSAL]** A single `store.Gate` type owns `commitGate`. `EscalationLock` and
`EscalationUnlock` are **deleted**, so an unmigrated caller fails to compile.
Every access in one change:

| Access | Site | Becomes |
|---|---|---|
| G1 slice start | `task_runtime.go:134` | `Gate.Exclusive`, before the lease |
| boundary | `:203` | `Gate.Exclusive`, lease retained |
| checkpoint walk | `store_snapshot.go:71` | `Gate.Exclusive(maintenance)` |
| ordinary commit | `store_txn.go:2650` | `Gate.Shared`: same `RLock`, plus class and wait recorded |
| nested call under a holder | `call_verb.go:348` | exempt via inherited grant (§1) |
| test helpers | `*_test.go` | `Gate` API |

Outside the gate by design and **not** claimed as governed: direct live-store
writes (`store_core.go:277-279`) and sweep recycle hooks (F18), which are
excluded by lease quiescence instead.

The only fairness claim this licenses: *exclusive grants are policy-ordered;
blocked shared committers are admitted between every two exclusive holds, so
shared wait ≤ one hold + drain.* Shared committers are tracked, not ordered — they
run concurrently, so there is nothing to order. The residual: the arbiter cannot
see *future* shared demand, so a foreground commit arriving just after a
background grant still waits one hold.

## 5. A holds the gate in `curl`; B has a 50 ms soft target

Donation cannot help: A is blocked in network I/O, not short of CPU, and Go has
no goroutine priority to donate. Weights only order the queue *behind* A. B's wait
is at least A's residual, which is a timeout the MOO programmer chose
(`builtins/curl.go:29-36`). The target is unmeetable by policy.

**What must change:** `curl` must end the slice, as `exec` already does
(`builtins/system.go:258-284`): cross the boundary, start the request on a helper
goroutine, return `Suspend`; the slice commits and releases at `:495`; the
completion re-queues the task, which resumes as G1. This is Toast's existing
semantics — `bf_curl` returns `background_thread(...)` (`src/curl.cc:91`) — so
`curl` becomes a suspension point in Barn as it already is in Toast, with the
usual consequences (other tasks interleave; resumed budgets are background,
`task_runtime.go:254-264`). Conformance-first: a Toast-passing test must exist
before the Barn change, and `set_thread_mode(0)` behaviour must be checked
against the oracle. The same applies to `open_network_connection`
(`builtins/connection.go:356`). **I do not claim this contributed to the observed
Mongoose delay; no trace shows `curl` on that path.**

**Plain long computation after an effect** (`server_log(...)` then seconds of
bytecode): async I/O is irrelevant, releasing mid-hold reopens #296, and nothing
preempts. B waits up to A's remaining seconds budget. What the design can offer:
B is *next*; B is unaffected if its slice is write-free (F3); the hold is traced
and attributed; operators can lower background limits (an existing MOO-visible
setting). A 50 ms soft target is a *report* here, not a promise.

## 6. Two retryabilities

**[FACT]** Scheduling retryability (`taskIsConflictRetryable`,
`task_runtime.go:742-744`) puts first-run forks in solo batches; runtime
retryability (`captureTaskRetryState`, `:788-791`) lets them retry internally and
cross the boundary un-escalated. The comment at `:739-741` says the split is
deliberate.

Production concurrency today: **1 scheduler task ∥ N connection lanes ∥ the
checkpoint goroutine ∥ ungated helper goroutines.** "All workers blocked on the
gate" is hypothetical — it needs `Plan` to co-schedule forks. Slice 1 need not
solve pool exhaustion; it must not introduce it. Parking the scheduler lane at a
boundary stalls only background work and cannot deadlock, since no holder waits
on the scheduler (§1).

## 7. Cancellation and quotas

| Moment | Behaviour |
|---|---|
| waiting, G1 (no lease yet) | remove request; kill and unregister the task (it was `ResumeAndClaim`ed, `task_factory.go:402`) |
| waiting, boundary (lease held) | hook returns abort; VM unwinds via `FlowAbortAttempt`; lease released; **kill check prevents re-run** |
| grant races cancel | `Cancel` observes granted → immediate `Release` |
| slice started | not interruptible (F20); the hold runs out. Residual limit |
| nested call, inherited lease | inherits the grant; never requests |
| checkpoint behind a holder | queued in the arbiter, holding nothing, no writer preference; ages to the front |
| GC behind a holder | not an arbiter client in slice 1; waits for lease quiescence |

| Quantity | MOO ticks/seconds | Scheduler ledger |
|---|---|---|
| queue, gate, lease wait | no (as today, F12) | no — feeds aging only |
| speculative retries | no (budget restored per attempt, `:814-828`) | **yes** |
| execution occupancy | yes | yes |
| gate occupancy | — | yes, in addition |

Donation changes *order only*. A task promoted because a foreground waiter is
stuck behind it is billed to its own principal at full rate, and its class
reverts at release. No free future service.

## Decision table

| Item | Safe now | Needs representation work | Changes MOO semantics |
|---|---|---|---|
| `Gate` type, all accesses migrated, traced | ✔ | | |
| Policy-ordered exclusive grants, lane-owned, late-bound | ✔ | | |
| G1 acquisition before the lease | ✔ | | |
| Cancellable waits + kill check before re-run | ✔ | | |
| Inherited lease ⇒ inherited grant | ✔ | | |
| Reconsider after each task; wake on `CompleteExec` | ✔ | | |
| Checkpoint as aged maintenance request | ✔ | | |
| Boundary park by goroutine, lease retained | ✔ (GC liveness cost) | | |
| Boundary park by suspension | | ✔ re-execute-call resume | |
| Ungated or lazy resumed slices | | ✔ continuation snapshot | |
| Parallel fork dispatch + in-flight caps | | ✔ needs the above | |
| `curl` / `open_network_connection` async | | | ✔ matches Toast; conformance-first |
| Releasing the gate mid-hold; VM preemption | | | ✔ rejected |

## Recommended first slice: "the gate becomes a scheduled resource"

Keep the **serialized scheduler lane**. The resource model supports it: G1 tasks
are serialized by the gate regardless, and production is already serial.
Throughput sacrificed relative to *today*: none. Relative to a hypothetical:
parallel first-run forks, which cannot be had safely before phase 4. A *fully*
serialized server (lanes too) is not recommended; the existing tuning note
(`task_runtime.go:38-43`: early escalation cost −45% goodput at 16p) is the
nearest evidence, and I did not reproduce it.

**Owned components**
- `db/store`: `Gate` (arbiter + instrumented shared path); delete
  `EscalationLock/Unlock`; migrate `store_snapshot.go`.
- `engine/task_runtime.go`, `engine/call_verb.go`: G1 before lease; cancellable
  boundary wait; kill check; inherited grant.
- `engine/runtime.go`, `engine/internal/scheduler`: lane loop that reconsiders
  after every task and late-binds its gate request; completion wake-up.
- `server`: checkpoint as a maintenance request; lane wait cancelled on disconnect.
- Policy in this slice: class order (foreground lane > interactive-lineage
  continuation > background > maintenance-by-age), FIFO within class, one bounded
  `foreground_burst`. No per-principal ledger yet.

**Exit criteria**
1. No raw exclusive access compiles anywhere.
2. Deterministic schedules R1-S1, S2, S3, S7, S8 pass, plus: cancel-while-parked
   is not re-run; nested call under a holder does not deadlock.
3. `go test -race` clean for `engine` and `db/store`.
4. CI-style conformance suite unchanged; `TestMongooseRealWorkload` 1p and 16p
   within run-to-run noise.
5. Traces decompose one login into queue / lease / gate (by mode) / shared-commit
   / execution / external. **This decides whether phase 2 or 3 comes next.**

**Subsequent phases (at most three)**
2. Async `curl` and `open_network_connection`, conformance-first.
3. Per-principal round-robin with a service ledger and an interactive-lineage
   budget (R1 §6.4).
4. Continuation snapshot for resumed slices; only then ungated resumed slices,
   parallel dispatch, and in-flight caps.

## Disagreements with the parent proposal

- "Bounded admission + per-principal weighted service" is **not** the first
  deliverable. Until the gate is ordered and visible, those policies govern a
  queue the contended tasks are not in.
- The per-principal in-flight cap is inert before phase 4.
- Weighted service is a Barn enhancement; Toast's cross-queue behaviour is
  round-robin at sub-second granularity (R1 F22). Phase 3 should start there.
- A "50 ms latency objective" should be a reported attribution, not a knob, while
  any hold can last a full seconds budget.

## Residual limits

One non-preemptible hold always stands between a waiter and the gate; N
simultaneous logins still serialize their `read()` continuations; boundary waits
still pin GC; the arbiter cannot see future shared demand. None of this is
measured — slice 1's traces are what make the next decision evidence-based.
