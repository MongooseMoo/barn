# Task: Implement task_local() Builtin

## Context
Barn is a Go MOO server. `task_local()` provides per-task storage that persists across verb calls within a single task but is isolated between tasks.

## Objective
Implement `task_local()` builtin for per-task variable storage.

## How task_local() Works
```moo
task_local()           -> returns the task-local map for current task
task_local("key")      -> returns value for "key", or E_RANGE if not set
task_local("key", val) -> sets "key" to val, returns old value or 0
```

Task-local storage:
- Each task has its own map
- Persists across verb calls within the same task
- Survives suspend/resume
- Cleared when task ends

## Reference Implementations

### ToastStunt (C++)
Location: `~/src/toaststunt/`
- Search for `bf_task_local`
- Check how tasks store their local data

### cow_py (Python)
Location: `~/code/cow_py/`
- Check `src/moo_interp/builtins/` for task_local
- Look at Task class for storage mechanism

## Implementation Requirements

1. **Add task-local storage** to TaskContext:
   ```go
   type TaskContext struct {
       // ... existing fields
       TaskLocal map[string]Value  // Per-task storage
   }
   ```

2. **Implement builtinTaskLocal**:
   ```go
   func builtinTaskLocal(ctx *types.TaskContext, args []types.Value) types.Result {
       switch len(args) {
       case 0:
           // Return entire map
       case 1:
           // Get value for key
       case 2:
           // Set value for key, return old
       default:
           return types.Err(types.E_ARGS)
       }
   }
   ```

3. **Register** in `builtins/registry.go`

## Files to Modify
- `C:\Users\Q\code\barn\types\context.go` - add TaskLocal field
- `C:\Users\Q\code\barn\builtins\tasks.go` - implement builtin
- `C:\Users\Q\code\barn\builtins\registry.go` - register builtin

## Test Command
```bash
cd /c/Users/Q/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9650 -v -k "task_local" 2>&1
```

## Output
Write findings and implementation status to `./reports/implement-task-local.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
