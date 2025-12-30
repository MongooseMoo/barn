# Task: Fix eval() Return Value Propagation

## Context
Barn is a Go MOO server. The eval() builtin always returns `{1, 0}` regardless of what the evaluated code returns.

## The Bug
```
; return eval("return 42;");
{1, 0}   // WRONG - should be {1, 42}

; return eval("return 1 + 1;");
{1, 0}   // WRONG - should be {1, 2}
```

## Root Cause
From investigation: the return value is not being propagated through:
`EvalString()` → `EvalStatements()` → return statement handling

When a `return` statement executes, its value needs to bubble up through EvalStatements and be captured by EvalString.

## Files to Fix
- `vm/eval.go` - EvalString() function
- `vm/eval_stmt.go` - EvalStatements() and return statement handling

## What to Check
1. How does `return X;` set its return value?
2. How does EvalStatements() detect and return the value from a return statement?
3. How does EvalString() capture and return that value?

Look at how Flow types work - likely `types.FlowReturn` should carry the return value.

## Test Command
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9235 &
sleep 2
printf 'connect wizard\n; return eval("return 42;");\n' | nc -w 3 localhost 9235
```

Expected: `{1, 42}`

## Output
Write status to `./reports/fix-eval-return-value.md`

## CRITICAL: Do NOT modify tests
The tests are correct. Only fix the Go implementation.

## CRITICAL: File Modified Error Workaround
If Edit/Write fails:
1. Read the file again
2. Retry the Edit
3. Try path formats: `./vm/eval.go`, `C:/Users/Q/code/barn/vm/eval.go`
4. NEVER use cat, sed, echo
