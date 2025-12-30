# Task: Add Call Stack Tracking to scheduler.CallVerb

## Context
When hook verbs like `user_connected` fail with exceptions, the traceback shows "(no stack)" because `scheduler.CallVerb` doesn't track activation frames. This makes debugging difficult.

Currently in `server/connection.go`:
```go
cm.sendTracebackToPlayer(player, result.Error, nil)  // nil = no stack!
```

## Objective
Modify `scheduler.CallVerb` to track call stack (activation frames) so that when exceptions occur, full tracebacks can be sent to users.

## Research Required
1. Look at how `scheduler.runTask` pushes activation frames (line ~169 in scheduler.go)
2. Look at `task.ActivationFrame` structure in `task/task.go`
3. Understand how `vm.Evaluator.CallVerb` works and where exceptions propagate

## The Problem
`scheduler.CallVerb` (line ~342 in scheduler.go) is used for synchronous hook calls:
```go
func (s *Scheduler) CallVerb(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) types.Result {
    // Creates context but NOT a task
    // No activation frames are tracked
    return s.evaluator.CallVerb(objID, verbName, args, ctx)
}
```

When verbs called this way raise exceptions, there's no call stack to format.

## Approach Options

### Option A: Track stack in Evaluator
Have `vm.Evaluator` maintain its own call stack during verb execution, return it with Result.

### Option B: Create lightweight Task for hook calls
Create a Task object just for tracking the call stack, even for synchronous execution.

### Option C: Add CallStack to Result type
Add a `CallStack []task.ActivationFrame` field to `types.Result` (but beware circular imports - task imports types).

## Files to Modify
- `server/scheduler.go` - `CallVerb` method
- `vm/evaluator.go` or `vm/verbs.go` - wherever verb calls happen
- `types/result.go` - possibly add call stack field
- `server/connection.go` - update to use the call stack from result

## Test
After fix:
1. Start barn with Test.db (which has no user_connected verb)
2. Connect as wizard
3. User should see a proper traceback like:
   ```
   #8 <- #0:user_connected (this == #0), line 1:  Verb not found
   #8 <- (End of traceback)
   ```
   Instead of:
   ```
   #8 <- (no stack):  Verb not found
   #8 <- (End of traceback)
   ```

## Output
Write findings/status to `./reports/fix-callverb-callstack.md` when done.
