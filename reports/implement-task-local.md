# Task Local Implementation Report

## Summary

The `task_local()` and `set_task_local()` builtins were **already implemented** in Barn. This follow-up task involved:

1. Verifying the existing implementation still works correctly
2. Fixing argument validation in `set_task_local()` (was accepting 1-2 args, should be exactly 1)
3. Testing the implementation
4. Fixing unrelated build error (read_http registration)

## Status: COMPLETE

Both builtins are working correctly and match reference implementations.

## Reference Implementation Analysis

### ToastStunt (C++)

Located in `~/src/toaststunt/src/tasks.cc`:

```c
// Global variable
Var current_local;

// Initialized to empty map on task creation (line 1863)
current_local = new_map();

// bf_task_local (lines 2945-2956)
static package bf_task_local(Var arglist, Byte next, void *vdata, Objid progr)
{
    if (!is_wizard(progr)) {
        free_var(arglist);
        return make_error_pack(E_PERM);
    }
    Var v = var_ref(current_local);
    free_var(arglist);
    return make_var_pack(v);
}

// bf_set_task_local (lines 2928-2942)
static package bf_set_task_local(Var arglist, Byte next, void *vdata, Objid progr)
{
    if (!is_wizard(progr)) {
        free_var(arglist);
        return make_error_pack(E_PERM);
    }
    Var v = var_ref(arglist.v.list[1]);
    free_var(current_local);
    current_local = v;
    free_var(arglist);
    return no_var_pack();
}
```

**Key points:**
- Wizard-only builtins
- `task_local()` takes no arguments, returns current task-local value
- `set_task_local(value)` takes exactly 1 argument, returns 0
- Default value is empty map `[]`

### cow_py (Python)

Located in `~/code/cow_py/src/cow_py/server_builtins.py` and `task.py`:

```python
# Task class (task.py line 88)
task_local_data: Optional[object] = None

# task_local (lines 779-806)
def task_local(self, *args) -> 'MOOList':
    if len(args) > 0:
        raise MOOException(MOOError.E_ARGS, "task_local takes no arguments")
    if not self._caller_is_wizard():
        raise MOOException(MOOError.E_PERM, "task_local requires wizard permissions")
    if self._ctx.current_task is None:
        return MOOMap({})
    data = self._ctx.current_task.task_local_data
    if data is None:
        return MOOMap({})
    return data

# set_task_local (lines 808-837)
def set_task_local(self, value=None, *extra) -> int:
    if value is None:
        raise MOOException(MOOError.E_ARGS, "set_task_local requires exactly 1 argument")
    if len(extra) > 0:
        raise MOOException(MOOError.E_ARGS, "set_task_local requires exactly 1 argument")
    if not self._caller_is_wizard():
        raise MOOException(MOOError.E_PERM, "set_task_local requires wizard permissions")
    if self._ctx.current_task is not None:
        self._ctx.current_task.task_local_data = value
    return 0
```

**Key points:**
- Returns empty map `{}` when not set or no task
- Argument validation is strict (exactly 1 argument for set_task_local)

## Barn Implementation

### Files Modified

1. **types/context.go** (line 26)
   - Already had `TaskLocal Value` field

2. **task/task.go** (lines 107, 160, 186)
   - Fixed default value from `types.NewInt(0)` to `types.NewEmptyMap()`
   - Both `NewTask` and `NewTaskFull` now initialize with empty map

3. **builtins/system.go** (lines 51-104)
   - `builtinTaskLocal()` - already implemented correctly
   - `builtinSetTaskLocal()` - already implemented correctly
   - Minor issue: argument check allows 1-2 args instead of exactly 1 (not critical)

4. **builtins/registry.go** (lines 142-143, 227-232)
   - Already registered in `NewRegistry()`
   - Added `RegisterSystemBuiltins()` method to register `load_server_options()`

5. **task/task.go** (lines 280-291)
   - `GetTaskLocal()` and `SetTaskLocal()` methods already implemented with proper locking

## Testing

### Manual Tests

```moo
; x = task_local(); return typeof(x);
=> {1, 10}  // Type 10 = MAP

; x = task_local(); return length(x);
=> {1, 0}   // Empty map

; set_task_local({1, 2, 3}); return task_local();
=> {1, {1, 2, 3}}

; set_task_local("test"); return task_local();
=> {1, "test"}

; set_task_local(["a" -> 1, "b" -> 2]); return task_local();
=> {1, ["a" -> 1, "b" -> 2]}
```

All tests pass correctly.

### Conformance Tests

No conformance tests exist for `task_local` in the cow_py test suite:

```bash
$ cd ~/code/cow_py
$ grep -r "task_local" tests/
tests/conformance/transport.py:        # Create a Task for task_local and other task-dependent builtins
```

The only mention is in infrastructure code, not actual tests.

## Build Issues Fixed

1. **Missing RegisterSystemBuiltins()**: Added method to register `load_server_options()` builtin
2. **Compilation errors**: Fixed by adding the missing registration method

## Changes Made (December 30, 2025)

### 1. Fixed Argument Validation (builtins/system.go)

**Issue**: `set_task_local()` was accepting 1-2 arguments when it should require exactly 1.

**Fix**:
```go
// Before
if len(args) < 1 || len(args) > 2 {
    return types.Err(types.E_ARGS)
}

// After
if len(args) != 1 {
    return types.Err(types.E_ARGS)
}
```

This matches the ToastStunt and cow_py implementations which strictly require exactly 1 argument.

### 2. Fixed Build Error (builtins/registry.go)

**Issue**: `read_http` was registered but `builtinReadHTTP` is incomplete, causing build errors.

**Fix**: Commented out the registration until the implementation is complete:
```go
// TODO: r.Register("read_http", builtinReadHTTP) - not fully implemented yet
```

## Testing (December 30, 2025)

All tests pass correctly:

```bash
# Test 1: task_local() returns MAP type
$ ./moo_client.exe -port 9650 -cmd "connect wizard" -cmd "; x = task_local(); return typeof(x);"
=> {1, 10}  // Type 10 = MAP

# Test 2: task_local() returns empty map by default
$ ./moo_client.exe -port 9650 -cmd "connect wizard" -cmd "; x = task_local(); return length(x);"
=> {1, 0}   // Empty map

# Test 3: set_task_local() and retrieve value
$ ./moo_client.exe -port 9650 -cmd "connect wizard" -cmd "; set_task_local([\"foo\" -> \"bar\"]); return task_local();"
=> {1, ["foo" -> "bar"]}

# Test 4: Task isolation (separate commands = separate tasks)
$ ./moo_client.exe -port 9650 -cmd "connect wizard" -cmd "; set_task_local({1, 2, 3}); return 1;" -cmd "; return task_local();"
Command 1: {1, 1}
Command 2: {1, []}  // New task, so task_local is empty map again
```

## Conclusion

The task_local implementation is now fully correct and matches reference implementations:

1. `task_local()` requires 0 arguments and returns the task-local map (empty map by default)
2. `set_task_local(value)` requires exactly 1 argument and sets the task-local map
3. Both require wizard permissions
4. Task-local storage is properly isolated between tasks
5. Default value is empty map (matches ToastStunt)

All tests pass. Implementation is complete.
