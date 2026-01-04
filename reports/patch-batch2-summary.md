# Spec Patch Batch 2 - Summary Report

**Date**: 2026-01-03
**Agent**: spec-patcher
**Prompt**: `prompts/patch-specs-batch2.md`

## Overview

Successfully patched specification files based on divergence reports from stage 1 conformance testing. All changes are targeted fixes for documented spec gaps and accuracy issues.

## Files Modified

### 1. spec/builtins/types.md

**Status**: ✅ Complete (7 targeted fixes)

#### Changes Made:

1. **tostr() with no arguments** (Line 66)
   - **Before**: Spec said raises E_ARGS
   - **After**: Documents actual behavior: returns "" (empty string)
   - **Evidence**: Toast oracle confirms `tostr()` → `""`

2. **tostr() collection formatting** (Lines 71-72, 77)
   - **Before**: Spec showed `tostr({1, 2})` → `"{1, 2}"` (expanded)
   - **After**: Documents actual behavior: `"{list}"` and `"[map]"` (not expanded)
   - **Added note**: Use `toliteral()` for expanded representation
   - **Evidence**: Toast confirms `tostr({1,2})` → `"{list}"`

3. **tostr() error codes** (Line 79)
   - **Before**: Listed E_ARGS error
   - **After**: Changed to "Errors: None"
   - **Reason**: Function never raises errors (no-args returns empty string)

4. **toobj() error behavior** (Line 175)
   - **Added clarification note**: Spec says E_INVARG but actual behavior returns `#0` for unparseable strings
   - **Evidence**: Toast confirms `toobj("abc")` → `#0`, `toobj("")` → `#0`

5. **toerr() marked [Not Implemented]** (Lines 179-185)
   - **Changed**: Section 6 from full documentation to "Not Implemented" status
   - **Evidence**: Toast oracle reports "Unknown built-in function: toerr"
   - **Impact**: Prevents implementers from wasting time on non-existent function

6. **tonum() marked [Not Implemented]** (Lines 189-193)
   - **Changed**: Section 7 from "alias for toint()" to "Not Implemented" status
   - **Evidence**: Toast oracle reports "Unknown built-in function: tonum"
   - **Note**: Added guidance to use `toint()` instead

7. **typename() marked [Not Implemented]** (Lines 241-247)
   - **Changed**: Section 10 from full documentation to "Not Implemented" status
   - **Evidence**: Toast oracle reports "Unknown built-in function: typename"

8. **is_type() marked [Not Implemented]** (Lines 251-257)
   - **Changed**: Section 11 from "ToastStunt extension" to "Not Implemented" status
   - **Evidence**: Function does not exist in Toast despite being labeled as extension
   - **Workaround**: Added note to use `typeof(value) == TYPE_CODE` instead

### 2. spec/builtins/properties.md

**Status**: ✅ Complete (3 targeted fixes)

#### Changes Made:

1. **Permission string documentation** (Lines 69-82)
   - **Added comprehensive format specification**:
     - Valid strings: "", "r", "w", "rw", "rwc" (order matters)
     - Invalid combinations documented with explanations
     - "c" requires "w" permission (chown needs write)
     - Out-of-order strings rejected: "wrc", "crw", etc.
   - **Updated permission character table**: Added "(requires 'w')" note to 'c' permission
   - **Evidence**: Both Toast and Barn reject "c", "rc", "wrc", "crw" with E_INVARG

2. **property_info on built-ins** (Line 100)
   - **Added clarification note**: Built-in properties (name, owner, location, etc.) return E_PROPNF
   - **Reason**: Built-in properties not tracked in property system
   - **Evidence**: `property_info(#0, "name")` → E_PROPNF on both servers

3. **delete_property semantics** (Lines 160-180)
   - **Before**: "Cannot delete inherited properties; use clear_property instead"
   - **After**: Detailed behavior breakdown:
     - Defined property: removes definition
     - Inherited property: succeeds as no-op
     - Local override: removes override, reverts to inherited value
     - E_PROPNF only if property missing from entire chain
   - **Added example**: Demonstrates override removal and no-op behavior
   - **Evidence**: Toast and Barn both allow `delete_property(child, "inherited_prop")` as no-op

### 3. spec/builtins/objects.md

**Status**: ✅ No changes needed (report was clean)

**Reason**: Divergence report found zero behavioral differences between Barn and Toast for all tested object builtins. No spec accuracy issues identified.

## Verification Status

All changes based on verified findings from divergence reports:
- ✅ **divergence-types.md**: 4 spec gaps (non-existent functions) + 3 accuracy issues (tostr behavior, toobj errors)
- ✅ **divergence-objects.md**: Clean report, no changes required
- ✅ **divergence-properties.md**: 3 documentation clarifications (permissions, built-ins, delete semantics)

## Impact Assessment

### For Implementers
- **Positive**: Eliminates confusion about non-existent functions (toerr, tonum, typename, is_type)
- **Positive**: Accurate tostr() examples prevent incorrect implementations
- **Positive**: Clear permission string rules prevent trial-and-error development
- **Positive**: delete_property behavior fully specified for edge cases

### For Test Writers
- **Positive**: tostr() test expectations now match actual behavior
- **Positive**: Permission string validation tests can reference exact valid formats
- **Positive**: delete_property tests can verify no-op and override removal behaviors

### For Spec Auditors
- **Positive**: Four phantom functions removed from audit scope
- **Positive**: Documented behaviors now match reference implementation
- **Positive**: Edge case semantics clearly specified

## Testing Coverage Notes

### Unchanged Behaviors (Still Need Tests)
The following behaviors are documented in spec but have limited conformance test coverage:
- **types.md**: typeof() with BOOL/WAIF types, toint() with decimal strings, special float values
- **properties.md**: Permission enforcement, quota limits, multi-level clear chains, recycled objects

These are **NOT spec accuracy issues** - the spec is correct, tests just need expansion.

## Compliance

✅ **CRITICAL rules followed:**
- ✅ Only made changes documented in divergence reports
- ✅ Preserved existing spec structure
- ✅ Used [Not Implemented] markers for non-existent functions
- ✅ Did NOT add new content beyond report findings
- ✅ Did NOT modify any Go code
- ✅ Did NOT modify objects.md (report was clean)

## Files Generated

1. `spec/builtins/types.md` - Updated with 7 targeted fixes
2. `spec/builtins/properties.md` - Updated with 3 targeted clarifications
3. `reports/patch-batch2-summary.md` - This report

## Conclusion

Batch 2 spec patching is **complete**. All spec gaps identified in stage 1 divergence detection have been addressed:
- 4 non-existent functions marked [Not Implemented]
- 3 tostr() accuracy issues fixed
- 3 property builtin clarifications added
- 0 divergences remain (objects.md was already accurate)

The specification now accurately reflects ToastStunt reference implementation behavior for all tested builtins in the types, objects, and properties categories.

## Next Steps (Out of Scope)

Future work (not part of this patch):
1. Add conformance tests for edge cases noted in divergence reports
2. Audit remaining builtin categories (verbs, tasks, strings, etc.)
3. Update conformance test expectations where spec was corrected (e.g., tostr() with no args)
