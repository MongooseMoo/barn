# Task: Fix nil pointer crash in fork/suspend handling

## Context

Barn (Go MOO server) crashes when running tests that use `fork` with `suspend(0)`. The crash happens in `ListValue.String()` due to a nil element in a list.

## Crash Details

```
panic: runtime error: invalid memory address or nil pointer dereference
[signal 0xc0000005 code=0x0 addr=0x20 pc=0x8f8b15]

goroutine 1710 [running]:
barn/types.ListValue.String({{0xaf5148?, 0xc0004f0168?}})
    C:/Users/Q/code/barn/types/list.go:92 +0x95
barn/builtins.builtinToliteral(0x0?, {0xc000488d60?, 0xc0000ad080?, 0x9?})
    C:/Users/Q/code/barn/builtins/types.go:159 +0x90
barn/vm.(*Evaluator).builtinCall(0xc00011a108, 0xc0000ad0c0, 0xc0005861b0)
    C:/Users/Q/code/barn/vm/eval.go:417 +0x3f6
```

The triggering MOO code was:
```
fork (0) suspend(0); endfork;
```

## Working Theory

When a forked task suspends and resumes, something in the task context or return value contains a nil Value where there should be a proper Value. This nil propagates into a list (possibly the task result or notification) and crashes when converted to string.

## Files to Investigate

1. `server/task.go` - Task creation and management
2. `server/scheduler.go` - Task scheduling, suspend/resume
3. `vm/eval_stmt.go` - Fork statement handling (look for `fork` or `ForkStmt`)
4. `builtins/tasks.go` - suspend(), resume() builtins
5. `types/list.go` - Where crash occurs (nil check added as bandaid)

## What to Look For

1. How is a forked task's return value initialized?
2. When suspend() is called, what value is stored?
3. When the task resumes, what value is returned?
4. Is there a path where a nil Value gets into a list?

## Test Command

```bash
cd /c/Users/Q/code/barn
# Build
go build -o barn_test.exe ./cmd/barn/

# Start server
./barn_test.exe -db Test.db -port 9500 > server_test.log 2>&1 &

# Test fork/suspend
printf 'connect wizard\n; fork (0) suspend(0); endfork; return 1;\n' | nc -w 5 localhost 9500
```

## Expected Behavior

The fork/suspend should complete without crashing. The forked task should suspend briefly then complete.

## Output

Write findings and fix to `./reports/fix-fork-nil-crash.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
