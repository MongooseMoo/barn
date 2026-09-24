# Research: cooperative runtimes, MVCC, and admission for Barn

Date: 2026-09-18. Research-papers plugin research workflow. This report owns literature interpretation and design recommendations; it does not claim a new Barn runtime experiment or source audit. Recommendations below are adaptations, not performance results or inherited correctness proofs.

The final integration choice and review dispositions are in the
[transaction scheduling design](../plans/barn-transaction-scheduling-design.md).
That document distinguishes mechanisms worth borrowing from later resource
admission experiments that are not required for the initial implementation.

## Summary

Cooperative multitasking with transactions is well explored. CoroBase supplies a direct coroutine/MVCC implementation; Orleans supplies a mature cooperative actor runtime with transaction scheduling; Calvin demonstrates admission before consuming an execution worker. None supplies a drop-in protocol for arbitrary MOO code with irreversible effects and nonrestartable continuations. The practical foundation is an explicit transaction lifecycle, resource-eligible cooperative dispatch, established proportional-share accounting, and a separate irrevocability protocol. These components should share task identity and wakeups, rather than collapsing every wait and every resource into one opaque queue.

## Approaches found and their transfer limits

### 1. CoroBase: direct coroutine-to-transaction MVCC precedent

**Primary paper:** He, Lu, Wang, *CoroBase: Coroutine-Oriented Main-Memory Database Engine*, PVLDB 14(3), 2021. [Full paper HTML](https://arxiv.org/html/2010.15981), especially §§4.2–4.6 and §5.9.

The design interleaves transaction coroutines over ERMIA's shared-everything multiversion engine. Snapshot isolation remains usable; serializability adds SSN dependency checking and possible aborts. Execution scheduling and isolation are distinct responsibilities.

The paper's batch scheduler does not replace finished transactions until the batch drains (§4.2). Its authors acknowledge that mixed long/short transactions may need priority scheduling. §5.9 measures increased transaction latency with larger batches; those measurements exclude durable-log I/O.

§4.4 identifies a concrete hazard: one coroutine's epoch exit can permit reclamation while another suspended coroutine still needs the memory. CoroBase therefore enters/exits epochs around whole batches and makes formerly thread-local transaction resources distinct per transaction. §4.5 avoids suspension on latch-holding structural-update paths.

**Barn inference:** adopt transaction-owned state and explicit suspension/root-lifetime rules. Reject fixed-batch admission as the responsiveness default. Its cache-prefetch throughput results do not establish an appropriate Mongoose policy or solve irreversible effects.

**Complexity:** medium for lifecycle principles; high and unjustified to import its cache-prefetch machinery.

### 2. Orleans: cooperative execution plus explicit transaction lifecycle

**Primary paper:** Eldeeb et al., *Cloud Actor-Oriented Database Transactions in Orleans*, PVLDB 17(12):3720–3730, 2024. [Author-hosted full PDF](https://www.cs.columbia.edu/~junfeng/papers/orleans-vldb24.pdf), §§4.2, 5.1–5.2, 6–7, printed pp.3723–3726.

Orleans uses grain-level 2PL with a pipelined distributed commit protocol. Early lock release allows subsequent transactions to proceed, but records commit dependencies and permits cascading aborts. Crucially, §4.2 requires transactional methods to avoid effects outside transactional grain state. That is programmer discipline, not automatic effect rollback. The experimental reconnaissance phase (§7) preloads actors without holding execution locks; actual execution rechecks the necessary state.

**Barn inference:** a useful example of separating execution, waiting, and commit lifecycle. It does not justify releasing Barn's protection after an arbitrary external effect. Importing speculative commit dependencies would add complexity while retaining the need to handle irrevocability.

**Complexity:** medium to reuse lifecycle patterns; high to adopt distributed 2PC and cascading aborts, which are not warranted here.

The [official scheduler documentation](https://learn.microsoft.com/en-us/dotnet/orleans/implementation/scheduler) distinguishes request scheduling from synchronous task turns. In [WorkItemGroup.Execute](https://raw.githubusercontent.com/dotnet/orleans/main/src/Orleans.Runtime/Scheduler/WorkItemGroup.cs), lines 126–200 in the retrieved source, the activation scheduling quantum is checked after `RunTaskFromWorkItemGroup` returns. Remaining work is rescheduled. This is cooperative fairness between turns, not a mechanism for interrupting an arbitrarily long synchronous turn. The implementation separately warns about long turns.

The [transaction queue implementation](https://raw.githubusercontent.com/dotnet/orleans/main/src/Orleans.Transactions/State/TransactionQueue.cs) uses a commit queue, asynchronous storage work, promises, and notifications. `NotifyOfPrepared` updates readiness and wakes storage work; `NotifyOfConfirm` awaits a completion promise. These are useful ownership patterns, not a promise that replacing a mutex with a channel removes contention. Source URLs track `main`; this report is a retrieval-time reading, not a pinned vendor dependency.

### 3. Calvin: admit resource-ready transactions before workers

**Primary paper:** Thomson et al., *Calvin: Fast Distributed Transactions for Partitioned Database Systems*, SIGMOD 2012, pp.1–12. [Full PDF](https://dsf.berkeley.edu/cs286/papers/calvin-sigmod2012.pdf), §3.2, PDF pp.5–6; §3.2.1 for dependent transactions.

Calvin establishes transaction order before execution. It requests complete read/write lock sets in that order, then gives a transaction to a worker after acquiring its locks. Dependent transactions require a reconnaissance query; predicted access sets must be rechecked and may require deterministic restart. The approach also relies on deterministic execution/replay.

**Barn inference:** borrow the admission boundary: a known resource wait should not occupy a scarce execution worker. Do not adopt global input/commit order or require all MOO read/write sets in advance. Dynamic object access and external inputs make those assumptions a poor match. A slow globally ordered transaction can become a barrier for its dependents; imposing extra global order would create unnecessary barriers for unrelated work.

**Complexity:** low for the admission principle, very high for a Calvin-style architecture conversion.

### 4. Dynamic stride: explicit shares for cooperative service

**Primary paper:** Waldspurger and Weihl, *Stride Scheduling: Deterministic Proportional-Share Resource Management*, MIT/LCS/TM-528, 1995. [Full PDF](https://people.eecs.berkeley.edu/~prabal/teaching/resources/eecs582/waldspurger95stride.pdf), §§2.1–2.4, printed pp.2–6; §4 for hierarchy.

Select the minimum-pass eligible client; advance its pass in proportion to service consumed and inversely to its weight. The paper explicitly handles nonuniform quanta, including nonpreemptive overrun (§2.4). Joining/leaving preserves the remaining distance from global virtual progress (§2.2), rather than accumulating unlimited entitlement while asleep. Hierarchy gives groups a shared allocation.

**Barn inference:** this is a stronger foundation than a bespoke foreground-burst-plus-aging heuristic when configurable shares are the objective. Charge actual resource occupancy and retain accounting across resumption and retry. A cooperative overrun still blocks competitors until the holder relinquishes that resource. Published equal-quantum bounds cannot be quoted as Barn latency bounds.

**Complexity:** modest for one serial resource; substantially higher for concurrent workers, multi-resource admission, and hierarchical policy. Begin with the actual bottleneck, not a general cluster scheduler.

## Existing implementations and license evidence

| Implementation | Useful reference | License and limits |
|---|---|---|
| [dotnet/orleans](https://github.com/dotnet/orleans) | C# cooperative turns, transaction queues, asynchronous storage and completion ownership | [MIT license](https://raw.githubusercontent.com/dotnet/orleans/main/LICENSE). Borrow ideas or attributed compatible code only after normal review; not a Go component. |
| [sfu-dis/corobase](https://github.com/sfu-dis/corobase) | C++20 transaction coroutines and MVCC resource management | [MIT license](https://raw.githubusercontent.com/sfu-dis/corobase/master/LICENSE). Repository README says the supplied `ermia_SI` executable is snapshot isolation, not serializable; paper discusses SSN separately. |
| [yaledb/calvin](https://github.com/yaledb/calvin) | Research implementation of deterministic transaction scheduling | Repository describes code associated with the 2014 evaluation. A root LICENSE lookup returned 404 and repository page did not establish a license; reuse rights are unverified. Treat as a reading reference. |
| Stride paper algorithms | Small scheduling algorithm, dynamic participation and service accounting | Paper includes pseudocode/C-style descriptions. No separately inspected maintained reusable Go package; implement the documented algorithm rather than vendor an unknown port. |

## Answers for the Barn design

### What is the correct unit to schedule?

An eligible transaction continuation, grouped under its intended fairness principal. A logical task, transaction attempt, continuation, and worker are different objects. Maintain those identities explicitly. A retry is still service consumed by the same principal; changing task IDs must not erase debt.

### Where does the commit gate fit?

It is a scheduled resource with a safety protocol, not merely another FIFO mutex. The irreversibility protocol determines eligibility and who may overlap; policy chooses among eligible contenders. Ordinary shared commits, exclusive nonretryable execution, and checkpoint access must all participate in the same admission contract. This report does not choose the exact STM irrevocability protocol; that requires the companion research and Barn's store invariants.

### Can one weighted score schedule everything?

Not responsibly without defining what is being shared. CPU execution time and exclusive gate occupancy are different resources. A slow external operation can consume little CPU while excluding all writers. Record both. Start with separate service ledgers and a shared principal identity. A combined scalar or dominant-resource policy needs an explicit objective and validation; it is not entailed by stride or MVCC.

### What does it mean for waiting work not to consume a worker?

Known pre-execution waits belong in an admission queue and become runnable when prerequisites change. Claim ownership only when an executor can actually begin, with cancellation-safe handoff. An already executing builtin may need a parked goroutine temporarily, but parking preserves any held resources and GC roots; it is not equivalent to a fully materialized continuation. This is an implementation distinction the design must expose.

### Does async I/O remove gate starvation?

Only if the chosen semantics permit releasing the relevant transaction protection across the wait. Moving a call to another goroutine frees an OS execution context, not the gate. The design must specify what commits before launch, how exactly one result is delivered, whether resumed work is a fresh transaction, and what happens when cancellation races with an irreversible outcome. Orleans' rollback assumptions cannot answer those MOO questions.

### How should foreground/background priority work?

Use explicit nonzero shares for competing groups rather than unlimited strict priority. Within each fairness principal, use stable FIFO initially. If foreground and background of one principal must each make progress, give them explicit child queues/shares; owner-level fairness alone cannot guarantee that. Completed asynchronous work becomes eligible immediately and competes at the next cooperative boundary. Do not wait for a stale admitted batch to drain. Maintenance also needs an explicit service class and eligibility rules.

### Does weighted service guarantee prompt response?

No hard bound follows while a single turn, gate holder, or noncancellable external call can run arbitrarily long. The defensible claim is proportional service under continuing eligibility and finite releases. Measure longest hold time, not only average service. Admission caps and bounded queues control memory and overload; they do not shorten a running irrevocable section.

## Recommended implementation sequence and effort

1. **Minimal, medium effort:** define lifecycle states, resource eligibility, completion wakeups, cancellation ownership, and two independent service measurements. Implement service-aware selection at existing safe boundaries with conservative protection unchanged. Simulate irregular slice lengths, sleeping clients, retries, and mixed demand before tuning weights.
2. **Responsiveness, medium-to-high effort:** materialize safe waiting continuations, make eligible completions visible without batch barriers, and narrow external-I/O exclusion only at semantics-approved boundaries. Reclamation ownership is part of this work, not a later optimization.
3. **Concurrency, high effort:** reduce irrevocability through restartable continuation checkpoints or conflict-aware protection. This requires correctness arguments for dynamic reads, published prefixes, cancellation, GC, and effects. Do not advertise it as a mutex refactor.

The beautiful design here is small: explicit task/transaction state, explicit resource ownership, one accountable fairness identity, and independently testable policy. A new database architecture, speculative dependency graph, or universal adaptive priority formula is unnecessary until measurements show that the simpler system is insufficient.

## Remaining project questions

- Which MOO boundaries permit committing and releasing protection while preserving the verified oracle semantics?
- Which VM and builtin state can be safely rooted and suspended without an execution lease?
- Which continuations can be reconstructed for conflict retry without repeating external effects?
- What are the intended foreground/background shares within one programmer, and how is identity preserved across calls?
- What are the actual distributions of runnable delay, gate wait, gate hold, active execution, and external wait on the current workload?

These are source/semantic and workload questions. The literature establishes the mechanisms and failure modes; it does not supply missing Barn measurements.
