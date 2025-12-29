# Fix Call Stack Tracking for Verb-to-Verb Calls

## Problem

When a verb called another verb in Barn, the traceback only showed 1 frame instead of the full call chain.

**Before (Barn):**
```
#2 <- #6:news (this == #2), line 45:  Range error
#2 <- (End of traceback)
```

**Expected (Toast):**
```
#61:description (this == #61), line 1:  Invalid argument
... called from #61:news_display_seq_full (this == #61), line 4
... called from #6:news, line 45
(End of traceback)
```

## Root Cause

In `vm/verbs.go:verbCall()`, the function used `defer t.PopFrame()` which automatically popped activation frames when the function returned, even when an exception occurred. This meant that when verbs called other verbs and an error occurred deep in the call chain:

1. Frame A pushed (for verb A)
2. Frame B pushed (for verb B called from A)
3. Frame C pushed (for verb C called from B)
4. Error occurs in C
5. Error returns from C → `defer` pops frame C
6. Error returns from B → `defer` pops frame B
7. Error reaches scheduler with only frame A left

The scheduler needs the full call stack to generate proper tracebacks, but the deferred pop operations destroyed that information before the scheduler could extract it.

## Solution

Changed `vm/verbs.go:verbCall()` to:
1. **NOT use `defer t.PopFrame()`** - removed automatic cleanup
2. **Track if frame was pushed** - use `framePushed` boolean
3. **Conditionally pop frames** - only pop on successful completion (not on exceptions)
4. **Leave frames on error** - preserve the stack when `result.Flow == types.FlowException`

This matches the approach already used in `vm/verbs.go:CallVerb()`, which was intentionally designed to leave frames on the stack for traceback extraction.

Also updated `server/scheduler.go:runTask()` to clean up the call stack after sending the traceback, preventing memory leaks.

## Changes Made

### vm/verbs.go:verbCall()

```go
// Before:
t.PushFrame(frame)
defer t.PopFrame()

// After:
var framePushed bool
// ... push frame ...
framePushed = true

// At the end, conditionally pop:
if result.Flow != types.FlowException && framePushed {
    if t, ok := ctx.Task.(*task.Task); ok {
        t.PopFrame()
    }
}
```

### server/scheduler.go:runTask()

Added cleanup after traceback:
```go
if result.Flow == types.FlowException {
    t.SetState(task.TaskKilled)
    s.sendTraceback(t, result.Error)
    // Clean up call stack after traceback has been sent
    for len(t.CallStack) > 0 {
        t.PopFrame()
    }
}
```

## Testing

### Unit Test

Created `vm/callstack_test.go:TestCallStackPreservedOnError()` which verifies:
- Creates 3 verbs: A calls B, B calls C, C errors
- Calls verb A with a task
- Verifies all 3 frames are preserved in the call stack
- **Result: PASS**

### Manual Test with toastcore_barn.db

Tested the `news` command which triggers a multi-level verb call chain that errors:

```
$ ./moo_client.exe -port 9980 -cmd "connect wizard" -cmd "news"
```

**After fix (Barn):**
```
#2 <- #46:messages_in_seq (this == #46), line 6:  Range error
#2 <- ... called from #45:messages_in_seq (this == #45), line 24
#2 <- ... called from #61:news_display_seq_full (this == #61), line 7
#2 <- ... called from #6:news (this == #2), line 45
#2 <- (End of traceback)
```

This now shows all 4 frames in the call chain, matching Toast's behavior.

## Impact

- **Tracebacks** - Now show full call stacks for debugging
- **Compatibility** - Matches Toast/LambdaMOO traceback format
- **No regressions** - Existing unit tests still pass
- **Memory** - Proper cleanup prevents leaks

## Files Modified

1. `vm/verbs.go` - Changed `verbCall()` to preserve stack on exceptions
2. `server/scheduler.go` - Added stack cleanup after traceback
3. `vm/callstack_test.go` - Added test to verify fix (new file)
4. `reports/fix-callstack-tracking.md` - This report

## Commit

Committed as: `1a35a8d - Fix call stack tracking to preserve all frames in verb-to-verb calls`

## Summary

The fix successfully resolves the issue where multi-level verb calls only showed 1 frame in tracebacks. By removing the automatic frame cleanup (`defer PopFrame()`) and only cleaning up frames on successful completion, we now preserve the full call stack when exceptions occur. This matches Toast/LambdaMOO behavior and provides much better debugging information for MOO programmers.

The fix is minimal, focused, and includes both unit tests and manual verification with the toastcore database.
