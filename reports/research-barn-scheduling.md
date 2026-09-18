# Research: Barn scheduling

Research date: 2026-09-18. Scope: scheduling choices for a Go MOO runtime with interactive connections, background simulation, external completions, and shared mutable database state. This is an implementation proposal, not a claim that a new policy has been implemented or benchmarked.

## Summary

The useful research direction is frequent scheduling decisions, short admission queues, service accounting by stable owner, and bounded preference for latency-sensitive work. There is no universally best scheduler across all job distributions. Start by returning to input/completion processing between bounded dispatch turns; only then consider adaptive policy. Instruction-level VM preemption is a separate semantic change: a resumable interpreter state does not prove that another task can safely observe partially updated MOO state. Datacenter papers supply mechanisms and tradeoffs, not directly transferable microsecond targets or proof of MOO compatibility.

## Source comparison supplied by the concurrent source investigation

The parent investigation reports that Toast's `src/tasks.cc` admits waiting work to per-programmer FIFOs, selects one task per `run_ready_tasks` call from the least-used queue, favors input within the selected queue, and rotates equal-use queues. Its server loop revisits network I/O and child completions between such calls. The reported usage accounting has integer wall-second granularity. Barn's `ProcessReadyTasks` snapshots ready work and `runReadyTasks` processes its batches before recollection. These are investigation inputs, not independently re-read source evidence in this research report; use the parent's exact source excerpts for the diagnosis.

This distinction suggests a queue-admission and event-loop responsiveness fix before a new VM execution model. Choosing a smarter order once per large snapshot still leaves arrivals waiting for that snapshot to drain.

The parent further reports that Toast's `progr_of_cur_verb` uses the top activation's programmer, and the live Mongoose verb owners differ: `s_run` is `#2778`, `confunc` is `#2259`, and `time_offset` is `#2`. Therefore player identity or the root task's initial owner is not an equivalent accounting key. Trace the exact programmer attribution at fork and suspension boundaries before implementing compatibility behavior. A different stable workload principal can be an explicit enhanced policy, but must not be silently substituted for Toast's programmer semantics.

## Approaches found

### Weighted fairness with a separate latency preference

**Sources:** [Linux EEVDF documentation](https://cdn.kernel.org/doc/html/latest/scheduler/sched-eevdf.html), [Linux group scheduling documentation](https://docs.kernel.org/scheduler/sched-design-CFS.html).

EEVDF selects an eligible entity using service lag, then chooses the earliest virtual deadline. Shorter requested slices can improve responsiveness without granting unlimited CPU entitlement. Its sleeper handling prevents short sleeps from repeatedly erasing service debt. Linux group scheduling also distinguishes fairness between users/groups from fairness between individual tasks.

**Barn inference:** account service to a stable programmer or workload owner so spawning more forks does not buy more share. Separate that entitlement from a bounded interactive latency preference. Connection identity is useful within an owner's allocation, but anonymous connections need an aggregate limit. Forks and external waits must inherit attribution; a resume should not erase debt. A least-normalized-service policy is simpler to implement first than full EEVDF.

**Pros:** tunable shares, explicit anti-starvation intent, resistance to fork-count gaming. **Cons:** identity and debt lifetimes require careful definition; non-preemptible slices still limit responsiveness. **Complexity:** medium for weighted owner accounting; high for a faithful EEVDF implementation.

### Shinjuku: frequent preemption for variable job lengths

**Source:** Kaffes et al., [Shinjuku: Preemptive Scheduling for microsecond-scale Tail Latency](https://www.usenix.org/conference/nsdi19/presentation/kaffes), NSDI, February 2019.

Shinjuku addresses short requests waiting behind long ones by using virtualization hardware for frequent preemption. It demonstrates why run-to-completion dispatch becomes problematic under highly variable job duration.

**Barn inference:** measure the longest uninterrupted VM execution as well as queue delay. Fixing admission cannot remove the latency floor imposed by one long atomic task. The hardware/dataplane mechanism is unsuitable as a portable Go runtime dependency.

**Pros:** directly addresses head-of-line blocking. **Cons:** specialized platform and safe-preemption constraints; arbitrary interruptions can occur inside library calls. **Complexity:** very high as an imported mechanism; low as motivation for measuring execution slices.

### Concord: compiler-enforced cooperation and bounded worker queues

**Source:** Iyer, Unal, Kogias, and Candea, [Achieving Microsecond-Scale Tail Latency Efficiently with Approximate Optimal Scheduling](https://marioskogias.github.io/docs/concord.pdf), SOSP, October 23–26, 2023; [author project](https://dslab.epfl.ch/research/concord/).

Concord combines compiler-enforced cooperation, short bounded worker queues, and a work-conserving dispatcher. Its JBSQ policy trades a little queueing for less dispatch overhead. The paper explicitly avoids preemption in uninstrumented external calls and uses lock tracking to prevent yields while application locks are held.

**Barn inference:** bound work already assigned to workers so a new interactive task is not hidden behind a long local backlog. VM safe points could eventually avoid arbitrary Go-stack interruption, but interpreter safety, lock safety, and MOO semantic atomicity are distinct requirements. Do not copy the paper's queue depth or timing constants without measuring Barn.

**Pros:** exposes latency/throughput tradeoffs; avoids expensive asynchronous interruptions. **Cons:** safe regions delay yielding; additional instrumentation and runtime state. **Complexity:** medium for bounded dispatch, high for VM preemption.

### Tiny Quanta: two scheduling levels and frequent safe-point switching

**Source:** Luo et al., [Efficient Microsecond-scale Blind Scheduling with Tiny Quanta](https://amyousterhout.com/papers/tinyquanta_asplos24.pdf), ASPLOS, April 27–May 1, 2024; [conference abstract](https://www.asplos-conference.org/asplos2024/main-program/abstracts/).

Tiny Quanta combines compiler-inserted yielding with two-level scheduling. Keeping a job on one worker improves locality while distributing scheduling work. It supports unknown job lengths rather than requiring accurate duration predictions.

**Barn inference:** if many-core scheduling overhead later dominates, separate owner admission from worker-local execution and keep local queues short. Preserve task affinity where useful. This is a later scaling option; increasing worker-local backlog now could recreate the starvation problem.

**Pros:** low-overhead switching and locality. **Cons:** more queue coordination and instrumentation; microsecond experiments concern a different runtime and workload. **Complexity:** high.

### Caladan: feedback from latency and interference

**Source:** Fried et al., [Caladan: Mitigating Interference at Microsecond Timescales](https://www.usenix.org/conference/osdi20/presentation/fried), OSDI, November 2020.

Caladan adjusts core allocation using signals about demand and interference instead of relying solely on fixed partitions. Its mechanism includes a dedicated scheduler and kernel support.

**Barn inference:** a future controller could adjust background admission using observed interactive queue age, runnable backlog, throughput, and commit contention. Keep a configured background minimum and use smoothing/hysteresis so bursts do not cause oscillation. This is an analogy, not an evaluated Caladan policy for Barn.

**Pros:** responds to changing workload rather than a single static setting. **Cons:** feedback tuning can oscillate; extra workers may worsen contention. **Complexity:** medium for guarded admission feedback, very high for Caladan's platform.

### Recent related work: XRT and deadline admission

**Sources:** Patel and Alian, [XRT: An Accelerator-Aware Runtime for Accelerated Chip Multiprocessors](https://www.usenix.org/conference/atc25/presentation/patel), USENIX ATC, July 7–9, 2025; [paper](https://www.usenix.org/system/files/atc25-patel.pdf). [A Tail Latency SLO Guaranteed Task Scheduling Scheme for User-Facing Services](https://ieeexplore.ieee.org/document/10891045/), online February 14, 2025, TPDS 36(4), April 2025.

XRT revisits scheduling when offloaded work and completion handling become bottlenecks. Its paper identifies wasted scheduling of jobs still awaiting offloads. Barn's related lesson is to distinguish blocked tasks from executable continuations and to make completion events visible promptly, not to adopt an accelerator runtime. The TPDS paper combines deadline queuing and admission control for fanout services; that workload differs from MOO, but reinforces that priority alone cannot guarantee latency when arrivals exceed capacity.

The bounded search found recent specialized work, but did not establish a 2026 general-purpose replacement that should supersede the above mechanisms for Barn. None of these papers proves one policy is universally superior or directly compatible with MOO execution.

## Existing implementations

| Project | Implementation and verified license information | Fit |
| --- | --- | --- |
| [Shinjuku](https://github.com/stanford-mast/shinjuku) | Native dataplane implementation; project states MIT-style license; hardware and DPDK dependencies | Read design, do not import its execution platform |
| [Concord](https://github.com/dslab-epfl/concord) | LLVM passes, runtime library, Shinjuku integration; repository currently labels MIT | Useful source for cooperation and bounded queue mechanisms |
| [Tiny Quanta](https://github.com/zhluo94/TinyQuanta) | C/C++ runtime and compiler instrumentation, instrumented RocksDB; no top-level license verified in the inspected listing | Research reference; do not assume code reuse permission |
| [Caladan](https://github.com/shenango/caladan) | Native runtime, I/O kernel, kernel support; repository labels Apache-2.0 | Feedback ideas, not a portable Barn dependency |

License labels describe the inspected top-level projects, not every vendored dependency. No third-party code was copied.

## Recommendations and complexity tradeoffs

1. **Fix dispatch granularity first.** Revisit input, external completions, and owner selection after a bounded unit of admitted work. An elapsed dispatch budget should be checked between existing task execution boundaries, not inserted as a new MOO yield. Count limits bound queue monopolization; elapsed limits detect expensive turns. Neither can interrupt an already executing atomic task.
2. **Add owner fairness before adaptive intelligence.** Maintain normalized accumulated service and deterministic tie rotation. Use a monotonic high-resolution elapsed measurement if that is what the runtime can reliably obtain; call it execution occupancy, not CPU time, because Go scheduling and GC pauses can be included. Decide explicitly whether conflict retries charge the owner and measure wasted work separately.
3. **Give interactive work bounded preference.** Carry request lineage across helper waits, but do not mark every external completion interactive. Preserve a background service floor and cap consecutive foreground service. Derive weights and budgets from measured latency/throughput tradeoffs, not paper-specific microsecond numbers.
4. **Instrument before automatic adaptation.** Record ready-to-dispatch, worker/gate wait, uninterrupted execution, external wait, completion-to-resume, conflict/retry time, and oldest pending age by class/owner. This distinguishes admission starvation from slow bytecode, locks, GC, or transaction contention.
5. **Only then consider feedback.** Reduce background admission when interactive queue age is high; increase it gradually when latency has margin. Keep bounded parameter ranges, hysteresis, and a static fallback. Do not introduce runtime prediction or reinforcement learning without evidence that this simple controller is insufficient.

Keep the first operator tuning surface small: **principal service share**, **latency objective or dispatch slice**, and **maximum in-flight work per principal**. These are design proposals, not existing settings or measured defaults. Bound foreground bursts and preserve background progress within the policy; expose more knobs only when workload evidence requires them. A latency objective is not a promised upper bound: weighted selection cannot guarantee response latency without a bound on every non-preemptible execution region. Keep VM ticks and execution timeout separate because quota exceptions are observable MOO behavior, while dispatch frequency is a scheduler choice.

## Cooperative VM execution, concurrency, and MVCC

This section is a design inference for Barn, not a claim that the cited papers implement MOO transactions.

Pausing at an opcode boundary can preserve interpreter state while still exposing a half-finished database update if another task runs. Holding an exclusive execution gate while paused preserves exclusion but does not help a foreground task requiring the same gate. Releasing it requires either an existing legal MOO suspension boundary or an explicit isolation protocol. MVCC alone does not prove serializable behavior, and serializability alone may not preserve every observable ordering guarantee.

A speculative approach would need read/write conflict validation and a defined commit order. Network output, `exec`, files, SQL, random/time observations, and task creation must either be buffered, represented as validated effects, or cause an irreversible boundary. Retrying a transaction must not duplicate external effects. Cancellation and quota accounting must survive pauses and retries. These requirements make general VM preemption a separate project, not a responsiveness knob in this fix.

Parallel workers should also have bounded admission: many speculative background tasks can exhaust workers or repeatedly invalidate foreground transactions even when the ready queue policy is fair. Reserve capacity only after proving that shared gates and commit ordering can use it.

## Estimated implementation effort

- **Minimal, low-to-medium complexity:** bounded dispatch with event-loop return, readiness visibility, and focused fairness/ordering regression coverage. Keeps existing MOO execution boundaries.
- **Practical policy, medium complexity:** stable owner accounting, bounded interactive preference, configuration, queue-age telemetry, and sustained-load fairness checks.
- **Adaptive policy, medium-to-high complexity:** controller, stability safeguards, overload behavior, and broad workload evaluation.
- **Full fine-grained preemption, high complexity:** explicit isolation/effect protocol plus VM continuation and cancellation integration. Estimate only after auditing existing transaction guarantees.

These are relative scope estimates, not elapsed-time promises.

## Open questions

- Which identity should own login work before authentication, and how does it transfer afterward?
- Does the execution gate serialize all useful foreground work, or can read-only/disjoint tasks proceed?
- Which MOO execution boundaries are already observable suspension/commit points?
- How long is the worst existing uninterrupted execution under the actual Mongoose workload?
- Do fork descendants inherit owner debt and interactive lineage consistently?
- Are background waits bounded under sustained foreground traffic, and vice versa?
- Does changing dispatch order preserve `queued_tasks`, cancellation, sibling visibility, and external completion semantics?

## Validation after the source-derived change

Use controlled workloads to confirm the specific implementation: background fork flood plus new input; multiple owners with unequal task counts; completed helpers with both interactive and background lineage; long atomic task; foreground flood; and multiple worker counts. Measure p50/p95/p99 input-to-first-output and completion-to-resume alongside background throughput and oldest task age. These checks confirm policy and expose tradeoffs; they do not replace inspecting Toast's scheduler or proving MOO behavior against the oracle.
