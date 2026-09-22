# Game out Barn scheduling with the commit gate in the algorithm

The user explicitly requests Claude Fable as an external design partner. Read
source first, then challenge and improve the proposal. This is a design exercise;
do not implement runtime changes or run conformance tests. No need to reproduce
existing login delays. Do not read credentials, private transcripts, or databases.

You own ONLY `reports/fable-scheduler-gate-20260918.md`. Other agents/user work is
present. Do not revert, stage, commit, start/stop servers, or edit other files.
Read AGENTS.md. Read-only source exploration and commands are authorized.

## Context and files

Barn branch fix/mongoose-workload, current HEAD ef5fcb0.
Read:
- reports/barn-toast-scheduler-source-audit-20260918.md
- reports/research-barn-scheduling.md
- engine/runtime.go and engine/internal/scheduler/scheduler.go
- engine/task_runtime.go (entire attempt, irreversible effect, retry, suspension,
  gate acquisition/release, callback and deferred GC boundaries)
- db/store/store_core.go, db/store/store_txn.go, db/store/store_snapshot.go
- relevant gate, irreversible-effect, concurrency, GC and foreground-input tests
- server/input_processor.go and engine/task_factory.go for direct foreground runs.

Verify paths before reading. Toast source if useful: WSL Debian
/root/src/toaststunt-mongoose-login-20260917, commit
72e3c7f96ce7a41fdeba793aef8818dc4408072e. Verify distribution before -d;
read-only WSL commands begin set -euo pipefail and use --exec bash -lc.

Source findings: Toast admits by current activation programmer to FIFO queues,
selects one task per main-loop turn from least-used active queue, charges whole
wall seconds, reinserts equal usage at back, tries input before background in a
queue. Barn drains all batches in a global snapshot, has no programmer usage
accounting. Input runs separately; exec continuations return through scheduler.
Mongoose s_run/confunc/time_offset verb owners differ (#2778/#2259/#2), but current
activation identity can differ from static/root owner. Do not overclaim timings.

## Candidate to challenge

Bounded admission + per-programmer weighted service + separate interactive
latency preference + per-principal in-flight cap. Initially preserve existing MOO
yield/transaction boundaries; do not arbitrarily preempt an activation. Treat
foreground, background, external continuations, gate ownership, gate waiters,
optimistic commit readers, checkpoints and GC as parts of one scheduling design.

User's key correction: the commit gate must be part of the chosen algorithm,
not an afterthought. A nice ready queue cannot fix priority inversion while a
background task holds the exclusive gate or all workers block trying to acquire
it. Conversely, speculative readers might proceed safely until commit; verify.

## Questions and concrete scenarios

1. Map the actual resource/state graph: execution slots, commit gate shared vs
   exclusive, store locks, lifecycle/GC barriers, direct foreground execution.
   Precisely distinguish gate acquisition, holding, waiting, and safe handoff.
2. Walk timelines for: A holds gate after an irreversible effect while B login
   arrives; workers all block acquiring it; B is read-only vs needs commit;
   repeated optimistic conflicts trigger escalation; a holder awaits an external
   result; heavy foreground traffic starves background; a checkpoint/GC waits;
   pre-login transitions owner; cancellation kills a waiter or holder.
3. Compare queue-only fair policy, gate-aware admission, priority inheritance or
   donation, a unified execution/gate arbiter, and optional serialized mode.
   What can help without new MOO-visible interleavings? What cannot be bounded?
4. Can an effectful task be parked BEFORE acquiring the gate without losing its
   VM/transaction state or duplicating effects? What must be revalidated? Is this
   materially different from yielding AFTER taking the gate? Locate source.
5. Recommend the smallest cohesive implementation and boundaries. Include a
   state machine or pseudocode, lock-order invariants, owner/lineage/accounting
   policy (wait time vs execution vs retry cost), and no-worker-exhaustion rule.
6. Suggest few meaningful operator knobs with defaults left unmeasured. Bound
   foreground preference; explain what latency objectives can/cannot promise.
7. Identify claims in our source/research reports that are wrong or too strong.
   Explicitly challenge the idea that simply reducing batch size solves this.
8. Which trace points and deterministic schedules would discriminate policies?
   Source evidence first; tests should validate the resulting diagnosis.

Write a concrete design report with source file:line citations, adversarial
timelines, rejected alternatives, recommended phases, unresolved choices, and
honest guarantees. Label proven source facts separately from design proposals.
We will follow up in the same session to challenge your first recommendation.
