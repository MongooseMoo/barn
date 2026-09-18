# Research: Barn transaction contention, irrevocability, and scheduling

Date: 2026-09-18. Research/design evidence, not an implemented or benchmarked change.

The final implementation choice is in the
[transaction scheduling design](../plans/barn-transaction-scheduling-design.md).
It retains lease-before-gate acquisition initially; the pre-execution resource
admission recommendation below is a later option, not a required first step.

## Summary

The closest established foundation is transactional contention management with explicit irrevocable execution. A CPU run queue alone cannot control who wins conflicting commits, and a fair commit queue alone cannot control when a continuation reaches it. The recommended Barn design combines fair cooperative admission with a conservative writer-drain protocol, makes replayability explicit, and reduces the duration of irreversible execution through legitimate asynchronous boundaries. Fine-grained protected execution is a possible later store feature, but is not a safe mechanical substitution for the current gate.

The first three papers below were read as complete extracted PDF text, including their evaluation and limitations; graphs were not independently digitized. Cicada and BOHM received targeted architecture/algorithm reading. Source inspection was pinned to the revisions recorded below. No Barn runtime performance result is claimed here.

## Approaches found and primary evidence

### 1. Irrevocability mechanisms, including writer drain and protected reads

**Source:** Michael F. Spear, Michael Silverman, Luke Dalessandro, Maged M. Michael, Michael L. Scott, *Implementing and Exploiting Inevitability in Software Transactional Memory*, ICPP 2008, pp. 59–66. [Author PDF](https://www.cs.rochester.edu/u/scott/papers/2008_icpp_inevitability.pdf).

**Algorithm:** Section II (PDF pp. 2–4) compares global exclusion, global writer exclusion, quiescence, writer drain, individual inevitable read locks, and a Bloom filter of inevitable reads. Writer drain reserves admission, drains existing writers, then excludes new writers. Protected reads permit disjoint writers but add synchronization and conflict checks. Section II.G requires acquiring the token, protecting previous reads, acquiring required writes, and validating before irrevocability becomes effective.

**Assumptions/limits:** Section I permits one inevitable transaction without advance access-set knowledge. Irrevocable execution cannot request transaction retry. Section V (PDF p. 8) makes the choice workload-dependent; opaque uninstrumented writes require stronger exclusion. The paper's transactional-memory protocols do not automatically specify a fixed-snapshot MVCC implementation.

**Complexity:** Writer drain is moderate; fine-grained protection is high because every relevant access and commit must participate. The comparison supports retaining a coarse mode while measuring whether reduced exclusion justifies additional bookkeeping.

### 2. Priority-aware conflict resolution, distinct from queue priority

**Source:** Michael F. Spear, Luke Dalessandro, Virendra J. Marathe, Michael L. Scott, *A Comprehensive Strategy for Contention Management in Software Transactional Memory*, PPoPP 2009, pp. 141–150. [Author PDF](https://www.cs.rochester.edu/u/scott/papers/2009_PPoPP_CM.pdf).

**Algorithm:** Sections 2–4 combine commit-time write acquisition, extendable timestamps, and selectively visible reads. Before committing, a writer checks whether it would invalidate higher-priority readers; conflicting lower-priority writers abort. The priority interface includes begin, abort, pre-commit, commit, and condition-retry hooks (Listing 1, PDF p. 7). A unique maximum priority represents inevitability. Consecutive aborts raise priority; successful completion resets it.

**Important qualification:** Section 3.1 gives no equal-priority fairness guarantee. Section 3.4 (PDF p. 6) explicitly says the combination of Karma, priority, and exponential backoff does **not** establish provable starvation or livelock freedom. The abstract must not be used to claim otherwise. Section 4.2 permits weaker memory ordering only below inevitable priority.

**Complexity:** High if adopted as a store mechanism. Useful architectural separation: execution policy selects importance; transaction machinery enforces compatible conflict decisions. A priority number in the scheduler is insufficient by itself.

### 3. Adaptive deadline-aware STM

**Source:** Walther Maldonado, Patrick Marlier, Pascal Felber, Julia Lawall, Gilles Muller, Etienne Riviere, *Deadline-Aware Scheduling for Software Transactional Memory*, DSN 2011. [Author PDF](https://who.paris.inria.fr/Gilles.Muller/papers/dsn2011.pdf).

**Algorithm:** Sections II–IV (PDF pp. 2–7) measure successful transaction durations, choose a quantile estimate `L`, and escalate on restart from optimistic execution to visible reads to irrevocability as the deadline approaches. The design sets the irrevocable threshold at `deadline - 4L`, and visible-read threshold another `2L` earlier. Its contention manager favors deadline-associated visible readers. Linux scheduler extensions reduce preemption/migration interference.

**Assumptions/limits:** The workload has reasonably stable transaction durations, rare irrevocability, and a modeled wait of at most one competing irrevocable execution of duration `L`. Contracts are percentile targets, not universal deadlines. The evaluation reports missed deadlines. Measurements exclude aborted, migrated, or preempted attempts. Arbitrary gate backlog and unpredictable network waits violate the useful timing assumptions.

**Complexity:** High for the full protocol; moderate for borrowing measured escalation decisions after reliable replay exists. Do not transplant its threshold constants into Barn or describe them as bounds on Mongoose login latency.

### 4. MVCC contention regulation: Cicada

**Source:** Hyeontaek Lim, Michael Kaminsky, David G. Andersen, *Cicada: Dependably Fast Multi-Core In-Memory Transactions*, SIGMOD 2017, pp. 21–35. [CMU PDF](https://15721.courses.cs.cmu.edu/spring2020/papers/04-mvcc2/lim-sigmod2017.pdf).

**Algorithm:** Sections 3.1–3.5 (PDF pp. 4–7) describe optimistic multiversion execution, timestamp-ordered validation, and early conflict detection. Section 3.9 (PDF p. 8) regulates retries using a shared maximum randomized backoff, adjusted by hill climbing on committed throughput. The paper's implementation samples throughput every 5 ms and changes maximum backoff by 0.5 microseconds.

**Assumptions/limits:** This optimizes throughput; it is not a per-programmer fairness or irrevocable-I/O protocol. Section 3.1 explicitly distinguishes serializability from external consistency and describes extra coordination for the latter. Section 3.8 makes reclamation/quiescence part of the design, rather than incidental cleanup.

**Complexity:** A database redesign if copied wholesale. The useful Barn lesson is to regulate speculative load using committed progress, while keeping responsiveness/fairness as separate objectives. Throughput-only tuning can select unacceptable interactive latency.

### 5. Deterministic MVCC: BOHM

**Source:** Jose M. Faleiro and Daniel J. Abadi, *Rethinking Serializable Multiversion Concurrency Control*, PVLDB 8(11), 2015, pp. 1190–1201. [Publisher PDF](https://www.vldb.org/pvldb/vol8/p1190-faleiro.pdf).

**Algorithm:** Sections 1 and 3 describe preordering transactions and preparing their versions before execution. Reads need not block writers or maintain read bookkeeping; execution follows the selected serialization order.

**Assumptions/limits:** The introduction (printed pp. 1190–1191) requires complete submitted transactions and advance write-set knowledge, obtained by declaration or speculative prediction with retry. Arbitrary interactive transactions are unsupported. Section 3 likewise depends on that knowledge for concurrency control.

**Complexity:** Very high for dynamic MOO verbs and effectful continuations. BOHM is evidence that the MVCC design space is broader than OCC plus a mutex, but its assumptions make it a poor default replacement for Barn. Consider it only for explicitly declared internal jobs with stable access sets, not as an excuse to infer arbitrary MOO effects.

## Existing implementations and provenance

### TinySTM: inspectable irrevocability, not a ready-made Go scheduler

Official repository: [patrickmarlier/tinystm](https://github.com/patrickmarlier/tinystm), inspected revision `02c10ccb26ef68845dc97e4a4e7fc81be02eed42`.

- [include/stm.h:684](https://github.com/patrickmarlier/tinystm/blob/02c10ccb26ef68845dc97e4a4e7fc81be02eed42/include/stm.h#L684): serial and parallel irrevocable API; unsuccessful promotion can abort to the saved transaction start.
- [src/stm.c:924](https://github.com/patrickmarlier/tinystm/blob/02c10ccb26ef68845dc97e4a4e7fc81be02eed42/src/stm.c#L924): token acquisition, validation, restart handling, serial quiescence, and final irrevocable-state transition.
- [src/stm_wbctl.h:410](https://github.com/patrickmarlier/tinystm/blob/02c10ccb26ef68845dc97e4a4e7fc81be02eed42/src/stm_wbctl.h#L410): committing writers participate in irrevocability. The optional `IRREVOCABLE_IMPROVED` branch contains an explicit quiescence-progress FIXME; do not blindly port it.
- [src/stm_internal.h:1418](https://github.com/patrickmarlier/tinystm/blob/02c10ccb26ef68845dc97e4a4e7fc81be02eed42/src/stm_internal.h#L1418): release occurs in commit cleanup.

C implementation. File headers offer GPLv2 or MIT dual licensing; the repository contains [MIT-LICENSE.txt](https://github.com/patrickmarlier/tinystm/blob/02c10ccb26ef68845dc97e4a4e7fc81be02eed42/MIT-LICENSE.txt). These inspected files establish provenance, not compatibility of every bundled dependency. No source was copied into Barn.

### Cicada: inspectable adaptive regulation

Engine repository: [efficient/cicada-engine](https://github.com/efficient/cicada-engine), inspected revision `af469679d67a59a89536de634216d349af577d3a`. C++, Apache-2.0 according to its [README](https://github.com/efficient/cicada-engine/blob/af469679d67a59a89536de634216d349af577d3a/README.md). [DB::update_backoff](https://github.com/efficient/cicada-engine/blob/af469679d67a59a89536de634216d349af577d3a/src/mica/transaction/db_impl.h#L291) is the concrete feedback controller: leader-only measurement, throughput gradient, bounded adjustment, updated shared backoff. The README warns about busy waiting with hyperthreading and RDTSC backoff during VM migration. These are reference implementation details, not appropriate defaults for Go goroutines.

## Transfer analysis for Barn

This section is original design analysis for the Barn constraints supplied by the parallel source audit. It does not assert that any paper proves the resulting Barn design.

### Replayability and irreversibility are separate facts

A continuation without a restorable start state cannot currently tolerate an OCC abort, even if it has performed no external effect. That is a runtime representation constraint. An emitted network message, an executed child process, and a committed external SQLite operation have a different constraint: restoring VM state cannot undo them. Represent these separately:

| Property | Consequence |
| --- | --- |
| Replayable, no effects crossed | Optimistic attempts are possible |
| Not replayable, no effects crossed | Needs protected execution until a restartable representation exists |
| External effect crossed | Must preserve the selected publication/effect contract; cannot retry the effect silently |
| Valid asynchronous boundary reached | Publish and detach only if MOO semantics define a new transaction segment there |

Do not turn a scheduler yield into an implicit database commit. Pausing instruction execution and publishing a transaction are distinct operations. Likewise, moving I/O onto another goroutine does not itself make releasing transaction protection safe.

### Counterexample: visible reads alone do not fix fixed snapshots

Suppose protected transaction `I` starts at snapshot `s`, reads `x`, and emits an irreversible effect. A concurrent writer `W` commits a change to `y`, which is not yet in `I`'s protected read set. `I` then reads the old `y(s)` and later fails current-version validation. Preventing writes only to locations already read did not make `I` irrevocable.

A fine-grained design therefore needs a specified version rule, not just a map of protected objects. Possibilities include a validated snapshot-extension protocol with atomic read protection, or a multiversion serialization protocol that allows the selected old versions to commit. Both need a proof covering future reads, read-modify-write, absent properties, object deletion, index/catalog predicates, and store metadata. Keeping today's fixed-snapshot validation while merely removing global exclusion is unsafe.

### Cancellation is not conflict abort

The future API must distinguish cancelling a queued request, abandoning an unclaimed offer, requesting cancellation of an executing VM, and aborting a speculative transaction. Once an irreversible effect has occurred, none of these imply permission for another goroutine to release the active holder's protection. The execution owner must reach its defined cleanup/publication boundary. A hung external operation is a responsiveness problem that fairness cannot conceal.

### Fairness has multiple measurable meanings

Equal completed transaction counts do not imply equal execution service: one long transaction can cost more than thousands of short ones. Equal CPU admission does not imply equal commit progress. A program can also monopolize a global gate while doing little CPU work. Barn should record separately:

1. Runnable wait and execution service by programmer.
2. Gate-request wait and actual exclusive occupancy.
3. Failed speculative work and successful committed work.
4. External wait and retained GC/execution resources.

Do not sum these into an unexplained single cost. Initially use execution service for dispatch fairness and gate occupancy for exclusive admission fairness. Retain a common identity/arrival sequence so the two policies do not fight by repeatedly rewarding the same work.

## Recommended design

### First buildable stage: explicit cooperative admission plus writer drain

Keep Barn's existing transaction semantics while making ownership explicit. A gate request has a stable identifier, programmer attribution, reason, arrival sequence, and state. Known protected continuations request admission before acquiring scarce execution/GC resources. Mid-slice promotion is a distinct path because it already owns a live VM and transaction.

Use one authority for the transition from ordinary committing writers to an exclusive holder. It closes ordinary-write admission, drains writers already in the publication phase, and claims the exclusive request. Validation is part of the promotion operation and completes before an effect is issued. Read-only behavior remains governed by the existing store contract. A checkpoint participates with its own reason and accounting.

The queue policy must guarantee that a continuously eligible older request cannot be overtaken indefinitely. Weighted service among programmers and a bounded interactive preference are reasonable implementation choices, provided the final rule is explicit and tested. A selected request cannot hold the gate while merely waiting for a worker that the holder prevents from becoming available. The design must state a single acquisition order across gate admission, VM leases, and GC quiescence.

This stage improves scheduling transparency and can reduce avoidable queue monopolization. It does not make a long active holder preemptible or prove improved login latency. It is the smallest useful integration surface for subsequent research-driven changes.

### Second stage: shorten the protected region through real continuation boundaries

Audit effectful builtins individually. For operations whose language contract already permits suspension, commit the defined prefix, issue the operation once, release execution resources, and enqueue exactly one completion continuation. Retain explicit effect state so cancellation, duplicate completion, and shutdown cannot duplicate the operation. Preserve error ordering and resumed values against the WSL Toast oracle.

Separately implement restorable slice-entry VM state for continuations that are semantically retryable. Include frame/stack state, local mutation, random/input observations, buffered output, and allocation roots. Replayability requires an inventory of everything outside the MVCC store; copying frame pointers alone is insufficient.

### Third stage: selective protected reads only if contention evidence warrants it

Prototype precise protection at Barn's logical validation-key granularity, rather than adopting machine-word Bloom filters prematurely. Require a written version/serialization argument and adversarial histories before enabling disjoint writers. Measure actual false-conflict cost first; Bloom filters are an optimization after the precise protocol works. Do not combine this store change with a new scheduler policy in the same experiment.

Adaptive control belongs after these mechanisms are correct. Tune concurrency or backoff using committed throughput subject to explicit latency/fairness constraints; freeze or reverse adaptation when sample confidence is poor. Estimates may guide soft escalation, but cannot manufacture deadline guarantees for unbounded external waits.

## Complexity versus quality and implementation effort

| Option | Effort drivers | Benefit | Main limitation |
| --- | --- | --- | --- |
| Dispatch fairness alone | Queue identity, admission points, accounting | Runnable work gets turns | Commit/exclusive wait can still dominate |
| Fair writer drain plus dispatch | Ownership state machine, shared-writer participation, GC order, cancellation | Coordinated progress with current semantics | Active long holder remains blocking |
| Real async effect boundaries | Per-builtin semantics, completion roots, exactly-once invocation | Removes legitimate external waits from occupied execution lanes | Cannot arbitrarily split atomic MOO work |
| Replayable continuation segments | Complete VM/runtime checkpoint contract | More useful optimistic execution | Effects remain non-replayable |
| Fine-grained irrevocability | New version/protection protocol and complete access instrumentation | Disjoint writers can proceed | Highest correctness burden |
| BOHM-style preordering | Stable access sets and transaction submission model | Deterministic concurrency | Poor match for arbitrary dynamic verbs |

Calendar estimates would be speculative before the source audit identifies exact ownership boundaries. The minimal useful deliverable is the admission/ownership stage; the full program includes asynchronous boundaries and replayable continuations. Fine-grained irrevocability is a separate store project, not a prerequisite for fixing queue unfairness.

## Questions resolved and remaining proof obligations

- **Is this an explored area?** Yes: the cited protocols explicitly integrate transaction progress, priority, irreversible operations, and scheduling.
- **Does MVCC eliminate this problem?** No. Version choice, validation, irreversible effects, and fair admission remain distinct obligations.
- **Can the gate be part of the algorithm?** Yes. Admission and promotion must be first-class transaction operations with compatible dispatch policy.
- **Is global exclusion theoretically necessary forever?** No. Safe weaker mechanisms exist under stronger instrumentation and version rules.
- **Should Barn immediately copy a fine-grained STM?** No. The fixed-snapshot counterexample identifies a missing proof obligation.
- **Can we promise a bound now?** Only a conditional service statement: finite admitted predecessors, eventual holder completion, nonzero service share, and scheduler progress. Hard elapsed-time bounds require bounded slices/effects and bounded admitted backlog.
- **Does priority escalation prove starvation freedom?** Not by itself; the PPoPP2009 body explicitly limits that claim.

Required implementation evidence: promotion race with a concurrent commit; cancellation at every request state; no duplicate effects after failed promotion; queued and active GC interactions; a continuously arriving foreground workload with old background/checkpoint requests; long-holder behavior; same-programmer queue flooding; future-read fixed-snapshot counterexample; absent-key/predicate conflicts; and measured Mongoose completion latency compared with the documented WSL oracle.
