# Task: Fix Call Stack Tracking for Verb-to-Verb Calls

## Problem

When a verb calls another verb, Barn's traceback only shows 1 frame instead of the full call chain.

**Barn output:**
```
#2 <- #6:news (this == #2), line 45:  Range error
#2 <- (End of traceback)
```

**Toast output (same database, same command):**
```
#61:description (this == #61), line 1:  Invalid argument
... called from #61:news_display_seq_full (this == #61), line 4
... called from #6:news, line 45
(End of traceback)
```

Toast shows 3 frames because the call chain is:
1. `#6:news` line 45 calls `$news:news_display_seq_full`
2. `$news:news_display_seq_full` line 4 calls `this:description()`
3. `#61:description` line 1 has the actual error

Barn only shows frame 1.

## Root Cause

When `callVerb` or similar functions invoke another verb, they need to push an activation frame onto the task's call stack BEFORE executing the verb, and pop it AFTER.

## Where to Look

1. `server/verbs.go` - `callVerb` function
2. `vm/verbs.go` - verb invocation in the VM
3. `vm/builtin_eval.go` - `call_function` builtin if used
4. `task/task.go` - `PushFrame` and `PopFrame` methods (already exist)

## What to Fix

Find where verbs are invoked and ensure:
1. `task.PushFrame(ActivationFrame{...})` is called BEFORE executing the verb
2. `task.PopFrame()` is called AFTER the verb completes (even on error)
3. The frame includes: This, Player, Programmer, Caller, Verb, VerbLoc, Args, LineNumber

## Test

```bash
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9950 > server.log 2>&1 &
sleep 3
./moo_client.exe -port 9950 -timeout 10 -cmd "connect wizard" -cmd "news"
```

After fix, traceback should show multiple frames like Toast does.

## Output

Write findings to `./reports/fix-callstack-tracking.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
