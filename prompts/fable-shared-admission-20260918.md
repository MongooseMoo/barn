# Review shared execution and commit admission before implementation

Authoring model: OpenAI Codex. Reviewer: Claude Fable, independent provider.
Repository: C:/Users/Q/code/barn, current HEAD 83751cc (verify).
Read-only source review; write only reports/fable-shared-admission-20260918.md.
Do not stage, commit, change runtime files, or launch live databases.

Read plans/barn-transaction-scheduling-design.md, engine/runtime.go,
engine/task_runtime.go, engine/eval.go, engine/call_verb.go,
engine/task_factory.go, server/input_processor.go, db/store/store_core.go,
db/store/store_txn.go, db/store/store_snapshot.go and relevant call sites.

The user authorized the next implementation: one tunable weighted-service
admission policy spanning foreground and background, and FIFO shared/exclusive
commit-gate admission. Earlier increments implemented persistent background
batches and wakeups. Gate ownership and admission are still outstanding.

Proposed implementation for challenge:
- Runtime-owned admission queue: principal = player for input, activation
  programmer for background; shared anonymous principal for pre-auth with
  separate connection ordering retained by current lanes; explicit system key.
- Min normalized settled+reserved service; positive bounded EWMA reservation;
  input/background class weights 3:1; configurable global and principal caps.
  No reset on idle; clamp returning principals to monotonic service watermark.
- Top-level runTaskSlice, Eval, and standalone CallVerb acquire before physical
  VM lease. Context/inherited/sweep-owned calls borrow rather than reacquire.
  Settle service over invocation including retries, subtract measured gate wait.
  Retain current retry-safe/solo background batch rules.
- Replace the store RWMutex with FIFO consecutive shared cohorts / exclusive
  holders. Keep lease-before-gate order, no maintenance drain. Cancellation and
  release must not unlock a live owner; snapshots and ordinary commits participate.
- Scope actual execution occupancy, not CPU accounting. No performance claim
  without identical live 1/16-client workload results.

Find concrete integration traps and recommend the smallest sound implementation.
Specifically inspect synchronous nested calls, checkpoint callbacks, finalizers,
root publication before admission wait, direct eval/verb paths, old task-ID-zero
hooks, held-worker eligibility, cancellation and shutdown. Enumerate ALL VM-entry
paths requiring admission or borrowing. If the proposed global cap can deadlock,
give the exact timeline and a feasible alternative. Identify where to account
gate waits (including ordinary commits) without conflating MOO quota clocks.
Do not merely repeat the design. Report blocking issues, file/line evidence,
recommended implementation seams and focused deterministic tests. End with
READY or REVISE and concrete corrections required.
