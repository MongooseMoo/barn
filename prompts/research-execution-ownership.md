# Research: concrete task and VM ownership

## Question

What is the smallest principled restructuring that eliminates task.Task's
BytecodeVM interface{} and builtins.Execution.CallingVM any, without a generic
VM facade, compatibility aliases, extra task allocation, new lock, or redundant
VM registry? Establish a concrete implementable design, not a list of options.

## Scope

Worktree C:/Users/Q/code/barn-execution-layering, branch
refactor/execution-layering. Read agent-notes/refactor-execution-layering.md for
the authorized goal and prior evidence. Q authorized implementation with
"Make it so!"; this assignment is read-only research plus the report.
Read task, vm, builtins execution/host/session interfaces, engine lifecycle,
scheduler and snapshot callers. Trace actual state and synchronization, including
unregistered direct tasks, fork setup, retry, and GC. Determine which existing
owner can hold a concrete VM, which lower state must remain shared, and how
construction retains one task allocation and the existing lock order.

## Output

Write docs/reports/execution-ownership.md using apply_patch. Give the exact file
and symbol moves, resulting dependency graph, concrete constructor and locking
design, and realistic patch size. State every unresolved issue. Evaluate whether
a simpler solution exists than splitting engine.Task from lower task.State.
Read source independently; earlier notes are leads, not verified facts.

## Constraints

Do not implement source changes, run tests/builds, commit, push, publish, or
spawn additional agents. You are not alone in this worktree; do not revert or
modify others' work. Own only this report. No new tests or guards. Q's AGENTS.md
rules apply. Never replace the saved live VM with copied persistence snapshots
to evade the dependency; performance and continuation semantics matter.
