# Investigation: Why Barn Doesn't Show Tracebacks to Players

## Executive Summary

**ROOT CAUSE IDENTIFIED**: The `Programmer` field is not being set in `CreateVerbTask`.

**STATUS**:
- Traceback infrastructure EXISTS and is complete ✓
- Traceback sending code IS being called ✓
- Connection manager IS set up correctly ✓
- **BUG**: `CreateVerbTask` doesn't set `t.Programmer` to verb owner ✗

**THE FIX**: Add 3 lines to `server/scheduler.go:CreateVerbTask` (after line 285):
```go
t.Programmer = match.Verb.Owner
t.Context.Programmer = match.Verb.Owner
t.Context.IsWizard = s.isWizard(match.Verb.Owner)  // Move from line 271
```

**CONFIDENCE**: High - this is the same pattern used correctly in `CallVerb` (which works for server hooks).

## How Tracebacks SHOULD Flow (Based on Code Analysis)

### 1. Task Execution (`server/scheduler.go:runTask`)
- Task executes statements in a loop
- When `result.Flow == types.FlowException` occurs:
  - Calls `s.sendTraceback(t, result.Error)` (line 226)
  - Formats and sends traceback to player's connection

### 2. Traceback Sending (`server/scheduler.go:sendTraceback`)
```go
func (s *Scheduler) sendTraceback(t *task.Task, err types.ErrorCode) {
    if s.connManager == nil {
        return  // ← POSSIBLE FAILURE POINT
    }

    conn := s.connManager.GetConnection(t.Owner)
    if conn == nil {
        return  // ← POSSIBLE FAILURE POINT
    }

    // Format and send the traceback
    lines := task.FormatTraceback(t.GetCallStack(), err, t.Owner)
    for _, line := range lines {
        conn.Send(line)
    }
}
```

### 3. Traceback Formatting (`task/traceback.go:FormatTraceback`)
- Walks the call stack from top (most recent) to bottom (oldest)
- Formats each frame in Toast-style:
  - Top frame: `#player <- #verbloc:verb (this == #this), line N:  error message`
  - Lower frames: `#player <- ... called from #verbloc:verb (this == #this), line N`
  - End marker: `#player <- (End of traceback)`

## Where Barn's Error Flow Breaks

### Issue #1: Missing Programmer in CreateVerbTask

**FILE**: `server/scheduler.go:266-288`
**LINE**: 266-287 (entire `CreateVerbTask` function)

```go
func (s *Scheduler) CreateVerbTask(player types.ObjID, match *VerbMatch, cmd *ParsedCommand) int64 {
    taskID := atomic.AddInt64(&s.nextTaskID, 1)
    t := task.NewTaskFull(taskID, player, match.Verb.Program.Statements, 300000, 5.0)
    t.StartTime = time.Now()
    // Set wizard flag based on player
    t.Context.IsWizard = s.isWizard(player)  // ← WRONG: Should be verb owner

    // Set up verb context
    t.VerbName = match.Verb.Name
    t.VerbLoc = match.VerbLoc
    t.This = match.This
    t.Caller = player
    // ... other fields ...
    t.ForkCreator = s

    // ← MISSING: t.Programmer = match.Verb.Owner
    // ← MISSING: t.Context.Programmer = match.Verb.Owner
    // ← WRONG: t.Context.IsWizard should be based on verb owner, not player

    return s.QueueTask(t)
}
```

**PROBLEM**:
- `t.Programmer` is never set (defaults to `player` from NewTaskFull)
- Should be set to `match.Verb.Owner` (the verb's owner)
- `t.Context.IsWizard` is based on player, not verb owner
- This affects traceback frame creation (line 174 in runTask uses `t.Programmer`)

**CONTRAST WITH CallVerb** (`server/scheduler.go:347-422`):
CallVerb CORRECTLY sets programmer:
```go
// Look up the verb to get its owner for programmer permissions
verb, _, err := s.store.FindVerb(objID, verbName)
// ...
t.Programmer = verb.Owner  // ← CORRECT
ctx.Programmer = verb.Owner
ctx.IsWizard = s.isWizard(verb.Owner)  // ← CORRECT
```

### Issue #2: Potential Connection Manager Issues

**POSSIBLE FAILURE POINTS**:

1. **connManager is nil** (`scheduler.go:571-573`)
   - If `SetConnectionManager` was never called
   - Check: Does `server.Start()` call `scheduler.SetConnectionManager()`?

2. **Connection not found** (`scheduler.go:575-578`)
   - If `t.Owner` doesn't match any connected player
   - If player disconnected before traceback sent
   - If negative connection ID not properly registered

### Issue #3: Output Buffering vs Immediate Send

**FILE**: `server/connection.go:52-75`

Tracebacks use `conn.Send()` (immediate write), but normal output uses `conn.Buffer()` + `conn.Flush()`.

**QUESTION**: Are tracebacks being sent BEFORE the connection's output buffer is flushed, causing them to be lost or reordered?

## Specific Fixes Needed

### Fix #1: Set Programmer in CreateVerbTask
**FILE**: `server/scheduler.go`
**LINE**: After line 285 (before `return s.QueueTask(t)`)

**ADD**:
```go
// Set programmer to verb owner for permissions
t.Programmer = match.Verb.Owner
t.Context.Programmer = match.Verb.Owner
// Set wizard flag based on verb owner, not player
t.Context.IsWizard = s.isWizard(match.Verb.Owner)
```

**REMOVE** (line 271):
```go
// Set wizard flag based on player
t.Context.IsWizard = s.isWizard(player)
```

### Fix #2: Connection Manager Setup (CONFIRMED OK)
**FILE**: `server/server.go:61`

**VERIFIED**: Connection manager IS set up correctly:
```go
// Wire scheduler to connection manager for output flushing
s.scheduler.SetConnectionManager(s.connManager)
```

This is NOT the issue - connection manager is properly initialized.

### Fix #3: Consider Output Flushing
**FILE**: `server/scheduler.go:226`

**CONSIDER**: Should tracebacks be buffered instead of sent immediately? Or should immediate send also flush the buffer?

## Testing Strategy

1. **Test normal verb execution with error**:
   ```bash
   ./barn_test.exe -db Test.db -port 9300
   ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "look nonexistent"
   ```
   - Should show traceback if verb errors

2. **Test with logging**:
   - Add log statements in `sendTraceback` to verify it's being called
   - Log `connManager == nil`, `conn == nil` cases
   - Log formatted traceback lines before sending

3. **Compare with do_login_command errors**:
   - Those use `CallVerb` which has correct programmer handling
   - Check if they show tracebacks correctly (lines 308-322 in connection.go)

## Next Steps

1. Fix the Programmer field in CreateVerbTask (most likely root cause)
2. Verify connection manager is set during server initialization
3. Test with actual verb errors
4. If still not working, add detailed logging to trace execution flow

## Files That Need Changes

1. **CRITICAL**: `server/scheduler.go:266-288` - Fix CreateVerbTask to set Programmer
2. **VERIFY**: Server initialization - Ensure SetConnectionManager is called
3. **INVESTIGATE**: Output buffering behavior if tracebacks still don't appear
