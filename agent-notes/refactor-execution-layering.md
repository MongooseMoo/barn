# Execution layering

## Goal

Q requested principled ownership and truthful naming across kernel, engine, VM,
and surrounding packages, including moving code and assessing performance.
Q added the untyped saved VM dependency to scope and authorized implementation
with "Make it so!" on 2026-10-26. Q delegated tooling choices.

## State

Worktree: `C:/Users/Q/code/barn-execution-layering`, branch
`refactor/execution-layering`, base `5b0cd9ca7379a5c8cb22df2a1c5b96b78cd2349e`.
The context slice is implemented: kernel/context.go and its tests moved to task;
callers now use task.Context and task.NewContext. The first slice reached
6e54bba7 (context move 0d5c074f plus selector repair).
CI run 40285084528 reports Build & unit tests pass (1m9s), MOO conformance suite
pass (7m38s); run 40285084536 reports Serializable histories and negative control
pass (3m1s). The Elle artifact is 12633877005, named
elle-evidence-0512c32f674e733789e50a17a1098226ffbfbe40; it has not been downloaded
or inspected, so this records the job result, not a separate isolation audit.
Earlier runs 40284826132 and 40284826145 failed on the repaired alias defect.
No merge or release for this task yet. The original checkout has unrelated
untracked work; it is not this task's edit target.

The profiling boundary is pushed as a92aba44: Execution.ProfileParent carries
the four mooprof sampling operations instead of CallingVM any. The live-frame
and baseline loops were extracted into VM methods, retaining the cached
Execution binding. Existing vm/profile_test.go covers nested stack attribution
and tick accounting. Build & unit tests passed (1m26s) in run 40288400397;
Elle job passed (2m50s) in 40288400399; conformance is still pending. No new tests.
The preparation was pushed as 1dec5d4b, separately from profiling:
gopls semantic previews renamed task.Task to State, the enum field to Status,
the shared mutex to Mu, readingOrder to ReadingOrder, and constructors to
NewState/NewStateFull. Applied all previews through apply_patch. Updated the
existing source-text setter test to recognize Status; retained its exact
two-setter requirement. State.Init initializes fresh inline storage, and both
constructors now share those defaults instead of duplicating them.
Master was incorporated in ef3dcc6c, migrating a concurrently added Elle caller.
That head passed conformance (7m27s) and Elle (2m35s); build CI stopped at formatting
in run 40289346185. The local ownership patch includes the formatting repair.

The ownership slice is pushed in d3855f83, with caller/style repairs in
2cb6ed0f and 93a6ec61. At 93a6ec61 PR checks report build/unit pass (1m12s),
conformance pass (7m29s), and Elle job pass (2m54s). The Elle archive has now
been inspected: three seeds true in both models, ten fault controls false,
nonzero required paths. See experiments/execution-layering-ownership-elle-summary.txt.
- engine.Task embeds task.State and owns machine *vm.VM under the shared Mu.
- Catalog and scheduler moved into engine; builtin consumers receive State.
- Combined persistence capture holds Mu across metadata and concrete VM capture.
- Settlement notification preserves catalog -> State.Mu -> unfinishedMu order.
- SuspendSeconds now belongs to State; the host no longer forwards that operation.
- Existing catalog, scheduling and builtin integration tests moved to engine.
  Standalone VM tests use an empty host (no task index); pure builtin tests keep
  their existing empty host stub. HTTP assertions inspect actual suspension.
- Retry, GC, fork, load and direct-hook callers now use the concrete owner, while
  nested VM/finalization paths use lower State. Saved-machine assertions removed.

No local test/typecheck CI steps were run.
Layout correction and evidence are pushed as 58a72043. Build/unit and Elle jobs
passed (1m4s/2m33s); conformance pending at the last check. Longer unprofiled
base/head/head/base continuation repeats do not consistently reproduce the
short-run slowdown (timing p=.165); host variability limits the conclusion.
Remaining: full current-head inspection and current CI. Current architecture is documented in
docs/execution-ownership.md. Performance evidence, fixture hashes and limitations
are in experiments/execution-layering-performance.md. The real workload has run
on both sources: baseline failed with a deadline; head exited successfully but
still had two native E_NACC failures. Neither supports clean workload acceptance.
All three jobs on
documentation head 95a0a1af completed SUCCESS
(runs 40286736548 and 40286736504); those precede this profiling implementation.

## Decisions and why

Ownership measurements now compare merge base 11a17d8c with head 93a6ec61.
Both interpreter benchmarks add 64 B/op with unchanged allocation counts.
Reflection reports VM size 632 -> 640 and profile state 64 -> 72 bytes;
these sizes do not by themselves prove the allocator's size-class cause.
Smallest correction: move the existing cmdStrSet and yielded flags next to
Resumable (about six lines changed), preserving the typed profiling boundary.
vm/vm.go has separate flag groups before aligned fields. Repository Go-source
search found no VM unsafe offsets or indexed reflection consumers. Measure
the resulting size and allocation change; timing remains unresolved because
the first continuation samples differ in both directions on a shared host.

The target is explicit ownership: task execution state below the interpreter;
bytecode execution and its frames in vm; scheduling, execution lifetime,
publication, and runtime orchestration in engine; transport in server.
Do not reconstruct old owners through compatibility aliases or wrappers.

First slice, smallest concrete change: move kernel/context.go and its two tests
into task, name the type task.Context and constructor task.NewContext, and
migrate callers. About 280 existing lines move, with roughly 200 caller/import
edits; no new execution algorithm, lock, service, or abstraction. Task context
and deferred effects already describe task-owned state, not a separate kernel.
Update the stale transaction comment: builtins/properties.go:145 writes through
readTxn(ctx), and engine/task_runtime.go:439 commits ctx.StoreTxn.

The full goal also includes eliminating untyped VM ownership; the first slice
does not complete it. Before that move, resolve all current consumers:

- task/task.go:97 stores BytecodeVM as interface{}; :805 and :813 protect it
  under the task mutex, including cross-goroutine GC reads.
- vm/vm.go:34 imports the concrete task; opcode, traceback, profiling, budget,
  and GC paths use it. Moving the VM field alone cannot resolve the cycle.
- task/snapshot.go:175 defines vmSnapshotter; :222 obtains persistence state
  from the saved VM while taking the task snapshot.
- engine/internal/scheduler/scheduler.go:314 and :404 inspect saved-VM presence
  for readiness. Scheduler and catalog changes must preserve this distinction.
- builtins/registry.go:37 carries CallingVM as any; vm/op_misc.go:17 supplies
  the real VM, and engine/call_verb.go:135 consumes it for nested profiling.

These are real caller contracts to relocate, not reasons for a new generic
execution interface or an extra VM lookup map. The exact second-slice move
will be written here after reading the remaining consumers.

### Second-slice source findings (2026-10-26)

The cycle is not just task -> VM: vm imports builtins and builtins itself needs
concrete task metadata/control (builtins/host.go:33-50, tasks.go, stdin.go,
network.go). Moving only the direct vm.Task field cannot let task import vm.

Do not replace the saved VM with its persistence snapshot: vm/retry.go:11-47
explicitly retains a different in-memory retry representation, and VM frame,
local, and builtin-closure reuse must remain intact. Snapshotting to avoid a
dependency would change both runtime cost and ownership.

The existing catalog owns task identity and an unfinished index, not VM state.
Its task -> catalog callback runs with task.mu held and takes only unfinishedMu
(task/task.go:338; task/manager.go:75-99). Preserve that lock order. Relocating
the catalog requires relocating this contract, not exposing arbitrary setters.

Direct server-hook tasks are created without catalog registration
(engine/call_verb.go:252 onward); fork VMs are installed before QueueTask
(engine/task_factory.go:304,348). A catalog-only machine slot is not a drop-in
replacement for task-owned execution. A new pointer-keyed runtime map could
cover those lifetimes, but adds an index and cleanup obligations; it is not yet
selected as the solution.

Candidate to finish evaluating: an engine-owned executable task with a concrete
*vm.VM and lower task state consumed by builtins/VM. This would move catalog and
scheduler ownership upward as well. It must retain one allocation/lock for the
task, support unregistered direct work, and keep snapshot capture atomic with
machine publication. Do not implement an embedded-pointer wrapper, new mutex,
generic Task[T], or broad Machine interface merely to compile around the cycle.
The selected construction is now an inline task.State in engine.Task, with a
concrete private *vm.VM and the existing shared mutex. The catalog and scheduler
move into engine so they retain concrete executable ownership. Lower state
notifies settlement through a narrow observer bound to its containing Task;
there is no extra catalog, task allocation, or lock. Rename the enum-valued
State field to Status before embedding. Preserve constructor defaults through
in-place initialization. Combined persistence capture stays under the same lock.
The report docs/reports/execution-ownership.md records the exact move and test
migration. Estimated substantive production change: 150-300 lines, with about
1,000-1,300 catalog/scheduler lines relocated and mechanical caller changes.

First implement the independently separable profiling boundary: replace
CallingVM any and its assertion with mooprof.Parent's four sampling operations,
extracted from vm/profile.go's current loops. This preserves live ancestor
frames and tick/time baselines without exposing VM execution to builtins.
Expected change about 60 production lines; no new tests or profiling algorithm.
Then perform the executable ownership move above.

Builtin test dependencies also matter: builtins/task_manager_test.go wires the
current task.Manager; several authority/completion/IO tests depend on it.
Moving that manager above builtins cannot be patched by importing engine from
same-package builtin tests. Preserve those behavioral tests using the final
public boundary, not a duplicate production manager in test scaffolding.

During caller migration, VM-only suspension tests also use an empty catalog.
The current manager's SuspendTask reads no manager fields: it only delegates
to State.Suspend, SuspendIndefinite and Resume. Move those existing 20 lines
to State.SuspendSeconds and remove that operation from the host controller.
Keep existing missing-host error branches unchanged. Pure interpreter tests
need only an empty finder/lister/controller, not a copied catalog implementation.
HTTP tests will inspect the actual suspended state instead of recording a
call to the removed manager method. This is relocation of the existing
operation and assertions, not new scheduling behavior or a new test guard.

## Tooling and verification

Use go list for current dependencies and gopls semantic rename previews for
identifier edits, including import aliases and names inside relocated packages.
Relocation is not an exception: rename symbols before moving the file, or edit
the exact declaration/caller. The scheduler move used a word substitution for
New and incorrectly changed errors.New to errors.newScheduler; CI 40291571255
caught it, alongside missed lower fork-snapshot test callers. The first patch incorrectly used
plain text replacement for task-package qualifiers; that also changed field
selectors such as wake.task.Resume to wake.taskpkg.Resume. Replace that
substitution approach with symbol-aware previews; restore the affected field
selectors directly. CI run 40284826132 reported network.go:631,684,717,757;
the same replacement also affected two engine test files.
Apply all source/note changes through apply_patch. gopls, gorename, and
benchstat are installed. Q said to choose tooling; no WOPR dependency remains.
The first rename formatting output was not applied across the complete changed
set after its command yielded; formatting only task/task.go left the pushed
preparation unformatted (CI 40289346185). Apply the formatter's returned edits
to the whole changed-file set, not just the declaration file. This corrects the
editing operation; it adds no separate local validation gate.
Follow PR CI rather than rerunning its checks locally. Preserve existing tests.
Read docs/elle-isolation.md before changing retry/suspension ownership; its
pinned checker, manifests, witnesses and nonzero paths are required evidence.

Performance work uses existing VM benchmarks, threaded continuation benchmarks,
and the real Mongoose workload. Compare this task's base and changed head with
the same Go toolchain, workload and machine; report allocation and time results
separately. scripts/perf-compare.ps1 hardcodes a June baseline, so that baseline
cannot establish this change's effect. Preserve vm/op_misc.go's cached builtin
Execution and closures and vm/vm.go's frame/local reuse. Use separate CPU,
allocation, and wait diagnostics as needed under docs/workload-diagnostics.md.
No speedup claim follows from moving packages.

### Initial performance evidence

Base worktree: `C:/Users/Q/code/barn-execution-layering-base`, detached at the
original base. Head measured: 6e54bba7. Windows/amd64, Go 1.27.1, Ryzen 5950X,
GOMAXPROCS=4. Runs were sequential base/head/base; CPU frequency was not pinned
and the shared machine was not isolated. These are preliminary microbenchmarks,
not real-workload or general performance acceptance.

Command in each worktree:
`go test ./vm -run '^$' -bench '^BenchmarkVM/(int_arith_1M|builtin_abs_200k)$' -benchmem -benchtime=300ms -count=10`

Raw files in experiments/: execution-layering-base-vm.txt,
execution-layering-context-vm.txt, execution-layering-base-repeat-vm.txt.
The initial comparison misleadingly suggested an 11.09% arithmetic improvement;
the repeated base removed it. Preserve the first run instead of discarding it.
execution-layering-context-benchstat.txt compares repeated base with head:

```text
VM/int_arith_1M-4      39.20m -> 39.31m sec/op  ~ (p=0.912 n=10)
VM/builtin_abs_200k-4  27.71m -> 27.83m sec/op  ~ (p=0.529 n=10)
VM/int_arith_1M-4      19.00 -> 19.00 allocs/op
VM/builtin_abs_200k-4  26.00 -> 26.00 allocs/op
```

Only these interpreter paths were measured in that comparison. Real Mongoose
workloads and any further ownership change still need their own comparison.

The next sequential base/head comparison uses the existing
BenchmarkThreadedContinuations, four cohorts (1/16 players, writes false/true),
same machine/toolchain/GOMAXPROCS, 300ms, count=10, with no diagnostics enabled.
Head source is unchanged at documentation commit 95a0a1af. Raw files are
execution-layering-base-continuations.txt and
execution-layering-context-continuations.txt; full benchstat output is
execution-layering-continuations-benchstat.txt. All four timing comparisons
are indistinguishable (p=0.315,0.796,0.190,0.190); allocation-count comparisons
are also indistinguishable (p=0.672,0.274,0.207,0.118). Both runs completed their
terminal-result checks. These synthetic cohorts do not establish real-workload
performance or isolation coverage. Machine isolation/frequency limitations
above still apply.

The researcher skill is active for the unresolved ownership design. Native
agent /root/execution_ownership is assigned only the report
docs/reports/execution-ownership.md under
prompts/research-execution-ownership.md. It may not implement or publish.
Parent must verify each proposed source claim before using or relaying it.

## Surprises

The existing August architecture note is historical: the current builtin
Registry and mutable Session are already split (builtins/session.go:7 and
builtins/registry.go:86). Do not repeat that old proposed split.

## Next step

The implementation/evidence head is 59c21c54. Its build/unit, conformance and
Elle jobs passed (1m5s, 7m25s, 2m35s). Full review is underway against master
11a17d8c; production and test diffs have first-pass ledger coverage, with
documentation/evidence and the second pass remaining. The six moved builtin
integration test bodies compare equal after explicit constructor, dispatch and
suspension relocation; their assertions and signature-wrapper entry are retained.
Smallest remaining prose cleanup: update pool ownership terminology and shorten
old race-test commentary that describes removed unlocked fields as current.
About twenty comment lines removed; test behavior stays unchanged.
Finish that cleanup, then complete current-head review and CI. Review target is PR #542;
review_gate.py requires every PR hunk and two converged passes. No completed
review or merge verdict has been claimed.
The source-based research report describes the design, but is not verification.
Q already
authorized the cleanup; routine implementation choices do not require reapproval.
Do not mistake the green first slice or these two microbenchmarks for completion
of the broader layering goal.
