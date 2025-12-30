# Fix create($temp) Permission Check - Investigation Report

## Task Summary
Investigate E_PERM error when running `create_grandchild` test that attempts `create($temp)`.

## Key Finding: **No Bug Found in barn**

The test failure was a **red herring**. There is no permission bug in barn's create() implementation.

## What Actually Happened

### The Misleading Test Failure
When running the `create_grandchild` test in isolation:
```bash
pytest tests/conformance/ -k "create_grandchild" -xvs
```

The test failed with:
```
AssertionError: Test 'create_grandchild' expected value [1, 1],
but got [0, ['E_PROPNF', "Property 'temp' not found on #0"]]
```

**The error was E_PROPNF (property not found), NOT E_PERM (permission denied).**

### The Real Issue: Test Dependencies

The `property.yaml` test suite has sequential dependencies:
1. `add_property_temp` - adds property `#0.temp`
2. `add_property_temp0` - adds property `#0.temp0`
3. `create_child_of_root` - `$temp = create(#1)` (writes to #0.temp)
4. `create_grandchild` - `$temp0 = create($temp)` (reads #0.temp)

When test #4 runs in isolation, properties from tests #1-2 don't exist, so reading `$temp` fails with E_PROPNF.

### Verification: All Tests Pass in Sequence
```bash
pytest tests/conformance/ -k "property::" -xvs
# Result: All 17 property tests PASSED
```

When run as a complete suite, all tests pass including `create_grandchild`.

## Code Analysis: Barn's create() Permission Logic

Reviewed `builtins/objects.go` lines 163-178:

```go
// Check fertile flag and permissions:
// - Wizards can create from any object
// - Non-wizards can only create from objects they own OR that have the fertile flag
// Note: Wizard check is based on Player, not Programmer (verb owner)
playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)
if !playerIsWizard {
    isOwner := parent.Owner == ctx.Programmer
    hasFertile := parent.Flags.Has(db.FlagFertile)
    if !isOwner && !hasFertile {
        return types.Err(types.E_PERM)
    }
}
```

**This implementation is correct:**
- Checks `ctx.IsWizard` flag (set during connection)
- Also checks `isPlayerWizard(store, ctx.Player)` for player object wizard flag
- Wizards bypass ownership and fertile flag checks
- Non-wizards require ownership OR fertile flag

The permission logic correctly allows wizards to create from ANY object, including $temp.

## Manual Testing Confirmed

Tested barn directly:
```bash
# Connection 1: create $temp from #1
printf 'connect wizard\n; return eval("$temp = create(#1); return $temp;");\n' | nc localhost 9240
# Result: {1, {1, #57}} - SUCCESS

# Connection 2: create from $temp
printf 'connect wizard\n; return eval("$temp0 = create($temp); return $temp0;");\n' | nc localhost 9240
# Result: {1, {1, #59}} - SUCCESS
```

Both connections succeed. Wizards can create from $temp even when:
- Different wizard object owns $temp
- $temp has no fertile flag
- Different task context

## Conclusion

**Status: No changes needed**

The barn implementation is correct. The test failure was due to running an isolated test without its setup dependencies, not a permission bug in create().

### What We Learned
1. The prompt's hypothesis about "each connect wizard creates a NEW wizard" was partially true but didn't cause the actual problem
2. The error message "E_PROPNF" should have been the first clue - it wasn't a permission issue
3. Conformance tests must run as complete suites to handle sequential dependencies
4. Barn's wizard permission check correctly uses both `ctx.IsWizard` and `isPlayerWizard()` to handle all scenarios

### Files Reviewed
- `C:\Users\Q\code\barn\builtins\objects.go` - create() implementation (lines 83-343)
- `C:\Users\Q\code\cow_py\tests\conformance\basic\property.yaml` - test definitions

### Test Results
- Individual test: FAILED (E_PROPNF - missing setup)
- Full suite: PASSED (all 17 property tests)
- Manual verification: PASSED (barn handles wizard permissions correctly)
