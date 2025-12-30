# Debug Report: Why confunc Permission Check Fails in Barn

## Summary

The `caller_perms()` builtin incorrectly returns the **current frame's** programmer instead of the **previous frame's** programmer. This causes permission checks in server hooks to fail.

## Root Cause Analysis

### The Call Flow

1. **Connection.callUserConnected** (connection.go:541-543)
   - Calls `scheduler.CallVerb(0, "user_connected", args, player)` where player = wizard

2. **Scheduler.CallVerb** (scheduler.go:350-422)
   - Line 395: Sets `ctx.Programmer = verb.Owner`
   - For `#0:user_connected`, verb.Owner is #0
   - Creates task with `t.Programmer = verb.Owner` (#0)
   - Calls evaluator with this context

3. **Inside user_connected** (MOO code)
   - Frame: {This: #0, Programmer: #0, Player: wizard, Caller: #-1}
   - Calls `user.location:confunc(user)`
   - This creates a new frame

4. **Inside confunc** (MOO code on #3)
   - Frame: {This: #3, Programmer: #3, Player: wizard, Caller: #0}
   - Calls `caller_perms()`

### The Bug

**Barn's implementation** (builtins/tasks.go:150-156):
```go
func builtinCallerPerms(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 0 {
        return types.Err(types.E_ARGS)
    }
    return types.Ok(types.NewObj(ctx.Programmer))  // <-- WRONG: Returns CURRENT programmer
}
```

This returns `ctx.Programmer` which is #3 (the confunc verb's owner).

**ToastStunt's implementation** (src/execute.cc):
```c
bf_caller_perms(Var arglist, Byte next, void *vdata, Objid progr)
{   /* () */
    Var r;
    r.type = TYPE_OBJ;
    if (top_activ_stack == 0)
        r.v.obj = NOTHING;
    else
        r.v.obj = activ_stack[top_activ_stack - 1].progr;  // <-- CORRECT: Previous frame
    free_var(arglist);
    return make_var_pack(r);
}
```

This returns the **previous activation frame's programmer**.

### The Permission Check

In `#3:confunc` (line 1):
```moo
if ((((cp = caller_perms()) == player) || $perm_utils:controls(cp, player)) || (caller == this))
```

This checks:
1. `caller_perms() == player` - Does the calling verb have the player's permissions?
2. `$perm_utils:controls(cp, player)` - Does the caller control the player?
3. `caller == this` - Is the caller the room itself?

**What should happen (Toast):**
- `caller_perms()` returns #0 (previous frame's programmer)
- `player` is wizard
- Check fails (#0 != wizard)
- But #0 controls wizard (as system object), so second check passes

**What actually happens (Barn):**
- `caller_perms()` returns #3 (current frame's programmer)
- `player` is wizard
- Check fails (#3 != wizard)
- #3 (generic room) doesn't control wizard
- All checks fail → permission denied

## The Fix

**File: `builtins/tasks.go`, function `builtinCallerPerms`**

Change from returning current programmer to returning **previous frame's programmer**:

```go
func builtinCallerPerms(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 0 {
        return types.Err(types.E_ARGS)
    }

    // caller_perms() returns the programmer of the PREVIOUS activation frame
    // (the verb that called the current verb)
    if ctx.Task == nil {
        return types.Ok(types.NewObj(types.ObjNothing))
    }

    t, ok := ctx.Task.(*task.Task)
    if !ok {
        return types.Ok(types.NewObj(types.ObjNothing))
    }

    stack := t.GetCallStack()
    if len(stack) < 2 {
        // If we're at the top level or only one frame deep, return NOTHING
        return types.Ok(types.NewObj(types.ObjNothing))
    }

    // Return the programmer of the frame BEFORE the current one
    // stack[len-1] = current frame
    // stack[len-2] = previous frame (the caller)
    prevFrame := stack[len(stack)-2]
    return types.Ok(types.NewObj(prevFrame.Programmer))
}
```

## Verification

After the fix, the call flow would be:

1. In `confunc`, call stack has:
   - stack[0]: {Programmer: #0} (user_connected)
   - stack[1]: {Programmer: #3} (confunc - current)

2. `caller_perms()` returns `stack[0].Programmer` = #0

3. Permission check:
   - `caller_perms() == player` → #0 == wizard → False
   - `$perm_utils:controls(#0, wizard)` → True (system controls wizard)
   - Check passes!

## Additional Notes

### Why This Matters for Server Hooks

Server hooks like `user_connected` run with the verb owner's permissions (typically #0). When they call other verbs, those verbs need to see that the **caller** has #0's permissions, not the current verb's permissions.

This is a fundamental security model in MOO:
- `caller_perms()` tells you **who wrote the code that called you**
- This is used to grant special permissions to system-initiated calls

### Related Builtins

The `caller` variable is different:
- `caller` is the **object** that called the current verb (stored in `Caller` frame field)
- `caller_perms()` is the **programmer** of the calling verb

Both are based on the **previous frame**, not the current one.
