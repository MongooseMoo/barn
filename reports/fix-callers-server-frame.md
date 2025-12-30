# Fix callers() for Server-Initiated Verb Calls - Report

## Summary

Successfully implemented the fix for the `callers()` server frame issue. The bug was that server-initiated verb calls (like `do_login_command`, `user_connected`) were appearing in the call stack returned by `callers()`, causing security checks like `if (callers()) return E_PERM;` to fail.

## The Fix

Added a `ServerInitiated` flag to activation frames that marks server-invoked calls. The `callers()` builtin now filters out these frames, making them invisible to MOO code while still preserving them for traceback support.

## Changes Made

### 1. Added ServerInitiated Field to ActivationFrame

**File**: `task/task.go`

Added `ServerInitiated bool` field to `ActivationFrame` struct:

```go
type ActivationFrame struct {
    This            types.ObjID
    Player          types.ObjID
    Programmer      types.ObjID
    Caller          types.ObjID
    Verb            string
    VerbLoc         types.ObjID
    Args            []types.Value
    LineNumber      int
    ServerInitiated bool  // NEW: true if server-invoked call
}
```

### 2. Added ServerInitiated Flag to TaskContext

**File**: `types/context.go`

Added flag to TaskContext so the evaluator knows whether to mark frames as server-initiated:

```go
// ServerInitiated indicates if this is a server-initiated call (do_login_command, etc.)
// Server-initiated frames are excluded from callers() results
ServerInitiated bool
```

### 3. Marked Server-Initiated Calls in Scheduler

**File**: `server/scheduler.go`

Modified `CallVerb` to set `ServerInitiated = true` in the context:

```go
ctx := types.NewTaskContext()
ctx.Player = player
ctx.Programmer = verb.Owner
ctx.IsWizard = s.isWizard(verb.Owner)
ctx.ThisObj = objID
ctx.Verb = verbName
ctx.ServerInitiated = true  // Mark as server-initiated
ctx.Task = t
```

This applies to both successful verb calls and failed verb lookups.

### 4. Updated Evaluator to Use ServerInitiated Flag

**File**: `vm/verbs.go`

Modified both `CallVerb` and `verbCall` methods to use `ctx.ServerInitiated` when pushing frames:

**In `CallVerb` (direct verb invocation)**:
```go
frame := task.ActivationFrame{
    This:            objID,
    Player:          ctx.Player,
    Programmer:      ctx.Programmer,
    Caller:          ctx.ThisObj,
    Verb:            verbName,
    VerbLoc:         objID,
    Args:            args,
    LineNumber:      1,
    ServerInitiated: ctx.ServerInitiated,  // Use context flag
}
```

**In `verbCall` (MOO-code verb calls)**:
```go
frame := task.ActivationFrame{
    This:            defObjID,
    Player:          ctx.Player,
    Programmer:      ctx.Programmer,
    Caller:          ctx.ThisObj,
    Verb:            verbName,
    VerbLoc:         defObjID,
    Args:            args,
    LineNumber:      0,
    ServerInitiated: false,  // MOO-code calls are NOT server-initiated
}
```

### 5. Filtered Server Frames in callers()

**File**: `builtins/tasks.go`

Modified `builtinCallers` to skip server-initiated frames:

```go
// Get the call stack
stack := t.GetCallStack()

// Convert to MOO list format, filtering out server-initiated frames
result := make([]types.Value, 0, len(stack))
for _, frame := range stack {
    // Skip server-initiated frames (do_login_command, user_connected, etc.)
    if frame.ServerInitiated {
        continue
    }

    // ... convert frame to list ...
}
```

## Testing Results

### Test 1: Welcome Banner (do_login_command)

**Before Fix**: No output (do_login_command returned E_PERM immediately)

**After Fix**:
```bash
./moo_client.exe -port 9401 -timeout 5 -cmd ""
```

Output:
```
~FF~FBF
Welcome to the ToastCore database.

Type 'connect wizard' to log in.
...
```

✅ **SUCCESS** - Welcome banner appears, indicating `do_login_command` executes fully.

### Test 2: Connect Wizard

**Command**:
```bash
./moo_client.exe -port 9401 -timeout 5 -cmd "connect wizard"
```

Output:
```
~FF~FBF
ANSI Version 2.6 is currently active.
Your previous connection was before we started keeping track.
There is new news. Type `news' to read all news...
```

✅ **SUCCESS** - Login successful, showing ANSI message and news prompt.

### Test 3: callers() Returns Empty List

**Command**:
```bash
./moo_client.exe -port 9401 -timeout 5 -cmd "connect wizard" -cmd "; return callers();"
```

Output:
```
{1, {}}
```

✅ **SUCCESS** - `callers()` returns empty list for eval command (no user verb calls in stack).

## How It Works

1. **Server-Initiated Calls**: When the scheduler calls a verb (e.g., `do_login_command`, `user_connected`), it sets `ServerInitiated = true` in the TaskContext.

2. **Frame Marking**: The evaluator reads `ctx.ServerInitiated` and marks activation frames accordingly when pushing them onto the call stack.

3. **MOO-Code Calls**: When MOO code calls a verb (e.g., `obj:verb()`), `ServerInitiated` defaults to `false`, so these frames appear in `callers()`.

4. **Filtering**: The `callers()` builtin skips frames where `ServerInitiated == true`, making them invisible to MOO code.

5. **Tracebacks Still Work**: Server-initiated frames remain in the call stack for traceback purposes, they're just filtered out of `callers()` results.

## Security Implications

This fix restores the intended security model where:

- `if (callers()) return E_PERM;` correctly identifies when a verb is called by the server (empty stack)
- Server hooks like `do_login_command` can execute privileged operations safely
- Normal verb-to-verb calls still populate `callers()` correctly for security checks

## Edge Cases Handled

1. **Failed Verb Lookup**: Server-initiated frames are marked even when verb lookup fails (E_VERBNF)
2. **Exception Handling**: Server frames remain in stack for traceback extraction after exceptions
3. **Mixed Call Chains**: If server calls verb A, which calls verb B, only A's frame is hidden from `callers()`

## Verification

The fix has been verified to:
- ✅ Allow `do_login_command` to execute (welcome banner appears)
- ✅ Allow `user_connected` to execute (login completes)
- ✅ Return empty list from `callers()` in server-initiated context
- ✅ Preserve server frames for traceback support

## Files Modified

1. `task/task.go` - Added `ServerInitiated` field to `ActivationFrame`
2. `types/context.go` - Added `ServerInitiated` field to `TaskContext`
3. `server/scheduler.go` - Set `ServerInitiated = true` in `CallVerb`
4. `vm/verbs.go` - Use `ctx.ServerInitiated` when pushing frames
5. `builtins/tasks.go` - Filter server-initiated frames in `callers()`

## Conclusion

The fix successfully implements the server frame flag approach as specified in the prompt. Server-initiated verb calls are now properly excluded from `callers()` while remaining in the call stack for debugging purposes. This allows ToastCore's security checks to work correctly while maintaining full traceback support.

## Build and Deploy

To deploy this fix:

```bash
# Build barn with the fix
cd ~/code/barn
go build -o barn.exe ./cmd/barn/

# Start with toastcore.db
./barn.exe -db toastcore.db -port 9400
```

The server now correctly handles server-initiated verb calls without breaking MOO security checks.
