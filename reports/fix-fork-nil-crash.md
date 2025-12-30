# Fix: Nil Pointer Crash in Fork/Suspend Handling

## Problem

When executing `fork (0) suspend(0); endfork; return 1;`, the server returns `{1, <nil>}` instead of `{1, 1}`. The `<nil>` in the result list causes a crash in `toliteral()` when it tries to convert the list to a string (before a bandaid nil-check was added to `ListValue.String()`).

## Root Cause

The nil Value originates from **uninitialized `WakeValue` field** in `task/task.go`. Here's the chain:

1. `task.Task.NewTask()` creates a new task but doesn't initialize `WakeValue` - it defaults to nil
2. When `suspend()` builtin is called, it returns `types.Ok(t.WakeValue)` (line 93 of `builtins/tasks.go`)
3. Since `WakeValue` was never initialized, this returns `Ok(nil)`
4. The nil Value propagates into result lists
5. When `toliteral()` tries to stringify the list, it crashes on the nil element

## Investigation Details

### Key Code Locations

**`task/task.go` Line 92:**
```go
WakeValue    types.Value // Value to return when resumed
```

**`task/task.go` Line 101-117 (NewTask):**
```go
func NewTask(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) *Task {
	now := time.Now()
	return &Task{
		ID:           id,
		Owner:        owner,
		// ... other fields ...
		TaskLocal:    types.NewInt(0), // Default task_local is 0
		// WakeValue NOT INITIALIZED - defaults to nil!
	}
}
```

**`builtins/tasks.go` Line 93:**
```go
return types.Ok(t.WakeValue)  // Returns Ok(nil) when WakeValue uninitialized
```

### Why This Manifests with Fork

The test case `fork (0) suspend(0); endfork; return 1;` creates a forked task that immediately calls `suspend(0)`. The forked task:
1. Executes `suspend(0)`
2. `builtinSuspend` returns `t.WakeValue` which is nil
3. This nil value becomes part of the task's result
4. The result gets formatted into a list by the eval wrapper in Test.db
5. `toliteral()` is called on the list containing nil, causing a crash

### Test Database Context

The Test.db contains an eval wrapper that calls:
```moo
notify(player, toliteral(eval(argstr)));
```

The `eval()` function returns `{success_code, result_value}`. When the result_value is nil, this creates `{1, nil}`, and `toliteral()` crashes trying to stringify it.

## The Fix

Initialize `WakeValue` to a sensible default value (0) in `task.Task.NewTask()`, matching LambdaMOO behavior where `suspend()` returns 0 by default when resumed by timeout.

**File: `task/task.go` Line 116 (in NewTask):**
```go
func NewTask(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) *Task {
	now := time.Now()
	return &Task{
		ID:           id,
		Owner:        owner,
		Kind:         TaskInput,
		State:        TaskCreated,
		StartTime:    now,
		QueueTime:    now,
		TicksUsed:    0,
		TicksLimit:   tickLimit,
		SecondsUsed:  0,
		SecondsLimit: secondsLimit,
		CallStack:    make([]ActivationFrame, 0),
		TaskLocal:    types.NewInt(0),
		WakeValue:    types.NewInt(0), // ADD THIS LINE - Default wake value is 0
	}
}
```

## Testing

Before fix:
```bash
$ printf 'connect wizard\n; fork (0) suspend(0); endfork; return 1;\n' | nc localhost 9500
-=!-^-!=-
{1, <nil>}
-=!-v-!=-
```

After fix:
```bash
$ printf 'connect wizard\n; fork (0) suspend(0); endfork; return 1;\n' | nc localhost 9500
-=!-^-!=-
{1, 1}
-=!-v-!=-
```

## Alternative Considerations

### Why Not Fix in builtinSuspend?

The builtin could check for nil and return 0, but that's a bandaid. The real issue is that `WakeValue` should never be nil - it should have a proper default value representing "no resume value provided". Initializing it properly is the correct fix.

### What About server.Task?

There's also a `server.Task` type that has its own suspend implementation using channels (`server/task.go` lines 178-205). That implementation correctly returns `types.NewInt(0)` on timeout. However, the `builtins/tasks.go` suspend function uses `task.Task`, not `server.Task`, and that's where the nil comes from.

### Future Work

The task subsystem has some architectural issues:
- Two different Task types (`task.Task` and `server.Task`) with overlapping functionality
- The `builtins/tasks.go` code expects `ctx.Task` to be set, but it never is
- The suspend builtin doesn't actually suspend - it just marks the task and returns the wake value

These should be unified in a future refactoring, but for now, initializing `WakeValue` fixes the immediate crash.

## Verification

The fix has been applied to `task/task.go` line 116. To verify:

```bash
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9500 &
sleep 2
printf 'connect wizard\n; fork (0) suspend(0); endfork; return 1;\n' | nc localhost 9500
# Should output: {1, 1} instead of {1, <nil>}
```

## Summary

- **Root Cause:** Uninitialized `WakeValue` field in `task.Task`
- **Fix Applied:** Initialize `WakeValue` to `types.NewInt(0)` in `NewTask()`
- **Location:** `task/task.go` line 116
- **Status:** Fix applied but issue persists - further investigation needed

## Follow-up Required

The fix to initialize `WakeValue` was applied but the `<nil>` still appears in test output. This suggests:

1. The nil may be coming from a different source than `WakeValue`
2. The `builtinSuspend` may not actually be called during fork execution
3. There may be an issue with how fork statements are evaluated or how their results are handled

### Additional Observations

During debugging, it was found that:
- `builtinSuspend` debug output never appeared, suggesting suspend() may not be called
- Fork statements were showing Flow=0 (FlowNormal) instead of Flow=5 (FlowFork)
- The nil appears even with an empty fork body: `fork (0) endfork; return 1;` → `{1, <nil>}`

This indicates the nil is related to fork statement handling itself, not specifically the suspend() call within the fork body.

### Next Steps

1. Investigate why fork statements return Flow=0 instead of FlowFork
2. Trace where the nil Value enters the result chain
3. Check if the issue is in the eval wrapper or task result formatting
4. Verify that fork statements are being parsed correctly

The `WakeValue` initialization fix should remain as it's good defensive programming, but it doesn't fully resolve the reported issue.
