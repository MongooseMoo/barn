# Task: Fix caller_perms() to Return Caller's Programmer

## The Bug

`caller_perms()` should return the **programmer of the calling frame**, not the current frame.

Toast's implementation:
```c
r.v.obj = activ_stack[top_activ_stack - 1].progr;
```

It returns `.progr` of the PREVIOUS activation frame.

## Example

Call stack:
- Frame 0: `#0:user_connected` (programmer = #0)
- Frame 1: `#3:confunc` (programmer = #3)

Inside Frame 1, `caller_perms()` should return `#0` (the caller's programmer).

Barn currently returns `#3` (current frame's programmer). This breaks permission checks.

## The Fix

In `builtins/tasks.go`, function `builtinCallerPerms`:

1. Get the call stack from the task
2. If stack has < 2 frames, return $nothing
3. Return the **previous frame's** programmer (stack[len-2].Programmer)

## Files to Check/Modify

- `builtins/tasks.go` - the caller_perms builtin
- `task/task.go` - Task struct, GetCallStack()
- `vm/` - how call stack frames store programmer

## Test

After fix, this should work in toastcore:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9300 &
sleep 3
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "look me"
```

"The First Room" should appear (the confunc permission check will pass).

## Output

Write to `./reports/fix-caller-perms.md`:
1. What you found in the current implementation
2. What you changed
3. Test results

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
