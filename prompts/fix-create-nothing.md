# Task: Fix create($nothing) E_INVARG error

## Context

The barn Go MOO server is failing conformance tests. The `create` test suite setup fails with E_INVARG when executing:

```moo
$test_obj = create($nothing);
```

This should create an object with no parent (parent = #-1).

## Problem

The setup code in `tests/conformance/builtins/create.yaml` at `~/code/cow_py/` does:

```yaml
setup:
  permission: wizard
  code: |
    add_property(#0, "test_obj", 0, {player, "rwc"});
    $test_obj = create($nothing);
    ...
```

When running against barn via socket transport, `create($nothing)` returns E_INVARG.

## Investigation Steps

1. Check if `$nothing` resolves correctly to `#-1` (ObjNothing) in barn
   - Look at `vm/environment.go` for how $nothing is handled
   - Trace what value gets passed to the create() builtin

2. Check the create() builtin implementation
   - File: `builtins/objects.go`
   - Find the `create` builtin registration
   - Check how it validates the parent argument
   - Determine if #-1 (no parent) is handled correctly

3. Test manually:
   ```bash
   printf 'connect wizard\n; return $nothing;\n' | nc -w 3 localhost 9327
   printf 'connect wizard\n; return create($nothing);\n' | nc -w 3 localhost 9327
   ```

## Expected Behavior

- `$nothing` should resolve to `#-1` (ObjNothing)
- `create($nothing)` or `create(#-1)` should create an object with no parent
- The result should be a valid object number, not E_INVARG

## Files to Check

- `~/code/barn/vm/environment.go` - system object resolution ($nothing, $object, etc.)
- `~/code/barn/builtins/objects.go` - create() builtin implementation
- `~/code/barn/types/values.go` - ObjNothing constant definition

## Output

Write findings and any fixes to `~/code/barn/reports/fix-create-nothing.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
