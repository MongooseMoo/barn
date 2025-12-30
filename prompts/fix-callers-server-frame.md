# Task: Fix callers() for Server-Initiated Verb Calls

## The Bug

`do_login_command` in toastcore.db has:
```moo
if (callers())
  return E_PERM;
endif
```

This security check ensures the verb is only called as a server task (top-level).

**Problem:** Barn's `scheduler.CallVerb` pushes an activation frame for traceback support, so `callers()` returns a non-empty list, and the verb immediately returns E_PERM.

**Expected:** Server-initiated calls (like `do_login_command`, `user_connected`, etc.) should NOT appear in `callers()` results.

## The Fix (Option B: Server Frame Flag)

Add a flag to activation frames that marks them as "server-initiated". The `callers()` builtin should filter these out.

### Files to Modify

#### 1. `task/task.go` - Add ServerInitiated flag to ActivationFrame

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
    ServerInitiated bool  // NEW: true if this is a server-invoked call (do_login_command, user_connected, etc.)
}
```

#### 2. `server/scheduler.go` - Mark server-initiated calls

In `CallVerb`, when pushing the frame, set `ServerInitiated: true`:

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
    ServerInitiated: true,  // This is a server-initiated call
}
```

#### 3. `builtins/tasks.go` - Filter server frames in callers()

Find `builtinCallers` and modify it to skip ServerInitiated frames:

```go
func builtinCallers(ctx *types.TaskContext, args []types.Value) types.Result {
    if ctx.Task == nil {
        return types.Ok(types.NewList([]types.Value{}))
    }

    t, ok := ctx.Task.(*task.Task)
    if !ok {
        return types.Ok(types.NewList([]types.Value{}))
    }

    stack := t.GetCallStack()
    var callers []types.Value

    // Skip first frame (current verb) and filter server-initiated frames
    for i := 1; i < len(stack); i++ {
        frame := stack[i]
        if frame.ServerInitiated {
            continue  // Don't include server-initiated frames in callers()
        }
        // ... build caller info ...
    }

    return types.Ok(types.NewList(callers))
}
```

#### 4. `vm/verbs.go` - Ensure verb-to-verb calls are NOT server-initiated

In `verbCall` (the method that handles `expr:verb(args)` calls from MOO code), ensure frames are pushed with `ServerInitiated: false` (the default).

## Test

After the fix:
```bash
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore.db -port 9400 &
sleep 2
./moo_client.exe -port 9400 -timeout 5 -cmd ""
```

Should now show the ToastCore welcome banner.

## Verification

1. `do_login_command` should execute fully (not return E_PERM)
2. `notify()` should send the welcome banner
3. `connect wizard` should create a player and log in
4. Tracebacks should still work (server frames ARE in the call stack, just filtered from callers())

## Output

Write findings to `./reports/fix-callers-server-frame.md` and implement the fix.

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
