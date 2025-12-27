# Task: Research MOO task/fork implementations

## Objective

Study how MOO tasks and the `fork` statement are implemented in:
1. cow_py (Python MOO server) - `~/code/cow_py/`
2. ToastStunt (C++ MOO server) - `~/src/toaststunt/`

## What to research

### 1. Fork statement semantics
- What does `fork (delay) ... endfork` do?
- What does `fork name (delay) ... endfork` do (named fork)?
- What is the fork ID and how is it used?
- How does the delay work?
- What happens to the parent task after fork?

### 2. Task model
- What is a task? (unit of execution)
- How are tasks scheduled?
- How do tasks relate to ticks/seconds limits?
- What happens when a task exceeds limits?
- How does suspend/resume work?

### 3. Forked task execution
- When does the forked code run?
- What context does it inherit? (variables, this, player, etc.)
- How are forked tasks tracked?
- How can forked tasks be killed?

### 4. Implementation details
- Data structures used
- How fork creates a new execution context
- How the scheduler runs forked tasks
- How queued_tasks() works with forks

## Files to examine

### cow_py (~\code\cow_py\)
- Look for fork handling in interpreter/evaluator
- Look for task/scheduler implementation
- Look for AST nodes related to fork

### ToastStunt (~\src\toaststunt\)
- `execute.cc` - likely has fork execution
- `tasks.cc` / `tasks.h` - task management
- `eval.cc` - evaluation
- `parse.y` / `ast.cc` - fork parsing

## Output

Write a comprehensive report to `./reports/research-task-fork.md` with:
1. Fork semantics summary
2. cow_py implementation details
3. ToastStunt implementation details
4. Key design decisions and tradeoffs
5. Recommendations for barn implementation

Be thorough - this is critical infrastructure.
