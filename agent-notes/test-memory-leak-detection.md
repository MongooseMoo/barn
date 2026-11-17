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
cache controls passed locally. The Linux descriptor control passed in CI.
Both source and structured-source VM replay controls passed. Python evidence
and confirmation tests passed, including a real compiled replay subprocess.
Cycle acceptance tests passed. The same-runtime lifecycle replay initially
failed, identified the scheduler retention defect described below, and passed
after the repair. On an earlier published head, managed conformance and the full Elle
job passed; Jepsen remains in progress. The unit test program then exposed 661
native goroutine leaks. Its saved full stacks identify scheduler workers whose
test fixtures never stopped their runtime. Fixes are committed in the existing
shared fixture and the affected direct-construction tests. Linux descriptor
controls ran successfully as part of the internal/leakcheck package in that job.
After fixture cleanup, the next full-suite capture had only one remaining leak:
database/sql's connectionOpener labeled moo.task 94010. That producer is the
threaded SQLite retry test. The shared fixture stopped its runtime but did not
close its session. It now follows production shutdown by stopping the runtime
and closing the session, reporting any close error. Full CI verification remains
pending for that correction. The next Go test run passed and its saved captures
show zero native goroutine leaks for the parent and both diagnostics children;
the same-runtime replay verdict is healthy. The Python driver suite then found
its provenance fixture expected the pre-change parameter list. The fixture now
sets leak_check=True and asserts it in the complete parameter comparison. The
next full Go and Python driver suites passed. Expanding the existing workflow
audit to the touched CI/Jepsen/Elle files exposed ShellCheck SC2054 on the
existing comma-separated workload argument in a Bash array. It is now quoted
as one argument, preserving the selected workload list. The next audit exposed
nine self-repository references in the newly audited workflows. GitHub's primary
documentation says $/ resolves actions at the running repository commit and
requires runner 2.336.0. The actual runner log reports 2.338.2. Those nine
references now use that supported syntax; the audit remains unchanged.

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

Status reporting error: an earlier head's Elle pass was incorrectly called
current after a newer push. The statement was corrected against the current
head and its raw checks. Proposed mechanical status-output guard text, not
installed or approved: "For gh pr checks, replace unqualified pass/fail output
with records containing pr_head_sha, check_head_sha, check_name, conclusion,
and details_url. Mark records whose SHAs differ as stale."

A later note edit also used an anchor missing its existing line prefix. That
note patch was rejected; the live file was reread before correcting it. The
three preceding workflow-file edits succeeded and were separately inspected.

## Next step

Push the supported self-repository action references, then
follow exact-head CI to completion. Q has a pending exact-text approval request
for a scoped memory reproduction instruction in claude-fix.yml: its current
single-fuzz-invocation recipe cannot reproduce retained-memory findings.
No guard or governing workflow instruction has been changed without approval.
The implementation includes its first verified scheduler retention repair.
No isolation verdict is claimed from local heap/regression tests alone.
