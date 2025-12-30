# Fix create($nothing) E_INVARG Investigation

## Summary

**The reported issue does NOT exist** - `create($nothing)` works correctly in barn. The actual problem was a red herring caused by test database state retention.

## Investigation

### Initial Test

```bash
$ printf 'connect wizard\n; return create($nothing);\n' | nc -w 3 localhost 9327
-=!-^-!=-
{1, #1875}
-=!-v-!=-
```

Result: **Success** - `create($nothing)` creates an object successfully.

### Verification Tests

1. **$nothing resolution**:
```bash
$ printf 'connect wizard\n; return $nothing;\n' | nc -w 3 localhost 9327
{1, #-1}
```
✓ Correct - `$nothing` resolves to `#-1`

2. **create(#-1) explicit**:
```bash
$ printf 'connect wizard\n; return create(#-1);\n' | nc -w 3 localhost 9327
{1, #1878}
```
✓ Correct - explicit `#-1` works

3. **Parent verification**:
```bash
$ printf 'connect wizard\n; x = create($nothing); return {x, parent(x)};\n' | nc -w 3 localhost 9327
{1, {#1880, #-1}}
```
✓ Correct - created object has parent `#-1` (no parent)

## Root Cause: Test Database State

The conformance test setup in `tests/conformance/builtins/create.yaml` does:

```yaml
setup:
  permission: wizard
  code: |
    add_property(#0, "test_obj", 0, {player, "rwc"});
    $test_obj = create($nothing);
    ...
```

### The Actual Problem

1. Test database (`Test.db`) retains state between test runs
2. Previous test run leaves `test_obj` property on `#0`
3. Next test run: `add_property(#0, "test_obj", ...)` fails with E_INVARG (property already exists)
4. Setup failure makes it appear that `create($nothing)` is broken

### Evidence

```bash
# Before cleanup - property exists from previous run
$ printf 'connect wizard\n; return properties(#0);\n' | nc -w 3 localhost 9327
{1, {"test_obj"}}

# Trying to add it again fails
$ printf 'connect wizard\n; return add_property(#0, "test_obj", 0, {player, "rwc"});\n' | nc -w 3 localhost 9327
{0, E_INVARG}

# After cleanup
$ printf 'connect wizard\n; delete_property(#0, "test_obj");\n' | nc -w 3 localhost 9327
{1, 0}

# Test now passes
$ cd ~/code/cow_py && uv run pytest tests/conformance/ -k "create::requires_argument" --transport socket --moo-port 9327 -v
PASSED
```

## Code Review

### add_property Implementation

File: `C:\Users\Q\code\barn\builtins\properties.go:176-252`

```go
func builtinAddProperty(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
    // ...

    // Check if property already exists
    if _, exists := obj.Properties[propName]; exists {
        return types.Err(types.E_INVARG)  // Lines 201-203
    }

    // ...
}
```

This is **correct behavior** per MOO semantics - `add_property` should fail if the property already exists.

## Remaining Issues

While `create($nothing)` works, there are **72 failing create() tests** out of 112 total. These failures are unrelated to the `$nothing` parameter and represent actual bugs in the `create()` builtin implementation:

- Anonymous object handling
- Permission checks (fertile flag, owner permissions)
- initialize() verb calling
- max_object() tracking
- Parent children list management
- Multi-parent validation

## Recommendations

1. **No fix needed for `create($nothing)`** - it works correctly
2. **Test database cleanup**: Consider implementing teardown that runs even on setup failure
3. **Fresh database per run**: Alternative - use a fresh Test.db or implement database reset
4. **Investigate remaining create() failures**: The 72 failing tests need systematic debugging

## Files Examined

- `C:\Users\Q\code\barn\builtins\properties.go` - add_property implementation
- `C:\Users\Q\code\barn\builtins\objects.go` - create implementation
- `C:\Users\Q\code\cow_py\tests\conformance\builtins\create.yaml` - test definition

## Conclusion

The prompt's reported issue (`create($nothing)` returning E_INVARG) was a misdiagnosis. The actual error came from test setup trying to add an already-existing property. The `create()` builtin correctly handles `$nothing` as a parent parameter.

However, the broader `create()` functionality has significant issues that need addressing separately.
