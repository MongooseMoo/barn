# Fix Misc Tests - Investigation Report

**Date**: 2025-12-30
**Status**: Investigation Complete - Fixes Required

## Overview

Investigation of 9 miscellaneous conformance test failures across multiple categories: waif, index_and_range, recycle, anonymous, caller_perms, and math.

## Test Failures Analyzed

### 1. Waif Nested Map Indexing (2 tests)
- `waif::nested_waif_map_indexes`
- `waif::deeply_nested_waif_map_indexes`

**Issue**: Returns E_PROPNF when accessing nested maps on waif properties (e.g., `w.data["outer"]["inner"]`)

**Root Cause**: Waif property assignment is broken due to immutability semantics not being properly handled.

**Location**: `vm/properties.go`, lines 473-510 (assignWaifProperty function)

**Problem**:
```go
// Line 507 - assigns but ignores the return value!
_ = waif.SetProperty(propName, value)
return types.Ok(value)
```

Waifs are immutable values. When `SetProperty()` is called, it returns a NEW waif instance with the property set. But this line ignores the new waif and doesn't update the variable holding the waif.

**Fix Required**:
The `assignProperty()` function needs special handling for waifs:
1. When target is a PropertyExpr with a waif value (e.g., `w.data`)
2. Extract the variable name from the PropertyExpr
3. Call SetProperty() on the waif and capture the NEW waif
4. Update the variable with the new waif instance

This is a fundamental architectural issue - the current code assumes mutable objects but waifs are immutable values.

### 2. Index and Range (2 tests)
- `index_and_range::range_list_single` - Expected: `["three"]`, testing `{"one", "two", "three"}[3..3]`
- `index_and_range::decompile_with_index_operators` - Expected decompiler output format

**Status**: Not fully investigated due to server connectivity issues during testing.

**Next Step**: Need to test these with toast_oracle to understand expected behavior.

### 3. Recycle Invalid Objects (3 tests)
- `recycle::recycle_invalid_already_recycled_object`
- `recycle::recycle_invalid_already_recycled_anonymous`
- `anonymous::recycle_invalid_anonymous_no_crash`

**Issue**: When calling `recycle()` on an already-recycled object or anonymous object, should return E_INVARG but likely returns something else or crashes.

**Expected Behavior** (from test):
```moo
x = create($object, 0);
recycle(x);
return recycle(x);  // Should return E_INVARG
```

**Fix Required**: Check if object is already recycled before attempting to recycle it in the `recycle()` builtin implementation.

**Location**: `builtins/objects.go` (recycle builtin)

### 4. Caller Perms Top Level (1 test)
- `caller_perms::caller_perms_top_level_eval`

**Issue**: `caller_perms()` at top level (eval) should return player object

**Expected**: Returns `#3` (the wizard player)
**Actual**: Unknown - needs testing

**Location**: `builtins/execution.go` (caller_perms builtin)

**Fix Required**: When called at top level of eval (no verb in call stack), should return the current player object.

### 5. Random 64-bit Range (1 test)
- `math::random_in_valid_range_64bit`

**Issue**: `random()` without arguments should return value in range [1, 9223372036854775807] on 64-bit systems

**Test Condition**: `skip_if: "not feature.64bit"`

**Location**: `builtins/math.go` (random builtin)

**Fix Required**: Check if random() correctly handles 64-bit max int range.

## Test Environment Issues

During investigation, encountered issues with:
1. Server crashing or exiting unexpectedly
2. moo_client connection failures
3. Python test framework connection errors

These issues prevented systematic testing of each failure case.

## Recommendations

1. **Priority 1**: Fix waif property assignment immutability handling
   - This is a fundamental architectural issue affecting all waif property modifications
   - Requires changes to assignProperty() and possibly the assignment evaluator

2. **Priority 2**: Fix recycle() to check for already-recycled objects
   - Simple validation fix in the recycle builtin

3. **Priority 3**: Fix caller_perms() top-level behavior
   - Should return player when no verb in call stack

4. **Priority 4**: Investigate and fix remaining test failures
   - range_list_single
   - decompile_with_index_operators
   - random_in_valid_range_64bit

## Files Requiring Changes

1. `vm/properties.go` - assignWaifProperty function (waif immutability)
2. `vm/eval.go` - assign function (handle waif property assignment)
3. `builtins/objects.go` - recycle builtin (check already recycled)
4. `builtins/execution.go` - caller_perms builtin (top level handling)
5. `builtins/math.go` - random builtin (64-bit range)
6. `vm/indexing.go` or `vm/decompile.go` - decompiler output format (if exists)

## Next Steps

1. Fix waif property assignment to handle immutability
2. Add validation to recycle() builtin
3. Fix caller_perms() for top-level calls
4. Test remaining failures with toast_oracle
5. Implement fixes for index_and_range and math tests
6. Run full test suite to verify fixes
7. Commit each fix individually with descriptive messages

## Notes

- Test database: Test.db (same database ToastStunt uses)
- Reference server: toast_oracle.exe for validating expected behavior
- Server port for testing: 9309 (to avoid conflicts with other agents)
- All tests are in C:/Users/Q/code/cow_py/tests/conformance/

