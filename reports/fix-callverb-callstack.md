# Fix CallVerb Call Stack Tracking

## Problem

When hook verbs like `user_connected` failed with exceptions, the traceback showed "(no stack)" because `scheduler.CallVerb` didn't track activation frames. This made debugging difficult.

Example of broken output:
```
#8 <- (no stack):  Verb not found
#8 <- (End of traceback)
```

## Root Cause

1. `scheduler.CallVerb` (used for synchronous hook calls like `user_connected`, `do_login_command`, etc.) created a `TaskContext` but NOT a `task.Task`
2. Without a Task, `vm.Evaluator.CallVerb` had no place to push activation frames
3. When exceptions occurred, there was no call stack to format

## Solution Implemented

### 1. Extended `types.Result` Structure

Added a `CallStack` field to `types.Result` to carry call stack information from synchronous verb calls:

```go
type Result struct {
    Val       Value
    Flow      ControlFlow
    Error     ErrorCode
    Label     string
    ForkInfo  *ForkInfo
    CallStack interface{} // []task.ActivationFrame - only set on exception
}
```

Used `interface{}` to avoid circular import (types → task).

### 2. Modified `scheduler.CallVerb` to Create Lightweight Task

Created a minimal `task.Task` just for call stack tracking, even for synchronous execution:

```go
func (s *Scheduler) CallVerb(...) types.Result {
    // Create lightweight task FIRST for call stack tracking
    t := &task.Task{
        Owner:      player,
        Programmer: player,
        CallStack:  make([]task.ActivationFrame, 0),
    }

    // ... verb lookup ...

    ctx := types.NewTaskContext()
    ctx.Task = t  // Attach task so CallVerb can track frames

    result := s.evaluator.CallVerb(objID, verbName, args, ctx)

    // Extract call stack BEFORE popping frames
    if result.Flow == types.FlowException {
        result.CallStack = t.GetCallStack()
    }

    // Clean up call stack
    if len(t.CallStack) > 0 {
        t.PopFrame()
    }

    return result
}
```

Key insight: Create the task BEFORE verb lookup, so even E_VERBNF errors get proper tracebacks.

### 3. Modified `vm.Evaluator.CallVerb` Frame Management

Changed from automatic `defer t.PopFrame()` to manual frame management:

```go
func (e *Evaluator) CallVerb(...) types.Result {
    // Push frame EARLY, before verb lookup
    // NOTE: We do NOT use defer PopFrame() because we want to keep
    // the frame on error so scheduler can extract call stack
    if ctx.Task != nil {
        if t, ok := ctx.Task.(*task.Task); ok {
            frame := task.ActivationFrame{
                This:       objID,
                Player:     ctx.Player,
                Programmer: ctx.Programmer,
                Caller:     ctx.ThisObj,
                Verb:       verbName,
                VerbLoc:    objID,
                Args:       args,
                LineNumber: 1,
            }
            t.PushFrame(frame)
            framePushed = true
        }
    }

    // ... verb execution ...

    // Don't pop frame - let scheduler do it after extracting stack
}
```

This ensures frames remain on the stack when errors occur, allowing the scheduler to extract them for tracebacks.

### 4. Updated All Hook Call Sites

Updated `server/connection.go` to extract and use call stacks:

```go
func (cm *ConnectionManager) callUserConnected(player types.ObjID) {
    result := cm.server.scheduler.CallVerb(0, "user_connected", args, player)
    if result.Flow == types.FlowException {
        // Extract call stack from result
        var stack []task.ActivationFrame
        if result.CallStack != nil {
            if s, ok := result.CallStack.([]task.ActivationFrame); ok {
                stack = s
            }
        }
        cm.sendTracebackToPlayer(player, result.Error, stack)
    }
}
```

Applied to:
- `callUserConnected`
- `callUserReconnected`
- `callUserDisconnected`
- `callDoLoginCommand`

## Result

After fix, hook verb errors show proper tracebacks:

```
#8 <- #0:user_connected (this == #0), line 1:  Verb not found
#8 <- (End of traceback)
```

The traceback now shows:
- Which verb was called (#0:user_connected)
- What object it was called on (this == #0)
- The line number where the error occurred (line 1)
- The error message (Verb not found)

## Files Modified

1. **types/result.go** - Added `CallStack interface{}` field
2. **server/scheduler.go** - Modified `CallVerb` to create task and extract call stack
3. **vm/verbs.go** - Modified `CallVerb` to push frames without defer popping
4. **server/connection.go** - Updated all hook call sites to extract and use call stacks

## Testing

Tested by connecting to server with missing `user_connected` verb:

```bash
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9480 &
./moo_client.exe -port 9480 -cmd "connect wizard"
```

Output now shows proper traceback instead of "(no stack)".

## Design Notes

**Why not add CallStack to Task struct directly?**
- Task already has `CallStack []ActivationFrame` field
- The issue was that synchronous hooks didn't create Tasks at all
- Solution: Create minimal Tasks just for stack tracking

**Why use interface{} instead of []task.ActivationFrame?**
- Avoids circular import: types → task → types
- Safe because we only set it in one place (scheduler)
- Type assertion at extraction point is straightforward

**Why not use defer for PopFrame?**
- `defer` pops BEFORE the result is returned
- We need to extract the call stack from the result AFTER CallVerb returns
- Solution: Manual frame management, pop after extracting stack

## Alternative Approaches Considered

1. **Track stack in Evaluator** - Would require passing stack back through all evaluator methods
2. **Create full Task** - Overkill; we only need call stack tracking, not full task infrastructure
3. **Add CallStack to Result at all return points** - Would pollute normal execution with stack tracking

The lightweight Task approach was cleanest and leveraged existing infrastructure.
