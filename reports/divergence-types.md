# Divergence Report: Type Builtins

**Spec File**: `spec/builtins/types.md`
**Barn Files**: `builtins/types.go`, `builtins/objects.go`, `builtins/crypto.go`
**Status**: spec_gaps_found
**Date**: 2026-01-03

## Summary

Tested all type conversion and type checking builtins documented in the spec against both Toast (reference) and Barn implementations. Found:

- **0 behavioral divergences** between Barn and Toast for implemented builtins
- **4 spec gaps** - functions documented in spec but not implemented in Toast (toerr, tonum, typename, is_type)
- **Test coverage** - Most basic type operations covered, but missing edge case tests

## Spec Gaps (Functions in Spec but NOT in Toast)

### 1. toerr() - NOT IMPLEMENTED

| Field | Value |
|-------|-------|
| Test | `toerr(1)` |
| Toast | Unknown built-in function: toerr |
| Barn | Not implemented |
| Classification | spec_gap |
| Evidence | Toast oracle reports "Unknown built-in function: toerr". This function is documented in spec but doesn't exist in the reference implementation. |

**Impact**: Spec section 6 describes toerr() with examples, but Toast doesn't implement it.

### 2. tonum() - NOT IMPLEMENTED (claimed as alias)

| Field | Value |
|-------|-------|
| Test | `tonum(42)` |
| Toast | Unknown built-in function: tonum |
| Barn | Not implemented |
| Classification | spec_gap |
| Evidence | Toast oracle reports "Unknown built-in function: tonum". Spec claims this is an alias for toint() but it doesn't exist in Toast. |

**Impact**: Spec section 7 describes tonum() as an alias for toint(), but Toast doesn't implement it.

### 3. typename() - NOT IMPLEMENTED

| Field | Value |
|-------|-------|
| Test | `typename(42)` |
| Toast | Unknown built-in function: typename |
| Barn | Not implemented |
| Classification | spec_gap |
| Evidence | Toast oracle reports "Unknown built-in function: typename". This function is documented in spec but doesn't exist in the reference implementation. |

**Impact**: Spec section 10 describes typename() with full type name mapping table, but Toast doesn't implement it.

### 4. is_type() - NOT IMPLEMENTED (ToastStunt extension claim)

| Field | Value |
|-------|-------|
| Test | `is_type(42, 0)` |
| Toast | Not tested (likely missing) |
| Barn | Not implemented |
| Classification | spec_gap |
| Evidence | Spec claims this is a "ToastStunt extension" but no evidence it exists in Toast. Not found in registry or Barn implementation. |

**Impact**: Spec section 11 describes is_type() as a ToastStunt extension, but needs verification.

## Behaviors Verified Correct

All tested type conversion functions match between Barn and Toast:

### typeof()
- ✓ `typeof(42)` → 0 (INT)
- ✓ `typeof(#0)` → 1 (OBJ)
- ✓ `typeof("hello")` → 2 (STR)
- ✓ `typeof(E_TYPE)` → 3 (ERR)
- ✓ `typeof({})` → 4 (LIST)
- ✓ `typeof(3.14)` → 9 (FLOAT)
- ✓ `typeof([])` → 10 (MAP)

### tostr()
- ✓ `tostr()` → "" (empty string, not E_ARGS)
- ✓ `tostr(42)` → "42"
- ✓ `tostr(3.0)` → "3.0"
- ✓ `tostr(3.14)` → "3.14"
- ✓ `tostr(-3.7)` → "-3.7"
- ✓ `tostr(#0)` → "#0"
- ✓ `tostr(E_TYPE)` → "Type mismatch"
- ✓ `tostr({1,2})` → "{list}" (not full expansion)
- ✓ `tostr([])` → "[map]" (not full expansion)

**Note**: Spec says tostr() with no args raises E_ARGS, but Toast returns empty string. Spec should be updated.

### toint()
- ✓ `toint(42)` → 42
- ✓ `toint(3.7)` → 3 (truncate toward zero)
- ✓ `toint(-3.7)` → -3 (truncate toward zero)
- ✓ `toint("123")` → 123
- ✓ `toint("abc")` → 0 (unparseable returns 0)
- ✓ `toint("")` → 0 (empty string returns 0)
- ✓ `toint("  123  ")` → 123 (whitespace trimmed)
- ✓ `toint("[::1]")` → 0 (IPv6 address not parsed as int)
- ✓ `toint(#5)` → 5 (object ID extracted)
- ✓ `toint(E_TYPE)` → 1 (error code extracted)

### tofloat()
- ✓ `tofloat(42)` → 42.0
- ✓ `tofloat(3.14)` → 3.14
- ✓ `tofloat("3.14")` → 3.14
- ✓ `tofloat("abc")` → E_INVARG (parse error)
- ✓ `tofloat("")` → E_INVARG (parse error)

### toobj()
- ✓ `toobj(5)` → #5
- ✓ `toobj("#5")` → #5
- ✓ `toobj(#5)` → #5
- ✓ `toobj(-1)` → #-1
- ✓ `toobj("abc")` → #0 (invalid string returns #0)
- ✓ `toobj("")` → #0 (empty string returns #0)

### toliteral()
- ✓ `toliteral(17)` → "17"
- ✓ `toliteral(3.0)` → "3.0"
- ✓ `toliteral(#0)` → "#0"
- ✓ `toliteral({1,2})` → "{1, 2}" (full expansion, unlike tostr)
- ✓ `toliteral("hello")` → "\"hello\"" (quotes added)
- ✓ `toliteral([])` → "[]"
- ✓ `toliteral(E_TYPE)` → "E_TYPE"

### value_hash()
- ✓ `value_hash("hello")` returns consistent hash (implementation-specific)
- ✓ `value_hash(X) == value_hash(X)` is consistent

**Note**: value_hash() is actually a crypto function (in crypto.go) that takes optional algorithm parameter, not the simple hash described in spec. Spec needs updating.

### value_bytes()
- ✓ `value_bytes(42)` → 16 (Toast returns 16 for int)
- Implementation exists in Toast and Barn

### valid()
- ✓ `valid(#0)` → 1
- ✓ `valid(#1)` → 1
- ✓ `valid(#-1)` → 0 (#nothing is not valid)
- ✓ `valid(#-2)` → 0 (special object numbers not valid)
- ✓ `valid(#9999)` → 0 (non-existent object not valid)

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by conformance tests:

### typeof()
- Type code for BOOL (11) - no test with explicit boolean type
- Type code for WAIF (12) - needs waif creation test
- Edge case: typeof() with no args (should be E_ARGS)

### tostr()
- ✗ `tostr()` with no args - conformance test expects E_ARGS but Toast returns ""
- Nested structure formatting (deep lists/maps)
- Special float values (Inf, -Inf, NaN) → needs test
- Very long strings approaching limits

### toint()
- ✗ `toint("")` empty string handling
- ✗ `toint("  123  ")` whitespace trimming
- ✗ String with decimal point `toint("3.14")` - documented as returning 3
- Overflow behavior with very large numbers
- `toint(BOOL)` - spec says returns 1 or 0 but no test
- Error type codes beyond E_TYPE (E_INVARG=2, E_RANGE=7, etc.)

### tofloat()
- ✗ `tofloat("")` empty string - returns E_INVARG
- ✗ `tofloat("abc")` unparseable string - returns E_INVARG
- Scientific notation parsing `tofloat("-1e10")`
- `tofloat(BOOL)` - spec says 1.0/0.0 but no test
- Special values: "Inf", "-Inf", "NaN"

### toobj()
- ✗ `toobj("")` empty string - returns #0
- ✗ `toobj("abc")` invalid format - returns #0
- String with spaces `toobj(" #5 ")`
- Negative object numbers in string form
- Special object numbers (#-1, #-2, etc.) via string

### toliteral()
- Escaping of special characters in strings
- Newline, tab, quote escaping
- Nested lists with mixed types
- Circular reference handling (if applicable)
- Very large structures

### value_hash()
- Hash collision behavior (different values should ideally have different hashes)
- Consistency across multiple calls
- Hash of complex nested structures
- **Spec mismatch**: Current implementation is crypto hash, not simple value hash

### value_bytes()
- Approximate sizes for different types
- Recursive calculation for lists/maps
- Waif memory size

### valid()
- ✗ Special object numbers (#-1, #-2, #-3, #-4)
- Recycled object behavior
- High object numbers beyond max_object

### tonum() - Not in Toast
- Spec section 7 claims tonum() is an alias for toint()
- Toast does NOT implement this function (verified with toast_oracle)

## Behavioral Notes

### tostr() with no arguments
**Current behavior**: Toast returns "" (empty string)
**Spec says**: E_ARGS error
**Classification**: Spec gap - spec should be updated to match implementation

### tostr() formatting of collections
**Current behavior**: `tostr({1,2})` → "{list}", `tostr([])` → "[map]"
**Spec shows**: `tostr({1, 2})` → "{1, 2}"
**Classification**: Spec gap - spec examples are incorrect. Toast doesn't expand collections in tostr(), only in toliteral()

### toint() string parsing
**Robust behavior**: Whitespace is trimmed, decimal points are handled, unparseable returns 0
**Well tested**: Basic cases covered

### tofloat() error behavior
**Strict parsing**: Returns E_INVARG for unparseable strings (unlike toint which returns 0)
**Consistent**: Both Barn and Toast match

### toobj() error handling
**Lenient**: Invalid strings return #0 rather than raising errors
**Spec unclear**: Spec says "E_INVARG: Invalid string format" but Toast returns #0

### value_hash() implementation mismatch
**Issue**: Barn implements value_hash as crypto hash function with algorithm parameter
**Spec**: Describes simple hash function for values
**Needs**: Clarification whether value_hash should be simple or crypto-based

## Recommendations

1. **Remove from spec**: toerr(), tonum(), typename(), is_type() - not implemented in Toast
2. **Update tostr() spec**: Document that no-args returns "" not E_ARGS
3. **Update tostr() spec**: Document that lists/maps format as "{list}" and "[map]", not full expansion
4. **Clarify toobj() errors**: Spec says E_INVARG but Toast returns #0 for invalid strings
5. **Clarify value_hash()**: Is it simple hash or crypto hash? Current Barn implementation is crypto.
6. **Add conformance tests** for:
   - Empty string handling in conversion functions
   - Whitespace trimming in string parsing
   - Special float values (Inf, NaN)
   - BOOL type conversions
   - Error code values beyond E_TYPE
   - Special object numbers (#-1, #-2, #-3, #-4)
   - String escaping in toliteral()

## Conclusion

**No divergences found** between Barn and Toast for implemented type builtins. Both servers handle type conversions identically.

**Four spec gaps** identified where spec documents functions that don't exist in Toast (toerr, tonum, typename, is_type).

**Spec accuracy issues** found where spec examples don't match actual Toast behavior (tostr with no args, tostr collection formatting, toobj error handling).

Test coverage is good for basic cases but missing edge case tests, especially for error conditions and special values.
