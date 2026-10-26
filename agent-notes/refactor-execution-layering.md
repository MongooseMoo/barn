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
callers now use task.Context and task.NewContext. No tests or performance
comparisons run yet.
PR #542 contains the first commit, 0d5c074f. CI runs 40284826132 and 40284826145
failed compilation on the alias-replacement defect described below. The field
selector repair is implemented and awaiting the next CI run. No merge or release
for this task yet. The original checkout has unrelated
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

## Surprises

The existing August architecture note is historical: the current builtin
Registry and mutable Session are already split (builtins/session.go:7 and
builtins/registry.go:86). Do not repeat that old proposed split.

## Next step

Commit/push the selector repair and read the new PR CI result. Continue tracing task/VM lifetime and snapshot ownership for the
second slice while CI runs. Overall completion remains unproven.
