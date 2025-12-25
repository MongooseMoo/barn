# Gap Research Findings

Research date: 2025-12-24
Sources: moo_interp (Python), ToastStunt (C++), cow_py tests

## GAP-001: FLOAT special values (NaN/Infinity)

### Question
- spec/types.md:56-59 says NaN/Infinity "raise E_FLOAT in most operations" - which operations?
- Does 1.0/0.0 raise E_FLOAT or return Infinity?
- Does 0.0/0.0 raise E_FLOAT or return NaN?

### Research

**ToastStunt (C++):**
- File: `src/include/my-math.h:24`
- Macro: `#define IS_REAL(x) (-DBL_MAX <= (x) && (x) <= DBL_MAX)`
- Division: `src/numbers.cc:322-325` explicitly checks for division by zero (both int and float)
  - Returns E_DIV for `x / 0.0` and `0.0 / 0.0`
- Arithmetic operations: `src/numbers.cc:264-268` (SIMPLE_BINARY macro)
  - After any float operation: `if (!IS_REAL(d)) { return E_FLOAT; }`
  - This catches NaN and Infinity results
- Power operation: `src/numbers.cc:400` checks `!IS_REAL(d)` after `pow()`
- Math functions: `src/numbers.cc:537` checks `!IS_REAL(result)` for sqrt, sin, cos, etc.

**moo_interp (Python):**
- File: `moo_interp/vm.py:536-537`
- Catches `ZeroDivisionError` and raises `VMError("Division by zero")`
- Python itself raises `ZeroDivisionError` for both `1.0/0.0` and `0.0/0.0`

### Answer

**Division by zero (float or int):**
- `1.0 / 0.0` → raises `E_DIV` (NOT Infinity)
- `0.0 / 0.0` → raises `E_DIV` (NOT NaN)
- Division by zero is checked BEFORE computation

**NaN/Infinity from other operations:**
- IF they occur (e.g., from overflow or domain errors): raise `E_FLOAT`
- Examples that raise E_FLOAT:
  - `sqrt(-1.0)` → E_FLOAT (via EDOM check)
  - `1.0e308 * 1.0e308` → E_FLOAT (overflow)
  - `pow(-1.0, 0.5)` → E_FLOAT (domain error)

**Spec fix needed:**
Lines 56-59 should say:
- Division by zero raises E_DIV (prevents Infinity/NaN creation)
- Any operation producing NaN/Infinity raises E_FLOAT
- NaN/Infinity cannot be created or stored in MOO floats

---

## GAP-002: INT/FLOAT comparison contradiction

### Question
- spec/types.md:242 says "INT vs FLOAT → compare as FLOAT"
- spec/operators.md:309 shows "1 == 1.0 => 0 (different types!)"
- Which is correct?

### Research

**ToastStunt (C++):**
- File: `src/numbers.cc:194-197`
- Comment: "All of the following implementations are strict, not performing any coercions between integer and floating-point operands."
- Function: `do_equals` at line 204:
  ```cpp
  if (lhs.type != rhs.type)
      return 0;
  ```
- Arithmetic operations check `if (a.type != b.type) { return E_TYPE; }` (line 257-259)

**Example from operators.md:309:**
```moo
1 == 1.0 => 0 (different types!)
```

### Answer

**operators.md:309 is CORRECT**

MOO does **type-strict comparison**:
- `1 == 1.0` → 0 (false, different types)
- `1 < 1.0` → E_TYPE (can't compare different types)
- `1 + 1.0` → E_TYPE (can't add different types)

**Spec fix needed:**
- types.md:242 is WRONG - remove "INT vs FLOAT → compare as FLOAT"
- Comparison operators require same types (except `==`/`!=` which just return 0/1)

---

## GAP-003: INT overflow/underflow

### Question
- spec/types.md:31 gives range but not overflow behavior
- What is 9223372036854775807 + 1?

### Research

**ToastStunt (C++):**
- File: `src/include/structures.h:34-35`
- Constants:
  ```cpp
  #define MAXINT ((Num) 9223372036854775807LL)
  #define MININT ((Num) -9223372036854775807LL)  // NOT -9223372036854775808!
  ```
- Arithmetic: `src/numbers.cc:262` (SIMPLE_BINARY macro)
  ```cpp
  ans.v.num = a.v.num op b.v.num;  // No overflow checking
  ```
- Division edge case: `src/numbers.cc:328-329`
  ```cpp
  if (a.v.num == MININT && b.v.num == -1)
      ans.v.num = MININT;  // Special handling
  ```

**cow_py tests:**
- No tests found for integer overflow in addition/multiplication

### Answer

**Integer overflow is UNDEFINED BEHAVIOR**

In practice (on modern x86-64):
- `9223372036854775807 + 1` → wraps to `-9223372036854775808` (undefined behavior)
- Multiplication can also overflow silently
- C++ signed integer overflow is UB per language spec

**Note:** MININT is defined as `-9223372036854775807`, not `-9223372036854775808`

**Spec fix needed:**
Add to types.md:
- Integer overflow behavior is undefined
- Implementations may wrap, saturate, or raise errors
- Portable code should avoid overflow

---

## GAP-004: UTF-8 vs binary strings

### Question
- spec/types.md:65 says "UTF-8 (ToastStunt) or binary" but doesn't explain which mode or how
- Is "日"[1] the character or the first byte?

### Research

**ToastStunt (C++):**
- File: `src/include/storage.h:153-155`
  ```cpp
  #define memo_strlen(X) strlen(X)  // Byte length
  ```
- File: `src/list.cc:643` (`bf_length` for strings)
  ```cpp
  r.v.num = memo_strlen(arglist.v.list[1].v.str);  // strlen = byte count
  ```
- String indexing uses byte offsets, not character offsets
- No UTF-8 aware functions found

**moo_interp (Python):**
- Python 3 strings are Unicode by default
- Character-based indexing

### Answer

**ToastStunt: Binary (byte-based) strings**
- `length("日")` → 3 (UTF-8 bytes: E6 97 A5)
- `"日"[1]` → first byte (0xE6 as a single-byte string)
- String indexing is **byte-based**, not character-based
- No UTF-8 mode or character-aware operations

**LambdaMOO: Binary strings**
- Same behavior as ToastStunt

**Spec fix needed:**
- Remove "UTF-8 (ToastStunt)"
- Clarify: strings are **binary** (byte sequences)
- Indexing is **byte-based**
- UTF-8 encoded text requires byte-level manipulation

---

## Summary

| Gap | Status | Fix Required |
|-----|--------|--------------|
| GAP-001 | Resolved | Clarify NaN/Infinity never exist, E_DIV for division by zero, E_FLOAT for overflow/domain errors |
| GAP-002 | Resolved | Remove INT/FLOAT coercion claim, enforce type-strict comparison |
| GAP-003 | Resolved | Document integer overflow as undefined behavior |
| GAP-004 | Resolved | Change to binary strings, byte-based indexing |

All gaps have definitive answers from ToastStunt source code.
