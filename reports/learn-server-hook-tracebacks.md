# Investigation: Why Server Hooks Don't Show Tracebacks

## Summary

**GOOD NEWS**: The traceback sending code DOES exist for both player commands and server hooks. Both code paths are nearly identical and both should work.

**THE MYSTERY**: If tracebacks aren't appearing for server hooks, it's NOT because the code is missing - it's because either:
1. The call stack is empty when we try to format it
2. The exception isn't being caught (it's happening inside a try/except block in MOO code)
3. There's a timing issue with connection state
4. The connection is gone by the time we try to send

## Player Commands (WORKING PATH)

### Flow:
1. **Entry**: `connection.go:dispatchCommand()` (line 399)
2. **Task Creation**: `scheduler.go:CreateVerbTask()` (line 266)
   - Creates a Task with code, owner, verb context
   - Queues task for async execution
3. **Execution**: `scheduler.go:runTask()` (line 127)
   - Line 171-179: **PUSHES initial activation frame** to task.CallStack
   - Line 204: Executes statements via `evaluator.EvalStmt()`
   - Line 222-235: If exception occurs:
     - Line 224: Sets task state to TaskKilled
     - Line 226: **Calls `s.sendTraceback(t, result.Error)`** ✓
     - Line 228-230: Cleans up call stack
4. **Traceback Sending**: `scheduler.go:sendTraceback()` (line 570)
   - Line 575: Gets connection via `connManager.GetConnection(t.Owner)`
   - Line 581-584: Formats traceback and sends each line via `conn.Send(line)` ✓
5. **Output Flushing**: `scheduler.go:processReadyTasks()` (line 119)
   - After task completes, calls `conn.Flush()` ✓

## Server Hooks (POTENTIALLY BROKEN PATH)

### Flow for `user_connected`:
1. **Entry**: `connection.go:callUserConnected()` (line 533)
2. **Verb Execution**: `scheduler.go:CallVerb()` (line 350)
   - Line 356-361: **Creates lightweight Task** with empty CallStack
   - Line 400: Sets `ctx.Task = t` so VM can push frames
   - Line 402: Calls `evaluator.CallVerb()` **synchronously**
3. **VM Execution**: `vm/verbs.go:CallVerb()` (line 216)
   - Line 228-244: **PUSHES activation frame** to ctx.Task.CallStack ✓
   - Line 306: Executes verb via `e.statements()`
   - If exception: returns Result with FlowException
4. **Exception Handling**: `scheduler.go:CallVerb()` (line 406-409)
   - Line 407: **Extracts call stack** from task: `result.CallStack = t.GetCallStack()` ✓
   - Line 409: Traces exception to log
   - Line 417-419: Pops frames (cleanup)
5. **Back to Caller**: `connection.go:callUserConnected()` (line 536-545)
   - Line 537: Logs error to server log: `log.Printf("user_connected error: %v", result.Error)`
   - Line 538-544: **Extracts call stack from result**
   - Line 545: **Calls `cm.sendTracebackToPlayer(player, result.Error, stack)`** ✓
6. **Traceback Sending**: `connection.go:sendTracebackToPlayer()` (line 519)
   - Line 520: Gets connection via `cm.GetConnection(player)`
   - Line 526-529: Formats traceback and sends each line via `conn.Send(line)` ✓

### Flow for `do_login_command`:
1. **Entry**: `connection.go:callDoLoginCommand()` (line 284)
2. **Verb Execution**: `scheduler.go:CallVerb()` (same as above)
3. **Exception Handling**: `connection.go:callDoLoginCommand()` (line 309-322)
   - Line 312-315: **Extracts call stack from result**
   - Line 318-321: **Formats and sends traceback directly** via `conn.Send(line)` ✓
   - Line 322: Returns (-1, nil) to indicate login failed

## Key Observations

### Both Paths Are Nearly Identical!

| Feature | Player Commands | Server Hooks |
|---------|----------------|--------------|
| Push activation frame? | ✓ Yes (line 171) | ✓ Yes (vm/verbs.go:241) |
| Extract call stack? | ✓ Yes (in task) | ✓ Yes (scheduler.go:407) |
| Format traceback? | ✓ Yes | ✓ Yes |
| Send via conn.Send()? | ✓ Yes | ✓ Yes |

### The ONE Difference: Output Buffering

**Player commands**:
- After `runTask()` completes, `processReadyTasks()` calls `conn.Flush()` (line 119)
- But `sendTraceback()` uses `conn.Send()` which writes immediately, not to buffer!

**Server hooks**:
- No flush call after hook execution
- But tracebacks also use `conn.Send()` which writes immediately!

**Conclusion**: Output buffering is NOT the issue since both use `conn.Send()` for immediate output.

## The REAL Differences (Subtle)

### 1. Call Stack Depth

**Player commands**: Start with 1 frame already on stack (pushed at runTask:171)
- When error occurs, stack has at least 1 frame

**Server hooks**: Start with 0 frames on stack (scheduler.go:359 creates empty stack)
- Frames only pushed when VM executes (vm/verbs.go:241)
- **If error occurs BEFORE verb execution** (e.g., verb lookup fails), stack is empty!

### 2. Connection Timing

**`do_login_command`**:
- Called with `connID` = negative connection ID (line 297)
- Connection exists but player not logged in yet
- Should work fine

**`user_connected`/`user_reconnected`/`user_disconnected`**:
- Called with actual player ID
- Connection should be mapped to player
- **BUT**: What if mapping isn't set up yet when `user_connected` is called?

### 3. Exception Handling in MOO Code

If the MOO verb itself has try/except blocks:
```
try
    // code that errors
except e (ANY)
    // error is caught, never propagates to Go
endtry
```

Then no exception reaches the Go code, so no traceback is sent!

## Evidence from Logs

From `barn_debug.log`:
```
2025/12/29 19:09:46 user_disconnected error: E_PERM
```

This shows:
- Exception WAS caught (logged to server)
- Error was E_PERM (permission denied)
- **But was traceback sent to player?** Player already disconnected at this point!

## Root Cause Hypothesis

### Most Likely: Exceptions Caught in MOO Code

Looking at `do_login_command` verb (lines 22-29):
```moo
try
    newargs = $http:handle_connection(@args);
    if (!newargs)
        return 0;
    endif
    args = newargs;
except v (ANY)
    // Silently catches ALL exceptions!
endtry
```

**If an error occurs inside `$http:handle_connection()`, it's CAUGHT by the try/except and never propagates to Go!**

This is the MOO verb's fault, not Barn's code.

### Less Likely: Connection Not Found

If `GetConnection(player)` returns nil (line 520 in sendTracebackToPlayer), the traceback silently fails.

**When could this happen?**
- During `user_disconnected`: Player already gone!
- During `user_connected`: Player mapping not set up yet?
- During `do_login_command`: Using negative connection ID might not be mapped correctly?

## What Needs to Change

### Option 1: Nothing (This is MOO code's problem)

If exceptions are being caught by try/except blocks in MOO verbs, that's **intentional behavior**. The MOO code is handling errors, so no traceback should appear.

**To verify**: Test with a verb that deliberately throws an error NOT inside a try/except.

### Option 2: Better Diagnostics

Even if connection is gone or not found, log the traceback to server log:

```go
// sendTracebackToPlayer
conn := cm.GetConnection(player)
lines := task.FormatTraceback(stack, err, player)

if conn == nil {
    // Connection gone - log to server instead
    log.Printf("Traceback for player %d (connection not found):", player)
    for _, line := range lines {
        log.Printf("  %s", line)
    }
    return
}

// Send to player
for _, line := range lines {
    conn.Send(line)
}
```

### Option 3: Ensure Connection Mapping Before Hooks

Make sure `user_connected` is called AFTER the player-to-connection mapping is fully established.

**Current code** (connection.go:367-369):
```go
if !alreadyLoggedIn {
    conn.SetPlayer(player)  // Line 368
    cm.playerConns[player] = conn  // Line 369
}
cm.mu.Unlock()  // Line 372
// ...
cm.callUserConnected(player)  // Line 393
```

Mapping IS established before calling the hook. ✓

## Exact Code Locations

### Where tracebacks are sent:

1. **Player commands**: `scheduler.go:226`
   ```go
   s.sendTraceback(t, result.Error)
   ```

2. **do_login_command**: `connection.go:318-321`
   ```go
   lines := task.FormatTraceback(stack, result.Error, connID)
   for _, line := range lines {
       conn.Send(line)
   }
   ```

3. **user_connected/reconnected/disconnected**: `connection.go:545, 562, 579`
   ```go
   cm.sendTracebackToPlayer(player, result.Error, stack)
   ```

### Where they diverge:

**NONE** - both paths use the exact same mechanism:
1. Extract call stack from task or result
2. Format via `task.FormatTraceback()`
3. Send via `conn.Send()` (immediate output)

## Recommendations for FIXER Agent

1. **First**: Verify the problem exists
   - Create a test verb that throws an error NOT inside try/except
   - Call it from `user_connected` hook
   - Check if traceback appears

2. **If tracebacks are missing**: Add server log fallback
   - Location: `connection.go:sendTracebackToPlayer()` (line 519)
   - Change: Log to server if connection not found

3. **If call stacks are empty**: Investigate frame pushing
   - Check if `ctx.Task` is nil when VM executes
   - Check if frames are being popped too early

4. **If timing issue**: Add connection state checks
   - Verify `GetConnection()` finds the connection
   - Add logging when connection is nil

## Test Cases for FIXER

```moo
// Test 1: Error in user_connected (not caught)
@verb #0:user_connected tnt rxd
@program #0:user_connected
raise(E_INVARG, "Test error in user_connected");
.

// Test 2: Error in nested verb call
@verb #0:test_nested tnt rxd
@program #0:test_nested
x = this.nonexistent_property;
.

@verb #0:user_connected tnt rxd
@program #0:user_connected
this:test_nested();
.

// Expected: Both should show tracebacks to player
```
