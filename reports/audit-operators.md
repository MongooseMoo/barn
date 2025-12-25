# Operators Feature Audit Report

**Feature:** Operators (Arithmetic, Comparison, Logical, Bitwise, Special)
**Spec Files:** `spec/operators.md`, `spec/types.md`, `spec/grammar.md`
**Audit Date:** 2025-12-24
**Auditor:** Blind Implementor (spec-only)

---

## Executive Summary

The operators specification is **moderately complete** but has **critical gaps** in:
1. **Precedence/associativity edge cases** (mixed operator chains)
2. **Short-circuit evaluation specifics** (side effect ordering)
3. **Type coercion ambiguities** (power operator with mixed types)
4. **Error propagation** (which errors take precedence when multiple occur)
5. **Edge case behaviors** (division truncation direction, shift overflow, negative exponents)

**Impact:** An implementor would need to make ~15-20 guesses, likely producing subtle incompatibilities with reference servers.

---

## Gaps Identified

### 1. Precedence & Associativity

- id: GAP-OPS-001
  feature: "operator precedence"
  spec_file: "spec/operators.md"
  spec_section: "1. Precedence Table"
  gap_type: test
  question: |
    What is the evaluation order for `a && b || c`?
    The spec lists `||` (level 6) and `&&` (level 7), both left-associative,
    but doesn't provide examples of chained boolean operators to clarify
    whether this parses as `(a && b) || c` or requires explicit parentheses.
  impact: |
    Implementor must test reference server to determine correct grouping.
    Precedence tables are notoriously misread without concrete examples.
  suggested_addition: |
    Add examples section:
    ```moo
    a && b || c       // Equivalent to: (a && b) || c
    a || b && c       // Equivalent to: a || (b && c)
    ```

- id: GAP-OPS-002
  feature: "right-associative operators"
  spec_file: "spec/operators.md"
  spec_section: "12. Power Operator"
  gap_type: assume
  question: |
    For assignment chains like `a = b = c`, does right-associativity mean
    `a = (b = c)` (evaluate c, assign to b, assign result to a)?
    Spec says assignment is right-associative but doesn't show multi-assignment.
  impact: |
    Implementor might incorrectly implement as `(a = b) = c`, which would
    attempt to assign to the result of the first assignment.
  suggested_addition: |
    Add to section 2.1:
    ```moo
    a = b = 5;        // Equivalent to: a = (b = 5)
                      // Both a and b become 5
    ```

- id: GAP-OPS-003
  feature: "operator mixing"
  spec_file: "spec/operators.md"
  spec_section: "1. Precedence Table"
  gap_type: test
  question: |
    What is the evaluation order for `2 + 3 * 4 ^ 5`?
    The spec lists precedence levels but doesn't clarify if `^` (level 15,
    right-assoc) binds before `*` (level 14, left-assoc) in this context.
  impact: |
    Standard math says this is `2 + (3 * (4 ^ 5))`, but implementor must
    verify that right-associativity of `^` doesn't affect this.
  suggested_addition: |
    Add to section 12:
    ```moo
    2 + 3 * 4 ^ 5     // Equivalent to: 2 + (3 * (4 ^ 5))
                      // Power binds first, then multiply, then add
    ```

---

### 2. Short-Circuit Evaluation

- id: GAP-OPS-004
  feature: "logical OR short-circuit"
  spec_file: "spec/operators.md"
  spec_section: "7.1 Logical OR"
  gap_type: guess
  question: |
    For `func_with_side_effects() || other_func()`, if the first function
    returns truthy, is the second function called at all?
    Spec says "don't evaluate right" but doesn't clarify if this applies to
    function calls with side effects.
  impact: |
    Critical for side effects. If implementor guesses wrong, code like
    `valid(obj) || throw_error()` might execute throw_error even when valid.
  suggested_addition: |
    Add to section 7.1:
    ```moo
    // Short-circuit guarantees NO evaluation of right operand:
    valid(obj) || crash()     // crash() NEVER called if valid(obj) is truthy
    x = 0 || (y = 5)          // y is assigned 5, x gets 5
    ```

- id: GAP-OPS-005
  feature: "logical AND short-circuit"
  spec_file: "spec/operators.md"
  spec_section: "7.2 Logical AND"
  gap_type: guess
  question: |
    For `a && b && c`, if `a` is falsy, are both `b` and `c` skipped?
    Spec says "don't evaluate right" but doesn't specify behavior for chains.
  impact: |
    Implementor might evaluate left-to-right but check all operands first,
    missing the short-circuit optimization.
  suggested_addition: |
    Add to section 7.2:
    ```moo
    a && b && c       // If a is falsy, neither b nor c are evaluated
    valid(obj) && obj.dangerous_property && obj:risky_verb()
                      // Safe: properties/verbs only accessed if valid(obj)
    ```

- id: GAP-OPS-006
  feature: "short-circuit return values"
  spec_file: "spec/operators.md"
  spec_section: "7.1 Logical OR"
  gap_type: assume
  question: |
    The spec says `||` returns "left value if truthy, else right".
    For `0 || ""`, does this return `0` or `""`?
    Both are falsy, so "right" suggests `""`, but this needs confirmation.
  impact: |
    Code like `config_value || default` might get wrong type if the falsy
    value is returned instead of the right operand.
  suggested_addition: |
    Add explicit examples:
    ```moo
    0 || ""           => ""     // Returns right operand (both falsy)
    "" || 0           => 0      // Returns right operand (both falsy)
    1 || crash()      => 1      // Returns left (truthy), crash() not called
    ```

---

### 3. Arithmetic Operators

- id: GAP-OPS-007
  feature: "integer division truncation"
  spec_file: "spec/operators.md"
  spec_section: "11.4 Division"
  gap_type: test
  question: |
    The spec says "truncated toward zero" for INT / INT.
    For `-7 / 2`, does this return `-3` (toward zero) or `-4` (floor)?
    "Toward zero" suggests `-3`, but implementor must verify.
  impact: |
    Python uses floor division (`-7 // 2 = -4`), C uses truncation (`-7 / 2 = -3`).
    Wrong choice breaks compatibility.
  suggested_addition: |
    Add to section 11.4:
    ```moo
    7 / 2             => 3      // Truncate toward zero
    -7 / 2            => -3     // Truncate toward zero (not floor)
    -7 / -2           => 3      // Truncate toward zero
    ```

- id: GAP-OPS-008
  feature: "modulo sign"
  spec_file: "spec/operators.md"
  spec_section: "11.5 Modulo"
  gap_type: test
  question: |
    Spec says "Result has same sign as dividend (left)".
    For `-7 % 3`, does this return `-1` (same sign as -7) or `2`?
    Needs confirmation with examples.
  impact: |
    Different languages have different modulo semantics:
    - C: `-7 % 3 = -1` (sign of dividend)
    - Python: `-7 % 3 = 2` (sign of divisor)
  suggested_addition: |
    Add to section 11.5:
    ```moo
    7 % 3             => 1      // Positive dividend
    -7 % 3            => -1     // Negative dividend, result negative
    7 % -3            => 1      // Positive dividend, result positive
    -7 % -3           => -1     // Negative dividend, result negative
    ```

- id: GAP-OPS-009
  feature: "power operator with negative base"
  spec_file: "spec/operators.md"
  spec_section: "12. Power Operator"
  gap_type: test
  question: |
    For `(-2) ^ 3`, does this return `-8` (INT) or raise E_FLOAT?
    Spec says INT ^ INT → INT, but doesn't clarify negative bases.
  impact: |
    Implementor might incorrectly reject negative bases, breaking valid code.
  suggested_addition: |
    Add to section 12:
    ```moo
    (-2) ^ 3          => -8     // Negative base with odd exponent
    (-2) ^ 4          => 16     // Negative base with even exponent
    (-2) ^ 0          => 1      // Any base to power 0
    ```

- id: GAP-OPS-010
  feature: "power operator with zero/negative exponent"
  spec_file: "spec/operators.md"
  spec_section: "12. Power Operator"
  gap_type: guess
  question: |
    For `2 ^ 0`, does this return `1` (INT)?
    For `2 ^ (-1)`, does this return `0` (truncated INT) or raise E_TYPE?
    Spec doesn't cover zero or negative exponents.
  impact: |
    Standard math: `x^0 = 1`, `x^(-n) = 1/(x^n)`.
    If implementor returns FLOAT for negative exponents, breaks type expectations.
  suggested_addition: |
    Add to section 12:
    ```moo
    2 ^ 0             => 1      // Any base to power 0 is 1 (INT)
    2 ^ (-1)          => E_TYPE // Negative exponents not supported for INT base
                                // (would require FLOAT return)
    2.0 ^ (-1)        => 0.5    // FLOAT base supports negative exponents
    ```

- id: GAP-OPS-011
  feature: "power operator type mixing"
  spec_file: "spec/operators.md"
  spec_section: "12. Power Operator"
  gap_type: assume
  question: |
    For `2 ^ 3.5`, is this allowed? Spec says:
    - INT ^ INT → INT
    - FLOAT ^ INT → FLOAT
    - FLOAT ^ FLOAT → FLOAT
    But doesn't explicitly forbid INT ^ FLOAT.
  impact: |
    Implementor might allow INT ^ FLOAT → FLOAT (like Python),
    or raise E_TYPE. Needs explicit guidance.
  suggested_addition: |
    Add to section 12:
    ```moo
    2 ^ 3.5           => E_TYPE // INT base with FLOAT exponent not allowed
                                // Use 2.0 ^ 3.5 instead
    ```

---

### 4. Comparison Operators

- id: GAP-OPS-012
  feature: "equality with float precision"
  spec_file: "spec/operators.md"
  spec_section: "9.1 Equality"
  gap_type: guess
  question: |
    For `0.1 + 0.2 == 0.3`, does this return `1` or `0`?
    Due to floating-point precision, these may not be exactly equal.
    Does MOO use exact bitwise comparison or epsilon tolerance?
  impact: |
    If implementor uses bitwise equality (common), this returns `0`.
    If epsilon comparison, returns `1`.
    Affects all floating-point equality tests.
  suggested_addition: |
    Add to section 9.1:
    ```moo
    1.0 == 1.0        => 1      // Exact equality
    0.1 + 0.2 == 0.3  => 0      // Floating-point precision issue
                                // Use abs(a - b) < epsilon for tolerance
    1 == 1.0          => 0      // Different types (INT vs FLOAT)
    ```

- id: GAP-OPS-013
  feature: "comparison chaining"
  spec_file: "spec/operators.md"
  spec_section: "9.2 Ordering"
  gap_type: test
  question: |
    For `1 < 2 < 3`, does this:
    (a) Parse as `(1 < 2) < 3` (compare 1 to 2, get 1, compare 1 < 3)?
    (b) Parse as `1 < (2 < 3)` (compare 2 to 3, get 1, compare 1 < 1)?
    (c) Raise a parse error?
    Spec says comparison is left-associative but doesn't show chaining.
  impact: |
    Python allows chaining (`1 < 2 < 3` means `1 < 2 and 2 < 3`).
    C-style languages evaluate as `(1 < 2) < 3` → `1 < 3` → `1`.
  suggested_addition: |
    Add to section 9.2:
    ```moo
    1 < 2 < 3         // Equivalent to: (1 < 2) < 3
                      // NOT: 1 < 2 and 2 < 3 (not Python-style)
                      // Evaluates to: 1 < 3 => 1
    ```

- id: GAP-OPS-014
  feature: "string comparison case sensitivity"
  spec_file: "spec/operators.md"
  spec_section: "9.2 Ordering"
  gap_type: assume
  question: |
    For `"A" < "a"`, does this return `1` or `0`?
    Spec says "lexicographic (case-sensitive)" but doesn't clarify if
    uppercase sorts before or after lowercase.
  impact: |
    ASCII: uppercase letters (65-90) sort before lowercase (97-122).
    So `"A" < "a"` should return `1`, but implementor might use
    locale-sensitive sorting.
  suggested_addition: |
    Add to section 9.2:
    ```moo
    "A" < "a"         => 1      // Uppercase sorts before lowercase (ASCII)
    "a" < "A"         => 0      //
    "Z" < "a"         => 1      // All uppercase before all lowercase
    ```

- id: GAP-OPS-015
  feature: "list/map equality depth"
  spec_file: "spec/operators.md"
  spec_section: "9.1 Equality"
  gap_type: assume
  question: |
    For `{1, {2, 3}} == {1, {2, 3}}`, does this perform deep comparison?
    Spec says "Lists/maps compared by value (deep)" but doesn't define
    "deep" explicitly.
  impact: |
    Implementor might implement shallow comparison (reference equality),
    breaking code that relies on structural equality.
  suggested_addition: |
    Add to section 9.1:
    ```moo
    {1, 2} == {1, 2}             => 1      // Deep equality
    {1, {2, 3}} == {1, {2, 3}}   => 1      // Recursive deep equality
    {} == {}                     => 1      // Empty lists equal
    ["a" -> 1] == ["a" -> 1]     => 1      // Deep equality for maps
    ```

---

### 5. Bitwise Operators

- id: GAP-OPS-016
  feature: "shift operator overflow"
  spec_file: "spec/operators.md"
  spec_section: "8.5 Left Shift"
  gap_type: guess
  question: |
    For `1 << 100`, does this:
    (a) Return 0 (all bits shifted out)?
    (b) Raise E_RANGE (shift count too large)?
    (c) Wrap modulo 64 (1 << (100 % 64))?
    Spec doesn't specify maximum shift count.
  impact: |
    Different platforms have different behaviors:
    - C: undefined behavior for shift >= 64
    - Java: wraps shift count (n % 64)
    - Python: unlimited precision (huge result)
  suggested_addition: |
    Add to section 8.5:
    ```moo
    1 << 64           => 0      // Shift >= 64 zeros all bits (or E_RANGE)
    1 << 100          => E_RANGE // Shift count exceeds type width
    1 << -1           => E_RANGE // Negative shift count invalid
    ```

- id: GAP-OPS-017
  feature: "right shift sign extension"
  spec_file: "spec/operators.md"
  spec_section: "8.6 Right Shift"
  gap_type: test
  question: |
    For `-8 >> 1`, does this return `-4` (arithmetic shift, sign-extended)?
    Spec says "arithmetic right shift (preserves sign)" but needs examples
    to confirm sign-extension behavior.
  impact: |
    Logical shift: `-8 >> 1` → large positive number (fills with 0)
    Arithmetic shift: `-8 >> 1` → `-4` (fills with 1, preserving sign)
  suggested_addition: |
    Add to section 8.6:
    ```moo
    8 >> 1            => 4      // Positive: fills with 0
    -8 >> 1           => -4     // Negative: sign-extended (fills with 1)
    -1 >> 10          => -1     // All-ones pattern remains all-ones
    ```

- id: GAP-OPS-018
  feature: "bitwise NOT on negative numbers"
  spec_file: "spec/operators.md"
  spec_section: "8.4 Bitwise NOT"
  gap_type: test
  question: |
    For `~(-1)`, does this return `0`?
    Ones complement of all-ones (−1 in two's complement) should be all-zeros (0).
    Needs confirmation with examples.
  impact: |
    Implementor might incorrectly compute bitwise NOT, breaking bit manipulation.
  suggested_addition: |
    Add to section 8.4:
    ```moo
    ~0                => -1     // All zeros → all ones
    ~(-1)             => 0      // All ones → all zeros
    ~5                => -6     // 00000101 → 11111010 (two's complement)
    ```

---

### 6. Special Operators

- id: GAP-OPS-019
  feature: "ternary operator evaluation order"
  spec_file: "spec/operators.md"
  spec_section: "3. Ternary Operator"
  gap_type: assume
  question: |
    For `condition ? a = 1 | b = 2`, does the ternary bind tighter than
    assignment, or do I need parentheses?
    Spec says ternary is level 2, assignment is level 1 (lower),
    so this should parse as `condition ? (a = 1) | (b = 2)`.
  impact: |
    Implementor might require `(condition ? a : b) = value`, breaking
    idioms like `flag ? x | y = compute()`.
  suggested_addition: |
    Add to section 3:
    ```moo
    flag ? x = 1 | y = 2          // Valid: assigns x or y based on flag
    result = flag ? "yes" | "no"  // Valid: ternary result assigned to result
    ```

- id: GAP-OPS-020
  feature: "catch expression with multiple errors"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: guess
  question: |
    For `\`x / y ! E_DIV, E_TYPE => 0\``, if both errors could occur,
    which is caught first?
    Spec shows syntax but doesn't clarify evaluation order.
  impact: |
    Implementor might catch only the first error in the list, or might
    match any error in the list (correct behavior).
  suggested_addition: |
    Add to section 4:
    ```moo
    \`x / y ! E_DIV, E_TYPE => 0\`  // Catches EITHER error, returns 0
    \`x / y ! E_DIV => 0\`           // Catches only E_DIV, E_TYPE propagates
    \`x / y ! ANY => 0\`             // Catches all errors
    ```

- id: GAP-OPS-021
  feature: "splice operator in function calls"
  spec_file: "spec/operators.md"
  spec_section: "5. Splice Operator"
  gap_type: test
  question: |
    For `func(1, @{2, 3}, 4)`, does this call `func` with 4 arguments
    (1, 2, 3, 4) or raise a syntax error?
    Spec says splice "spreads list as arguments" but doesn't show
    mixed splice and non-splice arguments.
  impact: |
    Implementor might only allow `func(@args)` (all args spliced),
    breaking code that mixes spliced and literal arguments.
  suggested_addition: |
    Add to section 5:
    ```moo
    func(@{1, 2, 3})              // Calls func(1, 2, 3)
    func(1, @{2, 3}, 4)           // Calls func(1, 2, 3, 4)
    func(@list1, @list2)          // Calls func with concatenation of both
    ```

- id: GAP-OPS-022
  feature: "scatter assignment with too many values"
  spec_file: "spec/operators.md"
  spec_section: "6. Scatter Assignment"
  gap_type: test
  question: |
    For `{a, b} = {1, 2, 3}`, does this:
    (a) Assign a=1, b=2, ignore 3?
    (b) Raise E_ARGS (too many values)?
    Spec shows examples with exact matches or rest targets, but not overflow.
  impact: |
    Implementor might silently drop extra values, hiding bugs.
  suggested_addition: |
    Add to section 6:
    ```moo
    {a, b} = {1, 2, 3}            => E_ARGS // Too many values, no rest target
    {a, b, @rest} = {1, 2, 3}     // a=1, b=2, rest={3} (correct)
    {a, @rest} = {1}              // a=1, rest={} (empty rest)
    ```

---

### 7. Operator Interactions

- id: GAP-OPS-023
  feature: "assignment in boolean context"
  spec_file: "spec/operators.md"
  spec_section: "2.1 Simple Assignment"
  gap_type: test
  question: |
    For `if (x = 5)`, does this assign 5 to x and then test truthiness of 5?
    Spec says assignment "returns the assigned value" but doesn't show
    assignment used as a boolean condition.
  impact: |
    C-style languages allow this (common bug source: `if (x = 5)` vs `if (x == 5)`).
    If MOO allows, implementor needs to ensure truthiness check happens.
  suggested_addition: |
    Add to section 2.1:
    ```moo
    if (x = 5)        // Valid: assigns 5 to x, then checks if 5 is truthy
                      // x becomes 5, condition is true
    if (x = 0)        // Valid: assigns 0 to x, condition is false
    ```

- id: GAP-OPS-024
  feature: "in operator with maps and non-string keys"
  spec_file: "spec/operators.md"
  spec_section: "9.3 Membership"
  gap_type: assume
  question: |
    For `1 in [1 -> "one"]`, does this return `1` (key exists)?
    Spec says "1 if key exists" but doesn't clarify if INT keys are supported.
    (spec/types.md says maps allow INT or STR keys, so this should work)
  impact: |
    Implementor might only check string keys, breaking INT key maps.
  suggested_addition: |
    Add to section 9.3:
    ```moo
    "key" in ["key" -> 1]         => 1      // String key exists
    1 in [1 -> "one"]             => 1      // Integer key exists
    "key" in [1 -> "one"]         => 0      // Key type must match
    ```

- id: GAP-OPS-025
  feature: "property access on invalid objects"
  spec_file: "spec/operators.md"
  spec_section: "14.1 Property Access"
  gap_type: assume
  question: |
    For `#-1.name`, does this raise E_INVIND immediately, or attempt to
    access the property and then raise E_PROPNF?
    Spec lists E_INVIND as an error but doesn't specify order of checks.
  impact: |
    Error precedence matters for catch expressions:
    \`#-1.name ! E_PROPNF\` should NOT catch if E_INVIND is raised instead.
  suggested_addition: |
    Add to section 14.1:
    ```moo
    #-1.name          => E_INVIND  // Invalid object checked FIRST
    #0.missing        => E_PROPNF  // Valid object, missing property
    ```

---

### 8. Type System Interactions

- id: GAP-OPS-026
  feature: "truthiness of error codes"
  spec_file: "spec/types.md"
  spec_section: "2.8 BOOL"
  gap_type: test
  question: |
    For `if (E_TYPE)`, is E_TYPE truthy or falsy?
    Spec lists falsy values but doesn't mention error codes.
    E_TYPE has numeric value 1, which should be truthy.
  impact: |
    Code like `if (result = \`operation ! E_TYPE\`)` depends on whether
    error codes are truthy.
  suggested_addition: |
    Add to spec/types.md section 2.8:
    ```moo
    if (E_NONE)       // False: E_NONE has value 0
    if (E_TYPE)       // True: E_TYPE has value 1 (non-zero)
    if (E_DIV)        // True: E_DIV has value 2 (non-zero)
    ```

- id: GAP-OPS-027
  feature: "object equality for invalid objects"
  spec_file: "spec/operators.md"
  spec_section: "9.1 Equality"
  gap_type: test
  question: |
    For `#-1 == #-1`, does this return `1` (equal) or raise E_INVIND?
    Spec says equality requires "same type" but doesn't specify if invalid
    objects can be compared.
  impact: |
    Common idiom: `if (result == #-1)` to check for "nothing" sentinel.
    If this raises E_INVIND, breaks a lot of code.
  suggested_addition: |
    Add to section 9.1:
    ```moo
    #-1 == #-1        => 1      // Invalid objects can be compared
    #0 == #-1         => 0      // Different object IDs
    #99999 == #99999  => 1      // Even non-existent objects compare by ID
    ```

---

## Summary Statistics

**Total Gaps:** 27
**Gap Types:**
- Guess: 9 (33%)
- Assume: 8 (30%)
- Test: 10 (37%)
- Ask: 0 (0%)

**Critical Gaps:** 8 (would cause runtime incompatibility)
- GAP-OPS-007 (division truncation)
- GAP-OPS-008 (modulo sign)
- GAP-OPS-010 (power with negative exponent)
- GAP-OPS-012 (float equality precision)
- GAP-OPS-016 (shift overflow)
- GAP-OPS-022 (scatter overflow)
- GAP-OPS-025 (error precedence)
- GAP-OPS-027 (invalid object equality)

**High Priority Gaps:** 11 (would cause confusion or bugs)
- All short-circuit gaps (GAP-OPS-004, 005, 006)
- All precedence/associativity gaps (GAP-OPS-001, 002, 003)
- Comparison chaining (GAP-OPS-013)
- Bitwise edge cases (GAP-OPS-017, 018)
- Special operator edge cases (GAP-OPS-020, 021)

**Medium Priority Gaps:** 8 (edge cases, less common)

---

## Recommendations

### Immediate Actions (Critical)

1. **Add example sections** to every operator with:
   - Edge cases (zero, negative, overflow)
   - Type mixing behavior
   - Error conditions with examples

2. **Specify error precedence** explicitly:
   - When multiple errors could occur, which is raised?
   - Example: `invalid_obj.missing_prop` → E_INVIND (not E_PROPNF)

3. **Add precedence examples** showing:
   - Operator chaining (`a && b || c`)
   - Mixed precedence (`2 + 3 * 4 ^ 5`)
   - Right-associative chains (`a = b = c`, `2 ^ 3 ^ 2`)

### Short-Term Improvements

4. **Document short-circuit semantics** with side-effect examples
5. **Clarify truncation/rounding** for division and modulo
6. **Specify bitwise operator behavior** for edge cases (overflow, negatives)

### Long-Term Enhancements

7. **Create conformance test suite** covering all gaps identified
8. **Add "common mistakes" section** for each operator category
9. **Include performance notes** (e.g., copy-on-write for assignments)

---

## Implementor Experience Prediction

An implementor working ONLY from this spec would:

1. **Successfully implement** (70%):
   - Basic arithmetic (without edge cases)
   - Simple comparisons
   - Logical operators (basic behavior)
   - Bitwise operators (positive integers)

2. **Need to guess** (25%):
   - Division/modulo truncation direction
   - Power operator edge cases
   - Shift overflow behavior
   - Error precedence
   - Scatter assignment overflow

3. **Get subtly wrong** (5%):
   - Float equality (bitwise vs epsilon)
   - Short-circuit side effects
   - Comparison chaining
   - Invalid object operations

**Conclusion:** The spec is structurally sound but needs 20-30 concrete examples to close gaps and prevent subtle incompatibilities.
