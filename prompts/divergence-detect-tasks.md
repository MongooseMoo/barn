# Task: Detect Divergences in Task Builtins

## Context

We need to verify Barn's task builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all task builtins.

## Files to Read

- `spec/builtins/tasks.md` - expected behavior specification
- `builtins/tasks.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Task Queries
- `task_id()` - current task ID
- `queued_tasks()` - list waiting tasks
- `task_stack()` - get task call stack
- `callers()` - get current call stack
- `caller_perms()` - get calling task permissions

### Task Control
- `suspend()` - suspend current task
- `resume()` - resume suspended task
- `kill_task()` - terminate task
- `queue_info()` - ToastStunt queue stats

### Forking
- `fork` statement - create background task
- `fork()` - task ID of forked task

### Limits
- `set_task_local()` - per-task storage
- `task_local()` - read per-task storage
- `ticks_left()` - remaining ticks
- `seconds_left()` - remaining time

## Edge Cases to Test

- Invalid task IDs
- Permission violations on task control
- Nested fork behavior
- Task limits (ticks, seconds)
- Suspended task queries
- caller_perms() in different contexts

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'task_id()'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return task_id();"

# Check conformance tests
grep -r "task_id\|queued_tasks\|suspend\|resume\|kill_task\|callers" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-tasks.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major task builtin
- Pay special attention to task isolation and permissions
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
