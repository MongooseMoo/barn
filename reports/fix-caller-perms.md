# Fix caller_perms() Builtin - Report

## Problem

The `caller_perms()` builtin was returning the **current frame's** programmer instead of the **calling frame's** programmer. This broke permission checks in confuncs and other scenarios where code needs to verify the permissions of its caller.

### Example Scenario

Call stack:
- Frame 0: `#0:user_connected` (programmer = #0)
- Frame 1: `#3:confunc` (programmer = #3)

When Frame 1 calls `caller_perms()`, it should return `#0` (the caller's programmer), not `#3` (current frame's programmer).

This is critical for permission checks where a verb needs to validate that its caller has appropriate permissions.

## Root Cause

In `builtins/tasks.go`, the `builtinCallerPerms` function was incorrectly implemented:

```go
func builtinCallerPerms(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 0 {
        return types.Err(types.E_ARGS)
    }

    return types.Ok(types.NewObj(ctx.Programmer))  // BUG: returns current frame
}
```

`ctx.Programmer` holds the **current** frame's programmer, not the caller's.

## The Fix

Updated `builtinCallerPerms` to:
1. Get the task's call stack
2. Check if there are at least 2 frames (need a caller)
3. Return the programmer from `stack[len-2]` (the previous/calling frame)

### Implementation

```go
func builtinCallerPerms(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 0 {
        return types.Err(types.E_ARGS)
    }

    // Get the task from context
    if ctx.Task == nil {
        return types.Ok(types.NewObj(types.NOTHING))
    }

    t, ok := ctx.Task.(*task.Task)
    if !ok {
        return types.Ok(types.NewObj(types.NOTHING))
    }

    // Get the call stack
    stack := t.GetCallStack()

    // Need at least 2 frames to have a caller
    if len(stack) < 2 {
        return types.Ok(types.NewObj(types.NOTHING))
    }

    // Return the programmer of the PREVIOUS frame (the caller)
    // stack[len-1] is current frame, stack[len-2] is caller
    callerFrame := stack[len(stack)-2]
    return types.Ok(types.NewObj(callerFrame.Programmer))
}
```

### Key Points

- Returns `#-1` (NOTHING) if there are fewer than 2 frames
- Accesses `stack[len-2]` to get the **calling** frame (not current frame at `stack[len-1]`)
- Properly handles nil task cases

## Testing

### Test Command

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9300 &
sleep 3
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "look me"
```

### Result

**PASS** - The login sequence completed successfully and displayed "The First Room":

```
Welcome to the ToastCore database.
...
The First Room
This is all there is right now.
...
Wizard
You see a wizard who chooses not to reveal its true appearance.
```

Before the fix, permission checks would fail because confuncs would see the wrong programmer.

### Why This Works

During the login flow:
1. Server calls `#0:do_login_command` (programmer = #0)
2. `do_login_command` calls `#3:confunc` (programmer = #3)
3. Inside `confunc`, when it calls `caller_perms()`:
   - Stack has 2 frames: [#0:do_login_command, #3:confunc]
   - Returns `stack[0].Programmer` = #0
   - Permission check passes (caller is a wizard)

## Files Modified

- `builtins/tasks.go` - Fixed `builtinCallerPerms` function

## Reference

Toast implementation (from `src/execute.cc`):
```c
r.v.obj = activ_stack[top_activ_stack - 1].progr;
```

Returns the `.progr` field of the previous activation frame, which is what Barn now does correctly.
