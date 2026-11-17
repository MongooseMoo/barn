# Memory leak detection

## Goal

Implement the approved memory-leak plan in an isolated worktree, using existing
fuzz, Elle, Jepsen and CI producers and their server runners.

## State

Published as PR #873 from the requested worktree; implementation is under CI.
The original checkout and its research artifacts remain untouched. Shared Go
capture, bounded VM corpus replay, native memory reporting, Elle capture,
managed Jepsen generation capture and CI integration are implemented. Full CI
and workflow audits are pending. Initial CI stopped at a staticcheck tagged-switch
finding in the new control test; that test is corrected on this branch.
Native goroutine, retained heap and bounded
cache controls passed locally. The Linux descriptor control awaits CI.
Both source and structured-source VM replay controls passed. Python evidence
and confirmation tests passed, including a real compiled replay subprocess.
Cycle acceptance tests passed. The same-runtime lifecycle replay initially
failed, identified the scheduler retention defect described below, and passed
after the repair. On the published head, managed conformance and the full Elle
job passed; Jepsen remains in progress. The unit test program then exposed 661
native goroutine leaks. Its saved full stacks identify scheduler workers whose
test fixtures never stopped their runtime. Fixes are prepared in the existing
shared fixture and the affected direct-construction tests. Linux descriptor
controls ran successfully as part of the internal/leakcheck package in that job.
After fixture cleanup, the next full-suite capture had only one remaining leak:
database/sql's connectionOpener labeled moo.task 94010. That producer is the
threaded SQLite retry test. The shared fixture stopped its runtime but did not
close its session. It now follows production shutdown by stopping the runtime
and closing the session, reporting any close error. Full CI verification remains
pending for that correction.

## Decisions and why

- Go 1.27 native goroutineleak profile first; no new dependency or public server
  endpoint. debug=2 emits all goroutines, so only normal goroutine stacks use it.
- Heap growth is a candidate requiring independent repeated-process confirmation.
  Exact descriptor and native goroutine findings are reported separately.
- Reuse production fuzz execution; do not collect or GC each native mutation.
- Keep Linux process resources and shared cgroup/tmpfs use separately attributed.
- Preserve correctness results even when leak capture fails.

## Surprises

The new same-runtime replay retained roughly 2.2 MB across 96 eval/suspend/create
cycles. Original evidence is under .fuzz/local-engine-memory/runtime-lifecycle/.
At all sample boundaries tasks=0, readers=0, history_entries=1 and pending
anonymous/WAIF GC=0. Profile differences point to VM locals and task contexts;
the retaining owner was scheduler.yieldBarriers. RequeueYield adds task pointers,
but only Dispatch pruned completed tasks. Eval drives Ready, and embedded batch
callers drive ReadyBatch. Moving the existing prune to shared readyLocked fixes
both paths without discarding queued barriers. The new deterministic regression
failed on both paths before the change and passed afterward. The same-runtime
heap replay then passed; fixed evidence is under .fuzz/local-engine-fixed/.

An explicit source search named a nonexistent engine/task_execution.go instead
of resolving it first; rg reported path-not-found. Subsequent source exploration
uses the live file inventory. Proposed guard from the research note remains
unapproved and has not been installed or changed.

Full-suite captures exposed test-runtime ownership leaks, not a need to ignore
worker stacks. The shared runtime fixture now registers Stop with test cleanup;
direct-construction tests now do the same. A diagnostic child inherited the
parent's capture directory and overwrote its snapshots; test-program captures
are now partitioned by PID. A multi-file edit was rejected because the fairness
test constructed its store inline; after reading that anchor, repairs were
applied per file. No rejected edit was partially installed.

## Next step

Push the session-ownership repair and improved incomplete-replay diagnostics, then
follow exact-head CI to completion. Q has a pending exact-text approval request
for a scoped memory reproduction instruction in claude-fix.yml: its current
single-fuzz-invocation recipe cannot reproduce retained-memory findings.
No guard or governing workflow instruction has been changed without approval.
The implementation includes its first verified scheduler retention repair.
No isolation verdict is claimed from local heap/regression tests alone.
