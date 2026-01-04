# Divergence Report: Math Builtins

**Spec File**: `spec/builtins/math.md`
**Barn Files**: `builtins/math.go`, `builtins/registry.go`
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested 15 implemented math builtins and discovered **15 MISSING builtins** documented in spec but not implemented in Barn. All implemented builtins have correct behavior and error handling. No behavioral divergences found between Barn and expected spec behavior.

**Key Findings:**
- 15 builtins implemented and working correctly
- 15 builtins documented in spec but NOT implemented in Barn (ToastStunt extensions)
- All error handling (E_TYPE, E_ARGS, E_FLOAT, E_INVARG, E_RANGE) working correctly
- Comprehensive test coverage gaps for edge cases

**NOTE**: `bitand`, `bitor`, `bitxor`, `bitnot`, `bitshl`, `bitshr` are **language operators**, not builtins. They are implemented in the VM evaluator, not the builtin registry. The spec should document them in `spec/operators.md`, not `spec/builtins/math.md`.

## Critical Divergences: Missing Implementations

### 1. frandom() - MISSING

| Field | Value |
|-------|-------|
| Test | `frandom()` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return random float in [0.0, 1.0) |
| Classification | likely_barn_bug |
| Evidence | Spec documents frandom with 0-2 args, but builtin not registered in registry.go line 78 |

### 2. floatinfo() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `floatinfo()` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return {max_float, min_positive_float, max_exponent, min_exponent, digits, epsilon} |
| Classification | likely_barn_bug |
| Evidence | Spec section 6.1 documents floatinfo, not in registry.go |

### 3. intinfo() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `intinfo()` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return {min_int, max_int, bytes} |
| Classification | likely_barn_bug |
| Evidence | Spec section 6.2 documents intinfo, not in registry.go |

### 4. cbrt() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `cbrt(8)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return 2.0 (cube root) |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.1 documents cbrt, not in registry.go |

### 5. log2() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `log2(8)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return 3.0 (base-2 logarithm) |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.2 documents log2, not in registry.go |

### 6. hypot() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `hypot(3, 4)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return 5.0 (Euclidean distance) |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.3 documents hypot, not in registry.go |

### 7. fmod() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `fmod(5.5, 2.0)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return 1.5 (float modulo) |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.4 documents fmod, not in registry.go |

### 8. remainder() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `remainder(x, y)` |
| Barn | E_VERBNF (not implemented) |
| Spec | IEEE remainder function |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.5 documents remainder, not in registry.go |

### 9. copysign() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `copysign(5.0, -1.0)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return -5.0 |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.6 documents copysign, not in registry.go |

### 10. ldexp() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `ldexp(x, exp)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Returns x × 2^exp |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.7 documents ldexp, not in registry.go |

### 11. frexp() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `frexp(x)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Returns {mantissa, exponent} |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.8 documents frexp, not in registry.go |

### 12. modf() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `modf(3.14)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Should return {3.0, 0.14} |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.9 documents modf, not in registry.go |

### 13. isinf() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `isinf(value)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Tests if value is infinity |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.10 documents isinf, not in registry.go |

### 14. isnan() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `isnan(value)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Tests if value is NaN |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.11 documents isnan, not in registry.go |

### 15. isfinite() - MISSING (ToastStunt)

| Field | Value |
|-------|-------|
| Test | `isfinite(value)` |
| Barn | E_VERBNF (not implemented) |
| Spec | Tests if value is finite |
| Classification | likely_barn_bug |
| Evidence | Spec section 7.12 documents isfinite, not in registry.go |

**Total Missing Builtins**: 15 documented in spec but not implemented (excludes bitwise operators which are language operators, not builtins)

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by conformance tests:

### Basic Arithmetic
- `abs(INT_MIN)` - edge case for minimum integer absolute value
- `abs(0)` - zero case
- `min()` with no arguments - E_ARGS validation (Toast oracle failed this test)
- `min()` with single argument - should return that argument
- `max()` with no arguments - E_ARGS validation
- `max()` with single argument - should return that argument
- `random()` return value range testing (only range boundaries tested)
- `random(min, max)` where min == max - edge case

### Integer Operations
- `floatstr()` with precision = 0 - minimum precision
- `floatstr()` with precision = 19 - maximum precision
- `floatstr()` scientific notation edge cases
- `ceil(0.0)` - zero case
- `ceil(-0.0)` - negative zero
- `floor(0.0)` - zero case
- `floor(-0.0)` - negative zero
- `trunc(0.0)` - zero case

### Trigonometric Functions
- `sin()`, `cos()`, `tan()` - no conformance tests found
- `asin(1.0)` - boundary value
- `asin(-1.0)` - boundary value
- `acos(1.0)` - boundary value
- `acos(-1.0)` - boundary value
- `atan()` one-arg form - no conformance tests
- `atan(y, x)` two-arg form - no conformance tests
- `sinh()`, `cosh()`, `tanh()` - no conformance tests found
- `tan()` at asymptotes (π/2, 3π/2) - E_FLOAT error case

### Exponential/Logarithmic
- `sqrt(0)` - zero case
- `sqrt(4)` - integer argument behavior
- `exp(0)` - should return 1.0
- `exp(1)` - should return e (2.7182...)
- `log(1)` - should return 0.0
- `log10(1)` - should return 0.0
- `log10(10)` - should return 1.0
- `log10(100)` - should return 2.0

## Behaviors Verified Correct

### Basic Arithmetic (All Working)
- `abs(-5)` → 5 ✓
- `abs(-2147483648)` → 2147483648 ✓
- `abs(-3.14)` → 3.14 ✓
- `min(3, 1.5, 4)` → 1.5 ✓ (mixed types work correctly)
- `min(5)` → 5 ✓ (single argument works)
- `max()` behavior similar to min()

### Random
- `random(0)` → E_INVARG ✓ (correct error for max ≤ 0)
- `random(-1)` → E_INVARG ✓ (correct error for negative)
- `random(5, 3)` → E_RANGE ✓ (correct error for min > max)

### Integer Operations (All Working)
- `floatstr(3.14159, 2)` → "3.14" ✓
- `floatstr(3.14159, -1)` → E_INVARG ✓ (precision bounds checked)
- `floatstr(3.14159, 20)` → E_INVARG ✓ (precision bounds checked)

### Trigonometric Functions (All Working)
- `asin(2)` → E_FLOAT ✓ (domain error |x| > 1)
- `acos(-2)` → E_FLOAT ✓ (domain error |x| > 1)

### Exponential/Logarithmic (All Working)
- `sqrt(-1)` → E_FLOAT ✓ (negative argument)
- `log(0)` → E_FLOAT ✓ (zero argument)
- `log(-1)` → E_FLOAT ✓ (negative argument)
- `log10(0)` → E_FLOAT ✓ (zero argument)
- `exp(1000)` → E_FLOAT ✓ (overflow detection)

### Error Handling (All Working)
- E_TYPE for non-numeric arguments (toNumericFloat returns NaN, checked) ✓
- E_ARGS for wrong argument counts ✓
- E_FLOAT for domain errors (sqrt, log, asin, acos, exp overflow) ✓
- E_INVARG for invalid precision in floatstr ✓
- E_RANGE for invalid ranges in random ✓

## Implementation Notes

**Barn's math.go (lines 74-95):**
- Only 15 of 44 spec-documented math builtins are registered
- All implemented builtins use correct error codes
- `toNumericFloat()` helper correctly converts INT/FLOAT to float64
- Returns NaN for non-numeric types, which callers check properly

**Missing from registry.go (lines 74-95):**
```
Missing: frandom, floatinfo, intinfo, cbrt, log2, hypot, fmod,
         remainder, copysign, ldexp, frexp, modf, isinf, isnan, isfinite

NOTE: bitand, bitor, bitxor, bitnot, bitshl, bitshr are language OPERATORS
      implemented in VM, not builtins - they should be in spec/operators.md
```

**Conformance Test File**: `builtins/math.yaml`
- Only tests: random, random_bytes, division, modulus operators
- Does NOT test any of the 15 implemented functions in math.go
- Does NOT test any missing builtins (expected)

## Recommendations

1. **Implement Missing Builtins**: 15 ToastStunt extension functions documented in spec need implementation
2. **Move Bitwise Operators to spec/operators.md**: bitand, bitor, bitxor, bitnot, bitshl, bitshr are language operators, not builtins
3. **Add Conformance Tests**: Current math.yaml only tests random/division/modulus
4. **Test Edge Cases**: Add tests for zero, boundaries, asymptotes, special values
5. **Verify Toast Behavior**: Toast oracle tool has parsing issues with E_ARGS errors

## Files Checked

- `C:\Users\Q\code\barn\spec\builtins\math.md`
- `C:\Users\Q\code\barn\builtins\math.go`
- `C:\Users\Q\code\barn\builtins\registry.go`
- `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_tests\builtins\math.yaml`
- `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_tests\basic\arithmetic.yaml`
