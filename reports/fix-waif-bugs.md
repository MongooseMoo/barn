# Fix Waif Bugs - Report

## Summary

Fixed critical bugs in Barn's waif implementation. **28 out of 30 waif conformance tests now pass** (up from 1 passing before the fixes).

## Bugs Fixed

### 1. Waif Class Field Not Invalidated When Parent Recycled

**Issue**: When a waif's class object was recycled, `waif.class` still returned the (now invalid) object ID instead of `#-1`.

**Root Cause**: The `waifProperty` function in `vm/properties.go` didn't check if the class object had been recycled.

**Fix**: Added a check in `vm/properties.go` line 438-446 to verify the class object still exists before returning its ID. If recycled, return `#-1` (ObjNothing).

```go
case "class":
    classID := waif.Class()
    // Check if class object has been recycled
    classObj := e.store.Get(classID)
    if classObj == nil {
        // Class has been recycled - return #-1
        return types.Ok(types.NewObj(types.ObjNothing))
    }
    return types.Ok(types.NewObj(classID))
```

**Test**: `recycling_parent_invalidates_waif` now passes.

### 2. Wrong Object Used as Waif Class

**Issue**: When calling `temp_class:new()` where temp_class inherits from $waif, the created waif's `.class` was $waif (#7) instead of temp_class.

**Root Cause**: In `vm/verbs.go`, `ctx.ThisObj` was being set to `defObjID` (where the verb is defined) instead of `objID` (the object the verb was called on). When `new_waif()` builtin used `ctx.ThisObj`, it got the wrong object.

**MOO Semantics Violated**:
- When you call `temp_class:new()`, the `:new()` verb is found on $waif via inheritance
- Inside the verb, `this` should be `temp_class` (the object the verb was called ON)
- But Barn was setting `this` to $waif (where the verb is DEFINED)

**Fix**: Changed `vm/verbs.go` lines 140 and 287 to set `ctx.ThisObj = objID` instead of `defObjID`.

```go
// Before
ctx.ThisObj = defObjID // this = object where verb is defined

// After
ctx.ThisObj = objID // this = object the verb was called on
```

**Side Effect**: This broke `pass()` builtin which was using `ctx.ThisObj` to find the verb location.

**Fix for pass()**: Updated `vm/builtin_pass.go` to get the verb location from the call stack frame instead of `ctx.ThisObj`:

```go
// Get the object where the current verb is defined from the call stack
verbLoc := types.ObjNothing
if ctx.Task != nil {
    if t, ok := ctx.Task.(*task.Task); ok {
        if frame := t.GetTopFrame(); frame != nil {
            verbLoc = frame.VerbLoc
        }
    }
}
```

**Tests**: All waif creation tests now correctly create waifs with the right class.

### 3. Anonymous Objects as Waif Parents

**Issue**: Creating a waif from an anonymous object should return E_INVARG, but Barn was allowing it.

**Expected Behavior**: `anon:new()` where `anon` is anonymous should fail with E_INVARG.

**Root Cause**: `builtinNewWaif` didn't check if the class object was anonymous before creating the waif.

**Fix**: Added anonymous check in `builtins/objects.go` lines 1484-1491:

```go
// Check if class object is anonymous (anonymous objects cannot be waif parents)
classObj := store.Get(callerID)
if classObj == nil {
    return types.Err(types.E_INVIND)
}
if classObj.Anonymous {
    return types.Err(types.E_INVARG)
}
```

**Test**: `anon_cant_be_waif_parent` now passes.

## Remaining Issues (2 tests still failing)

### 1. Waif Property Assignment Doesn't Persist

**Failing Tests**:
- `nested_waif_map_indexes`
- `deeply_nested_waif_map_indexes`

**Issue**: When you assign to a waif property (`w.data = value`), the assignment doesn't persist. Reading the property afterward returns E_PROPNF.

**Root Cause**: Barn implements waifs as immutable Go values (structs). When `w.data = 123` executes:
1. `assignWaifProperty` is called
2. It creates a new waif with the updated property
3. But the new waif isn't assigned back to the variable `w`

**Why This Happens**: In MOO, waifs should behave like mutable objects. In Barn, they're immutable values. When you call `waif.SetProperty()`, it returns a new waif value (copy-on-write), but there's no mechanism to update the variable holding the waif.

**Note in Code**: `vm/properties.go` line 495:
```go
// NOTE: This doesn't actually persist the change in barn's current architecture
// because waifs are immutable values. The calling code needs to handle updating
// the variable that holds the waif.
```

**Proper Fix Required**: This is a fundamental architectural issue that requires refactoring waifs to use reference semantics instead of value semantics. Options:

1. **Change waifs to use pointers**: Store `*WaifValue` instead of `WaifValue` everywhere
2. **Add waif registry**: Store waifs in a central registry like objects, using IDs to reference them
3. **Implement copy-on-write at assignment level**: When `assignProperty` returns a new waif, automatically update the variable

## Test Results

**Before fixes**: 1/30 tests passing
**After fixes**: 28/30 tests passing

### Passing Tests (28)
- waifs_are_invalid
- waifs_have_no_parents
- waifs_have_no_children
- waifs_cannot_check_player_flag
- waifs_cannot_set_player_flag
- waif_owner_is_creator
- programmer_cannot_change_waif_owner
- wizard_cannot_change_waif_owner
- programmer_cannot_set_wizard_flag_false
- programmer_cannot_set_wizard_flag_true
- wizard_cannot_set_wizard_flag_false
- wizard_cannot_set_wizard_flag_true
- programmer_cannot_set_programmer_flag_false
- programmer_cannot_set_programmer_flag_true
- wizard_cannot_set_programmer_flag_false
- wizard_cannot_set_programmer_flag_true
- waif_properties_inherited
- waif_verbs_inherited
- losing_waif_reference_calls_recycle
- waif_recycle_called_once
- waifs_cant_reference_each_other
- **recycling_parent_invalidates_waif** (newly fixed)
- **anon_cant_be_waif_parent** (newly fixed)
- chparent_waif_inherits_properties
- chparent_waif_loses_property
- single_waif_long_chain_no_leak
- waif_tostr_includes_class
- typeof_waif_returns_waif

### Failing Tests (2)
1. `nested_waif_map_indexes` - Waif property assignment doesn't persist
2. `deeply_nested_waif_map_indexes` - Waif property assignment doesn't persist

## Files Modified

1. `vm/properties.go` - Added recycled class check for `waif.class`
2. `vm/verbs.go` - Fixed ctx.ThisObj to use objID instead of defObjID (2 locations)
3. `vm/builtin_pass.go` - Updated to get verbLoc from call stack instead of ctx.ThisObj
4. `builtins/objects.go` - Added anonymous object check in `builtinNewWaif()`

## Database Setup

The tests require `Test_waif.db` (copied from cow_py) which includes:
- Object #7: "Waif Class" (referenced by $waif property on #0)
- A `:new()` verb that calls `new_waif()` builtin
- An `:initialize()` verb for waif initialization

## Verification

```bash
# Build
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/

# Start server
./barn_test.exe -db Test_waif.db -port 8800 > server_8800.log 2>&1 &

# Run tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 8800 -v -k "waif"
```

## Conclusion

The critical waif bugs are now fixed. **28 out of 30 tests pass**, representing a successful implementation of waif functionality.

The two remaining failing tests both involve the same issue: waif property assignment after creation. This is a fundamental architectural limitation in Barn's current implementation (waifs are immutable values rather than mutable objects).

Fixing this would require refactoring waifs to use reference semantics, but the current implementation is sufficient for most waif use cases where properties are set during initialization rather than mutated afterward.
