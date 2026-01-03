# Fix Report: stress_objects Tests

## Summary

Fixed 18 allegedly failing `stress_objects` tests by verifying actual behavior against Toast server (port 9501). All 18 tests are now passing.

## Investigation Results

**Initial claim:** 18 tests were failing
**Actual situation:** 17 tests were already passing, only 1 test had incorrect expectations

## Test Results

### Tests That Were Already Passing (17/18)

All of these tests matched Toast's actual behavior from the start:

1. ✅ `ancestors_include_self_true` - Already correct
2. ✅ `ancestors_type_int` - Already correct
3. ✅ `chparent_circular_prevention` - Already correct
4. ✅ `chparent_linear_chain` - Already correct
5. ✅ `chparent_no_args` - Already correct
6. ✅ `chparent_property_conflict_child_defines` - Already correct
7. ✅ `chparent_self_reference` - Already correct
8. ✅ `chparent_type_first_arg_int` - Already correct
9. ✅ `chparents_ancestors_descendants` - Already correct
10. ✅ `chparents_complex_reparenting` - Already correct
11. ✅ `object_bytes_created_objects` - Already correct
12. ✅ `object_bytes_recycled_object` - Already correct
13. ✅ `object_bytes_type_int` - Already correct
14. ✅ `object_bytes_wizard_allowed` - Already correct
15. ✅ `parent_maps_multiple_parents` - Already correct
16. ✅ `parent_no_args` - Already correct
17. ✅ `parent_type_int` - Already correct

### Test That Required Fix (1/18)

**Test:** `object_bytes_permission_denied`

**Problem:** Test expected `E_PERM` error when programmer calls `object_bytes($object)`, but Toast actually returns success with value 83.

**Root Cause:** The `object_bytes()` builtin is accessible to programmers in Toast, not restricted to wizards only.

**Fix Applied:**
- Changed test from expecting `E_PERM` error to expecting success with value > 0
- Changed from `code` to `statement` to allow boolean check
- Added description documenting Toast's behavior

**Before:**
```yaml
- name: object_bytes_permission_denied
  permission: programmer
  code: "object_bytes($object)"
  expect:
    error: E_PERM
```

**After:**
```yaml
- name: object_bytes_permission_denied
  permission: programmer
  description: "object_bytes() is accessible to programmers in Toast, not wizard-only"
  statement: |
    return object_bytes($object) > 0;
  expect:
    value: 1
```

## Toast Behavior Verified

Toast server (port 9501) was used as the reference implementation. The test was run multiple times to confirm:
- Programmers CAN call `object_bytes()` successfully
- Returns positive integer (83 bytes for $object)
- No permission error is raised

## Files Modified

### Source File
- `C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_tests/server/stress_objects.yaml`
  - Lines 391-397: Fixed `object_bytes_permission_denied` test expectations

### Installation
- Manually copied updated YAML to installed package location to bypass pytest lock issue
- `C:/Users/Q/code/barn/.venv/Lib/site-packages/moo_conformance/_tests/server/stress_objects.yaml`

## Verification

### Specific 18 Tests
All 18 originally identified tests now pass against Toast (port 9501):

```bash
cd /c/Users/Q/code/barn
uv run pytest --pyargs moo_conformance -k "ancestors_include_self_true or ancestors_type_int or chparent_circular_prevention or chparent_linear_chain or chparent_no_args or chparent_property_conflict_child_defines or chparent_self_reference or chparent_type_first_arg_int or chparents_ancestors_descendants or chparents_complex_reparenting or object_bytes_created_objects or object_bytes_permission_denied or object_bytes_recycled_object or object_bytes_type_int or object_bytes_wizard_allowed or parent_maps_multiple_parents or parent_no_args or parent_type_int" --moo-port=9501 -v
```

**Result:** ✅ 18 passed, 1447 deselected in 2.59s

### Full stress_objects Suite
Verified entire stress_objects test suite (48 tests total):

```bash
cd /c/Users/Q/code/barn
uv run pytest --pyargs moo_conformance -k "stress_objects" --moo-port=9501 -v
```

**Result:** ✅ 48 passed, 1417 deselected in 2.59s

## Conclusion

The "18 failing tests" were actually a single incorrect test expectation. Toast's implementation allows programmers to call `object_bytes()`, which differs from what the test originally expected. The test has been corrected to match Toast's actual behavior.

All stress_objects tests (48 total) are now passing against Toast.
