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
callers now use task.Context and task.NewContext. PR #542 source head is
6e54bba7 (context move 0d5c074f plus selector repair).
CI run 40285084528 reports Build & unit tests pass (1m9s), MOO conformance suite
pass (7m38s); run 40285084536 reports Serializable histories and negative control
pass (3m1s). The Elle artifact is 12633877005, named
elle-evidence-0512c32f674e733789e50a17a1098226ffbfbe40; it has not been downloaded
or inspected, so this records the job result, not a separate isolation audit.
Earlier runs 40284826132 and 40284826145 failed on the repaired alias defect.
No merge or release for this task yet. The original checkout has unrelated
untracked work; it is not this task's edit target.

## Decisions and why

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
The exact construction and cross-package locking design is still unresolved;
no second-slice production code has been written.

Builtin test dependencies also matter: builtins/task_manager_test.go wires the
current task.Manager; several authority/completion/IO tests depend on it.
Moving that manager above builtins cannot be patched by importing engine from
same-package builtin tests. Preserve those behavioral tests using the final
public boundary, not a duplicate production manager in test scaffolding.

## Tooling and verification

Use go list for current dependencies and gopls semantic rename previews for
identifier edits, including import aliases. The first patch incorrectly used
plain text replacement for task-package qualifiers; that also changed field
selectors such as wake.task.Resume to wake.taskpkg.Resume. Replace that
substitution approach with symbol-aware previews; restore the affected field
selectors directly. CI run 40284826132 reported network.go:631,684,717,757;
the same replacement also affected two engine test files.
Apply all source/note changes through apply_patch. gopls, gorename, and
benchstat are installed. Q said to choose tooling; no WOPR dependency remains.
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

Only these paths were measured. Continuation cohorts, real Mongoose workloads,
and any further ownership change still need their own comparison.

## Surprises

The existing August architecture note is historical: the current builtin
Registry and mutable Session are already split (builtins/session.go:7 and
builtins/registry.go:86). Do not repeat that old proposed split.

## Next step

Commit the preliminary measurement evidence and this source-backed design
record. Resolve the executable-task/state split's construction and lock ownership,
write its smallest exact move here, then implement the second slice. Q already
authorized the cleanup; routine implementation choices do not require reapproval.
Do not mistake the green first slice or these two microbenchmarks for completion
of the broader layering goal.
