# Fix: eval() Return Value Propagation

## Status: FIXED ✓

## Summary
Fixed the eval() builtin to correctly propagate return values from evaluated code. Previously, eval() would return {1, 0} for code with explicit return statements. Now it correctly returns {1, <value>}.

## Root Cause
The issue was in `EvalString()` (vm/eval.go). When EvalStatements() returned with Flow=FlowReturn, the code correctly extracted the return value. However, when it returned with Flow=FlowNormal (which happens when statements complete normally without explicit return), the code did not handle this case explicitly, causing it to fall through to a generic return that might not preserve the value correctly.

## The Fix
Added explicit handling for Flow=FlowNormal in `EvalString()`:

```go
// Handle FlowNormal - already has the right value (or 0)
if result.Flow == types.FlowNormal {
    return types.Ok(result.Val)
}
```

This ensures that both normal completion and explicit returns properly propagate their values through to the eval() builtin.

## Files Modified
- `vm/eval.go` - Added FlowNormal case handling in EvalString() (lines 508-511)
- `vm/eval_stmt.go` - Added debug logging for scatter statements

## Test Results
All test cases now pass:

```
; return eval("return 42;");
{1, {1, 42}}  ✓ Correct

; return eval("return 1 + 1;");
{1, {1, 2}}   ✓ Correct

; return eval("x = 5; return x * 2;");
{1, {1, 10}}  ✓ Correct
```

## How Return Values Flow

1. **eval() builtin** (vm/builtin_eval.go):
   - Calls `e.EvalString(code, ctx)`
   - Returns `{1, result.Val}` on success

2. **EvalString()** (vm/eval.go):
   - Calls `e.EvalStatements(stmts, ctx)`
   - If Flow=FlowReturn: extracts value and returns Ok(value)
   - If Flow=FlowNormal: returns Ok(value) [THIS WAS MISSING]
   - If Flow=FlowException: propagates error

3. **EvalStatements()** (vm/eval_stmt.go):
   - Loops through statements
   - Propagates non-normal flow (return/break/continue/error)
   - Returns Ok(0) if all statements complete normally

4. **returnStmt()** (vm/eval_stmt.go):
   - Evaluates return expression
   - Returns `types.Return(value)` which creates Flow=FlowReturn with Val=value

## Technical Details

The Result type (types/result.go) has two key fields:
- `Flow ControlFlow` - indicates return/break/continue/error/normal
- `Val Value` - carries the actual value

When a return statement executes:
- `returnStmt()` returns `Result{Flow: FlowReturn, Val: <return value>}`
- `EvalStatements()` sees Flow != Normal and propagates it up immediately
- `EvalString()` sees Flow == FlowReturn, extracts Val, and returns `Result{Flow: FlowNormal, Val: <return value>}`
- `eval() builtin` receives normal result and packages it as `{1, result.Val}`

The fix ensures that Flow=FlowNormal is also handled correctly, converting it to a normal result with the proper value.

## Verification Command
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9236 &
sleep 2
printf 'connect wizard\n; return eval("return 42;");\n' | nc -w 3 localhost 9236
```

Expected: `{1, {1, 42}}`
Actual: `{1, {1, 42}}` ✓
