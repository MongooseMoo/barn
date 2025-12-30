# Investigation Report: property::create_grandchild Test Failure

## Problem Statement

The conformance test `property::create_grandchild` is failing. The test expects `{1, 1}` but the actual behavior shows different issues.

## Test Definition

```yaml
- name: create_grandchild
  code: 'eval("$temp0 = create($temp); return parent($temp0) == $temp;")'
  expect:
    value: [1, 1]
```

## How the Test Framework Works

1. **Test framework** (schema.py line 218): Wraps `code` field with `return <code>;`
2. **Becomes**: `return eval("$temp0 = create($temp); return parent($temp0) == $temp;");`
3. **Sent via socket** as: `; return eval("...");`
4. **Server executes**: The `;` command invokes verb `#2:eval`

## Server-Side Execution Flow

### The #2:eval Verb (Test.db)
```moo
set_task_perms(player);
try
try
notify(player, "-=!-^-!=-");
notify(player, toliteral(eval(argstr)));
except e (ANY)
notify(player, toliteral({2, e}));
endtry
finally
notify(player, "-=!-v-!=-");
endtry
```

This verb:
1. Receives `argstr` = `return eval("$temp0 = create($temp); return parent($temp0) == $temp;");`
2. Calls builtin `eval(argstr)` which evaluates the string
3. Wraps result with `toliteral()` and sends via `notify()`

### The eval() Builtin (vm/builtin_eval.go)

```go
func (e *Evaluator) RegisterEvalBuiltin() {
    e.builtins.Register("eval", func(ctx *types.TaskContext, args []types.Value) types.Result {
        // ... validation ...
        code := strings.Join(lines, "\n")
        result := e.EvalString(code, ctx)

        if result.Flow == types.FlowException {
            // Return {0, error_value}
            return types.Ok(types.NewList([]types.Value{
                types.NewInt(0),
                types.NewErr(result.Error),
            }))
        }

        // Return {1, result_value}
        return types.Ok(types.NewList([]types.Value{
            types.NewInt(1),
            result.Val,
        }))
    })
}
```

The builtin returns:
- `{1, value}` on success
- `{0, error}` on failure

### EvalString and EvalStatements (vm/eval.go, vm/eval_stmt.go)

```go
func (e *Evaluator) EvalString(code string, ctx *types.TaskContext) types.Result {
    p := parser.NewParser(code)
    stmts, err := p.ParseProgram()
    if err != nil {
        return types.Err(types.E_INVARG)
    }

    result := e.EvalStatements(stmts, ctx)

    // Handle FlowReturn - extract the value
    if result.Flow == types.FlowReturn {
        return types.Ok(result.Val)
    }

    return result
}

func (e *Evaluator) EvalStatements(stmts []parser.Stmt, ctx *types.TaskContext) types.Result {
    for _, stmt := range stmts {
        result := e.EvalStmt(stmt, ctx)
        if !result.IsNormal() {
            return result
        }
    }
    // Normal completion - return 0 (default)
    return types.Ok(types.NewInt(0))
}
```

**Critical finding**: When code has no `return` statement, `EvalStatements` returns `types.NewInt(0)` (the "void" value).

## Actual Test Results

### Test Run Output
```
expected value [1, 1], but got [0, 'E_PERM']
```

The eval() call is failing with `E_PERM`, which means permission check is failing.

### Manual Socket Testing

```bash
# Simple eval with return
; eval("return 42;")
→ {1, 0}

# Eval without return
; eval("5")
→ {1, 0}

# Nested eval with return wrapper
; return eval("1 + 1");
→ {1, {0, E_INVARG}}
```

## Root Cause Analysis

### Issue #1: eval() Always Returns {1, 0}

When I test `eval("return 42;")`, it returns `{1, 0}` instead of `{1, 42}`.

**Hypothesis**: The return statement inside eval() is not properly propagating its value through EvalString(). Even though EvalString() checks for `FlowReturn` and extracts the value, something is going wrong.

**Need to investigate**:
1. Is `returnStmt()` setting `Flow = FlowReturn` correctly?
2. Is `result.Val` being set correctly in returnStmt()?
3. Is there a problem with how the value is being wrapped in the eval builtin?

### Issue #2: Permission Errors (E_PERM)

The test is getting `E_PERM` when trying to execute the eval code. This suggests:
- Either the task context doesn't have proper permissions set
- Or the builtin functions (like `create()`) are checking permissions and rejecting

**Note**: Even when connected as wizard, the eval is failing with permission errors.

### Issue #3: Expected Double-Wrapping (from prompt)

The original prompt mentioned seeing `{1, {1, 1}}` instead of `{1, 1}`. This would happen if:

1. Inner eval: `eval("$temp0 = create($temp); return parent($temp0) == $temp;")` → returns `{1, 1}`
2. Outer return: `return eval(...)` → wraps as `{1, {1, 1}}`

But the **SocketTransport** (transport.py lines 596-614) unwraps eval responses:

```python
if isinstance(value, list) and len(value) == 2 and isinstance(value[0], int):
    status, result = value
    if status == 1:
        # Success: {1, actual_value}
        return ExecutionResult(success=True, value=result)
```

So if the server sends `{1, {1, 1}}`, the transport unwraps it to just `{1, 1}` and stores that in `ExecutionResult.value`.

**But**: This only happens if the outer eval succeeds. Currently the outer eval is failing.

## Next Steps

### Priority 1: Fix eval() Return Value Bug

The builtin eval() is not properly returning the result of code with return statements. Need to:

1. Check if `returnStmt()` implementation is setting Flow and Val correctly
2. Verify `EvalString()` is properly extracting the value from FlowReturn
3. Add debug logging to trace exactly what's happening

### Priority 2: Fix Permission Errors

The E_PERM errors suggest:
1. Task context permissions not being set properly
2. Or builtin functions doing permission checks that fail

Need to check:
- How `set_task_perms()` is supposed to work
- Whether it's implemented in barn
- Whether builtins are checking permissions correctly

### Priority 3: Understand Variable Scope

The test relies on `$temp` being available from a previous test (`create_child_of_root`). Need to verify:
- Global variables persist between eval() calls
- Test framework is running tests in order
- Variables are stored in the right scope (global environment)

## Files Involved

- `C:/Users/Q/code/barn/vm/builtin_eval.go` - eval() builtin implementation
- `C:/Users/Q/code/barn/vm/eval.go` - EvalString() method
- `C:/Users/Q/code/barn/vm/eval_stmt.go` - EvalStatements() and returnStmt()
- `C:/Users/Q/code/barn/Test.db` - #2:eval verb definition
- `C:/Users/Q/code/cow_py/tests/conformance/transport.py` - Response parsing
- `C:/Users/Q/code/cow_py/tests/conformance/basic/property.yaml` - Test definitions

## Conclusion

The "double-wrapping" issue mentioned in the prompt is **not currently happening** - instead, the eval() builtin has more fundamental bugs:

1. **Return values not propagating**: `eval("return 42;")` returns `{1, 0}` instead of `{1, 42}`
2. **Permission errors**: eval() is failing with E_PERM even for wizard
3. **Variable scope issues**: $temp from previous tests may not be accessible

The first two issues must be fixed before the double-wrapping issue becomes relevant.
