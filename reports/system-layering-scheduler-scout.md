# Scheduler and admission source survey

## State

P4 scout is tracing actual readiness and ownership producers. This report is
persisted by the coordinator because the scout is read-only.

## Dispatch and execution capacity are distinct

InputProcessor calls asynchronous DispatchReady at server/input_processor.go:285.
Runtime supplies unfinished tasks and ReserveAdmission at engine/runtime.go:608.
Scheduler.Dispatch claims before incrementing inFlight and starting a goroutine
at engine/scheduler.go:201-229. runTask then waits for background admission at
engine/task_runtime.go:71-87 through engine/admission.go:59-67. Dispatch slots
therefore include admission waiting and callbacks, rather than just physical
execution. Runtime releases the physical lease separately; scheduler releases
its slot after run returns and before the done callback. These represent
different lifetimes. This trace alone proves no stranded capacity and does not
justify changing limits.

## Keep reservation and physical ownership

ReserveAdmission excludes duplicate dispatch while queued at task/task.go:486.
ReadyDeadline excludes admissionPending, executionActive and input wait at :401.
runTask waits for admission before TryClaimQueued, then acquires the physical
lease before VM work. It publishes saved state and releases context/physical
ownership before finishing the scope; callbacks follow that boundary.
PendingReadier intentionally survives admission, execution and retry/logical
suspension while physical ownership remains. Combining these states would
erase real invariants rather than simplify them.

## Delete candidate: unused required host capability

TaskYielder has no call site. Its only references are the declaration in
builtins/tasks.go, Host field and validation in builtins/host.go, server startup
wiring, three test assignments, and Runtime/InputProcessor wrappers named
YieldReadyTasks. ProcessReadyTasks has a real engine/eval.go caller and remains.
Delete the declaration, Host field/check, startup wiring, both wrappers and
three obsolete test assignments in engine/yin_commit_test.go and
engine/worker_pool_test.go. Add no replacement. Host validation's existing
connection-manager failure expectation remains valid. Existing Yin commit,
refresh and committed-fork regressions cover the real path; compile and PR CI
are appropriate for this pure deletion, without a bespoke mirror test.

## Keep yielding and async handoff

Admission Yield deliberately waits while physical VM/transaction ownership
remains. Production preemption refuses to yield under the exclusive commit
gate and lends only a root-owned scope. Retry releases the irreversible-effect
gate before rerunning. External workers stage their starts until commit and
resume through suspension generations; saved VM publication precedes yielded
requeue. No source-proven deadlock or stranded capacity was found by this
bounded survey. Scout delivered and stopped; no local tests were run.
