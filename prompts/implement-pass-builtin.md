# Task: Implement pass() Builtin for Verb Inheritance

## Context
Barn is a Go MOO server. The `pass()` builtin is a critical feature that calls the parent object's version of the currently executing verb. This enables verb inheritance chains.

## Objective
Implement `pass()` so verbs can call their parent's implementation.

## How pass() Works
When a verb calls `pass()`:
1. Find the parent of the object where the CURRENT verb is DEFINED (not `this`)
2. Look up the same verb name on that parent
3. Call it with the same `this`, `player`, and `args`
4. Return its result

Key insight: `pass()` looks at where the verb is defined, not where it's being called on.

Example:
```
e <- b <- c  (c inherits from b, b inherits from e)

e:foo = "return {\"e\", @`pass() ! ANY => {}'}"
b:foo = "return {\"b\", @`pass() ! ANY => {}'}"
c:foo = "return {\"c\", @`pass() ! ANY => {}'}"

c:foo() returns {"c", "b", "e"}
```

When c:foo() runs:
- Verb is defined on c, so pass() looks at c's parent (b)
- b:foo runs, pass() looks at b's parent (e)
- e:foo runs, pass() looks at e's parent ($nothing) - no verb found

## Reference Implementations

### ToastStunt (C++)
Location: `~/src/toaststunt/`
- Look for `bf_pass` in the builtins
- Check how it tracks the "definer" object during verb calls

### cow_py (Python)
Location: `~/code/cow_py/`
- Check `src/moo_interp/builtins/` for pass implementation
- Look at how verb execution tracks the defining object

## Implementation Requirements

1. **Track the verb definer** during verb execution
   - When a verb is called, track which object it was DEFINED on (not `this`)
   - This may require adding a field to TaskContext or VerbContext

2. **Implement builtinPass** in `builtins/verbs.go` or similar:
   ```go
   func builtinPass(ctx *types.TaskContext, args []types.Value) types.Result {
       // Get current verb name and definer from context
       // Find parent of definer
       // Look up same verb on parent
       // Call it with same this/player/args
       // Return result
   }
   ```

3. **Register** pass in `builtins/registry.go`

## Files to Examine
- `C:\Users\Q\code\barn\vm\verbs.go` - verb calling mechanism
- `C:\Users\Q\code\barn\types\context.go` - TaskContext definition
- `C:\Users\Q\code\barn\builtins\registry.go` - builtin registration

## Test Command
```bash
cd /c/Users/Q/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9650 -v -k "pass_single_inheritance" 2>&1
```

## Output
Write findings and implementation status to `./reports/implement-pass-builtin.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
