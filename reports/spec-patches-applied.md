# Spec Patches Applied - Gap Resolution

Date: 2025-12-24

## Summary

All 4 critical gaps from the blind implementor audit have been resolved through research of ToastStunt (C++) and moo_interp (Python) source code, then patched into the specification.

---

## GAP-001: FLOAT special values (NaN/Infinity)

### Status: **RESOLVED**

### Research Summary
Checked ToastStunt `src/numbers.cc` and `src/include/my-math.h`:
- Division by zero explicitly checked before computation (line 322-325)
- Macro `IS_REAL(x)` = `(-DBL_MAX <= (x) && (x) <= DBL_MAX)` rejects NaN/Infinity
- All float operations check `!IS_REAL(d)` after computation
- Math functions (sqrt, pow, sin, etc.) check for domain errors

### Resolution
NaN and Infinity **cannot exist** in MOO:
- `1.0 / 0.0` → `E_DIV` (NOT Infinity)
- `0.0 / 0.0` → `E_DIV` (NOT NaN)
- Any operation producing NaN/Infinity → `E_FLOAT`

### Spec Changes
**File:** `spec/types.md`

**Lines 56-59** (was vague, now precise):
```markdown
**Special values:** NaN and Infinity **cannot exist** in MOO.
- Division by zero (int or float): raises `E_DIV` before computation
- Operations that would produce NaN/Infinity: raise `E_FLOAT`
  - Example: `sqrt(-1.0)` → `E_FLOAT`
  - Example: `1.0e308 * 1.0e308` → `E_FLOAT` (overflow)
- Any result outside `[-DBL_MAX, DBL_MAX]` raises `E_FLOAT`
```

---

## GAP-002: INT/FLOAT comparison contradiction

### Status: **RESOLVED**

### Research Summary
Checked ToastStunt `src/numbers.cc:194-208`:
- Comment: "All implementations are strict, not performing any coercions"
- `do_equals`: returns 0 if types don't match
- Arithmetic operations: `E_TYPE` if types don't match
- Confirmed: NO automatic INT/FLOAT coercion

### Resolution
**operators.md:309 was CORRECT**, types.md:242 was WRONG:
- `1 == 1.0` → `0` (false, different types)
- `1 < 1.0` → `E_TYPE`
- `1 + 1.0` → `E_TYPE`

MOO uses **type-strict** comparison and arithmetic.

### Spec Changes

**File:** `spec/types.md`

**Line 242** (was: "INT vs FLOAT → compare as FLOAT"):
```markdown
| Arithmetic | INT + FLOAT → `E_TYPE` (no coercion) |
| Comparison | INT vs FLOAT → `0` for `==`, `E_TYPE` for `<`/`>` |
```

**File:** `spec/operators.md`

**Line 305** (added clarification):
```markdown
**Type strictness:** Equality requires **exact type match**. INT and FLOAT are never equal.
```

**Line 326** (added clarification):
```markdown
**Type strictness:** Both operands must be the **same type**. No INT/FLOAT coercion.
```

**Lines 393, 420, 434, 455** (added `E_TYPE` rows):
```markdown
| INT | FLOAT | `E_TYPE` (no coercion) |
```

---

## GAP-003: INT overflow/underflow

### Status: **RESOLVED**

### Research Summary
Checked ToastStunt `src/include/structures.h` and `src/numbers.cc`:
- MAXINT = `9223372036854775807LL`
- MININT = `-9223372036854775807LL` (NOT -9223372036854775808!)
- Arithmetic uses `a.v.num op b.v.num` with **no overflow checking**
- C++ signed integer overflow is undefined behavior

### Resolution
Integer overflow behavior is **undefined**:
- `9223372036854775807 + 1` → undefined (typically wraps)
- Implementations may wrap, saturate, or raise errors
- Portable code must avoid overflow

### Spec Changes
**File:** `spec/types.md`

**Line 33** (added overflow documentation):
```markdown
**Overflow behavior:** Undefined. Integer overflow in arithmetic operations (`+`, `-`, `*`) is not checked and may wrap, saturate, or produce unpredictable results. Portable code must avoid overflow.
```

---

## GAP-004: UTF-8 vs binary strings

### Status: **RESOLVED**

### Research Summary
Checked ToastStunt `src/include/storage.h` and `src/list.cc`:
- `memo_strlen(X)` → `strlen(X)` (byte count)
- `bf_length` for strings: `r.v.num = memo_strlen(...)` (line 643)
- String indexing uses byte offsets
- No UTF-8 aware functions found

### Resolution
Strings are **binary** (byte sequences):
- `length("日")` → `3` (UTF-8 bytes: E6 97 A5)
- `"日"[1]` → first byte (0xE6)
- Indexing is **byte-based**, not character-based
- No UTF-8 mode

### Spec Changes
**File:** `spec/types.md`

**Lines 69-73** (was: "UTF-8 (ToastStunt) or binary"):
```markdown
**Encoding:** Binary (byte sequences). Strings are **not** character-aware.

**Indexing:** 1-based **byte indexing**. Each index accesses one byte, not one character.
- UTF-8 text: `"日"` is 3 bytes, `"日"[1]` returns the first byte (0xE6)
- `length("日")` returns `3` (byte count, not character count)
```

---

## Files Modified

1. `spec/types.md`
   - Lines 33: Added integer overflow behavior
   - Lines 58-63: Clarified NaN/Infinity handling
   - Lines 69-73: Changed to binary strings with byte indexing
   - Lines 247-248: Removed INT/FLOAT coercion

2. `spec/operators.md`
   - Line 305: Added type strictness note for equality
   - Line 326: Added type strictness note for ordering
   - Lines 393, 420, 434, 455: Added E_TYPE for INT/FLOAT mixing
   - Line 458: Clarified division by zero for both int and float

---

## Follow-up Notes

### Potential Future Gaps

1. **String escaping:** How are non-printable bytes represented in string literals?
2. **List modification:** What happens to references when a list is modified (COW semantics)?
3. **Map iteration order:** Is it deterministic or implementation-defined?
4. **Error propagation:** Do errors bubble up through catch expressions correctly?

### Testing Recommendations

Create conformance tests for:
1. `1 == 1.0` → `0` (not 1)
2. `1 + 1.0` → `E_TYPE` (not coercion)
3. `1.0 / 0.0` → `E_DIV` (not Infinity)
4. `length("日")` → `3` (bytes, not characters)
5. Integer overflow behavior (document as undefined)

---

## Confidence Level

**HIGH** - All gaps resolved with direct source code evidence from ToastStunt.

No ambiguity remains. Specs now match actual implementation behavior.
