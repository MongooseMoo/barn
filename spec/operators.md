# MOO Operators Specification

## Overview

MOO has 18 operator precedence levels. This document specifies the exact semantics of each operator.

---

## 1. Precedence Table (Low to High)

| Level | Operator | Associativity | Name |
|-------|----------|---------------|------|
| 1 | `=` | Right | Assignment |
| 2 | `? \|` | Right | Ternary |
| 3 | `` ` ! => ' `` | N/A | Catch |
| 4 | `@` | Right | Splice |
| 5 | `{ } =` | Right | Scatter |
| 6 | `\|\|` | Left | Logical OR |
| 7 | `&&` | Left | Logical AND |
| 8 | `\|.` | Left | Bitwise OR |
| 9 | `^.` | Left | Bitwise XOR |
| 10 | `&.` | Left | Bitwise AND |
| 11 | `== != < <= > >= in` | Left | Comparison |
| 12 | `<< >>` | Left | Shift |
| 13 | `+ -` | Left | Additive |
| 14 | `* / %` | Left | Multiplicative |
| 15 | `^` | Right | Power |
| 16 | `! ~ -` | Right | Unary |
| 17 | `. : [ ]` | Left | Postfix |

### Precedence Examples

```moo
// Boolean operator chains
a && b || c           // Equivalent to: (a && b) || c
a || b && c           // Equivalent to: a || (b && c)

// Mixed arithmetic/power
2 + 3 * 4 ^ 5         // Equivalent to: 2 + (3 * (4 ^ 5))
                      // Power (15) > Multiply (14) > Add (13)

// Right-associative chains
2 ^ 3 ^ 2             // Equivalent to: 2 ^ (3 ^ 2) = 2 ^ 9 = 512
a = b = c             // Equivalent to: a = (b = c) - both get c's value

// Comparison chains (C-style, not Python)
1 < 2 < 3             // Equivalent to: (1 < 2) < 3 = 1 < 3 = 1
                      // NOT: 1 < 2 AND 2 < 3 (Python-style)
```

---

## 2. Assignment Operators

### 2.1 Simple Assignment (`=`)

```moo
variable = expression
```

**Semantics:**
- Evaluates `expression`
- Binds result to `variable`
- Returns the assigned value

**Valid targets (lvalues):**
- Local variables: `x = 5`
- Properties: `obj.prop = 5`
- Indexed elements: `list[i] = 5`
- Range assignment: `list[2..4] = {10, 20, 30}`

**Examples:**
```moo
// Multi-assignment (right-associative)
a = b = 5;            // Both a and b become 5
x = y = z = 0;        // All three set to 0

// Assignment in boolean context
if (x = get_value())  // Assigns result to x, then tests truthiness
    // x is now set AND condition tested

if (x = 0)            // x becomes 0, condition is false
```

**Errors:**
- `E_TYPE`: Invalid lvalue type
- `E_PERM`: Property not writable

### 2.2 Indexed Assignment

```moo
list[index] = value
map[key] = value
```

**Semantics:**
- For lists: replaces element at 1-based index
- For maps: sets or adds key-value pair
- For strings: replaces character (returns new string)

**Errors:**
- `E_RANGE`: Index out of bounds (lists), or key not found (maps)

### 2.3 Range Assignment

```moo
list[start..end] = replacement_list
```

**Semantics:**
- Replaces elements from `start` to `end` (inclusive)
- Replacement can be different length (list grows/shrinks)
- **Replacement must be a LIST** - if replacement is not a list, raises E_TYPE
- Assigning empty list deletes the range

**Examples:**
```moo
list = {1, 2, 3, 4, 5};
list[2..4] = {10, 20, 30};  // list = {1, 10, 20, 30, 5}
list[2..4] = {99};          // list = {1, 99, 5} (shrinks)
list[2..4] = {};            // list = {1, 5} (deletes elements 2-4)
list[2..2] = 5;             // E_TYPE (not a list)
```

**Errors:**
- `E_TYPE`: Replacement is not a LIST
- `E_RANGE`: Invalid range bounds (see Range Indexing in types.md)

---

## 3. Ternary Operator (`? |`)

```moo
condition ? true_expr | false_expr
```

**Semantics:**
1. Evaluate `condition`
2. If truthy, evaluate and return `true_expr`
3. If falsy, evaluate and return `false_expr`

**Short-circuit:** Only one branch is evaluated.

**Examples:**
```moo
// Basic ternary
x > 0 ? "positive" | "non-positive"
valid(obj) ? obj.name | "invalid"

// Ternary with assignment (ternary binds tighter than =)
result = flag ? "yes" | "no"      // Assigns "yes" or "no" to result
flag ? x = 1 | y = 2              // Assigns to x OR y based on flag
```

---

## 4. Catch Expression

```moo
`expression ! error_codes`
`expression ! error_codes => default`
```

**Semantics:**
1. Evaluate `expression`
2. If no error, return result
3. If error matches `error_codes`, return error (or `default`)
4. If error doesn't match, propagate error

**Error code forms:**
- `ANY` - Catch all errors
- `E_TYPE, E_RANGE` - Specific errors (comma = OR, not AND)
- `@list_expr` - Dynamic error list

**Examples:**
```moo
// Single error with default
`x / y ! E_DIV => 0`              // Returns 0 if division by zero

// Multiple errors (OR semantics)
`x / y ! E_DIV, E_TYPE => 0`      // Catches EITHER error, returns 0
`x / y ! E_DIV`                   // Catches only E_DIV, E_TYPE propagates

// Catch all
`risky() ! ANY => "failed"`       // Catches all errors
`obj.prop ! E_PROPNF`             // Catch missing property, return error code
```

---

## 5. Splice Operator (`@`)

```moo
@list_expression
```

**Contexts:**
1. **In list literals:** Flattens list into containing list
2. **In function arguments:** Spreads list as arguments
3. **In scatter assignment:** Collects remaining elements

**Type requirement:** The `@` operator requires a LIST operand. Splicing a non-list raises E_TYPE.

**Examples:**
```moo
// In list literals
{1, @{2, 3}, 4}           => {1, 2, 3, 4}
{@{1}, @{2, 3}}           => {1, 2, 3}
{1, @"string", 3}         => E_TYPE (cannot splice non-list)

// In function calls (mixed with regular args)
func(@{1, 2, 3})          // Calls func(1, 2, 3)
func(1, @{2, 3}, 4)       // Calls func(1, 2, 3, 4)
func(@list1, @list2)      // Concatenates both lists as args

// In scatter (rest target)
{a, @rest} = {1, 2, 3}    // a=1, rest={2, 3}
```

**Errors:**
- `E_TYPE`: Expression is not a LIST

---

## 6. Scatter Assignment

```moo
{targets} = list_expression
```

**Target types:**
- Required: `var` - Must have value
- Optional: `?var` or `?var = default` - Uses default if missing
- Rest: `@var` - Collects remaining elements

**Semantics:**
1. Evaluate right side (must be list)
2. Bind elements left-to-right
3. Required targets must have values
4. Rest target collects remaining

**Examples:**
```moo
// Exact match
{a, b, c} = {1, 2, 3}              // a=1, b=2, c=3

// Optional targets
{a, ?b = 0} = {1}                  // a=1, b=0 (default)
{a, ?b = 0} = {1, 2}               // a=1, b=2 (value provided)

// Rest target
{a, @rest} = {1, 2, 3, 4}          // a=1, rest={2,3,4}
{a, @rest} = {1}                   // a=1, rest={} (empty)
{a, ?b, @rest} = {1}               // a=1, b=0, rest={}

// Errors
{a, b} = {1, 2, 3}                 => E_ARGS  // Too many values, no rest target
{a, b} = {1}                       => E_ARGS  // Not enough values for required b
```

**Errors:**
- `E_ARGS`: Not enough values for required targets
- `E_ARGS`: Too many values without rest target

---

## 7. Logical Operators

### 7.1 Logical OR (`||`)

```moo
left || right
```

**Semantics (short-circuit):**
1. Evaluate `left`
2. If truthy, return `left` value (don't evaluate right)
3. If falsy, evaluate and return `right`

**Short-circuit guarantee:** Right operand is NEVER evaluated if left is truthy.

**Examples:**
```moo
1 || 2              => 1
0 || 2              => 2
"" || "default"     => "default"
0 || ""             => ""      // Both falsy, returns right
"" || 0             => 0       // Both falsy, returns right

// Side effects - RIGHT NEVER CALLED
valid(obj) || crash()          // crash() not called if valid(obj) is truthy
1 || (x = 5)                   // x not assigned, returns 1
0 || (x = 5)                   // x assigned 5, returns 5
```

### 7.2 Logical AND (`&&`)

```moo
left && right
```

**Semantics (short-circuit):**
1. Evaluate `left`
2. If falsy, return `left` value (don't evaluate right)
3. If truthy, evaluate and return `right`

**Short-circuit guarantee:** Right operand is NEVER evaluated if left is falsy.

**Examples:**
```moo
1 && 2          => 2
0 && 2          => 0
"hi" && "bye"   => "bye"

// Chained short-circuit
a && b && c     // If a is falsy, neither b nor c evaluated
                // If a truthy but b falsy, c not evaluated

// Side effects - safe property/verb access
valid(obj) && obj.dangerous_property     // Property only accessed if valid
valid(obj) && obj:risky_verb()           // Verb only called if valid
```

### 7.3 Logical NOT (`!`)

```moo
!expression
```

**Semantics:**
- Returns `1` if expression is falsy
- Returns `0` if expression is truthy

---

## 8. Bitwise Operators

All bitwise operators require INT operands.

### 8.1 Bitwise OR (`|.`)

```moo
left |. right
bitor(left, right)  // Function form
```

**Semantics:** Binary OR of integer bits

**Examples:**
```moo
12 |. 10        => 14      // 1100 | 1010 = 1110
bitor(12, 10)   => 14
```

### 8.2 Bitwise XOR (`^.`)

```moo
left ^. right
bitxor(left, right)  // Function form
```

**Semantics:** Binary XOR of integer bits

**Examples:**
```moo
12 ^. 10        => 6       // 1100 ^ 1010 = 0110
bitxor(12, 10)  => 6
```

### 8.3 Bitwise AND (`&.`)

```moo
left &. right
bitand(left, right)  // Function form
```

**Semantics:** Binary AND of integer bits

**Examples:**
```moo
12 &. 10        => 8       // 1100 & 1010 = 1000
bitand(12, 10)  => 8
```

### 8.4 Bitwise NOT (`~`)

```moo
~expression
bitnot(expression)  // Function form
```

**Semantics:** Binary NOT (ones complement) of integer

**Examples:**
```moo
~0              => -1      // All zeros → all ones
~(-1)           => 0       // All ones → all zeros
~5              => -6      // 00000101 → 11111010 (two's complement)
bitnot(0)       => -1
```

### 8.5 Left Shift (`<<`)

```moo
value << count
bitshl(value, count)  // Function form (ToastStunt)
```

**Semantics:** Shift bits left, fill with zeros

**Examples:**
```moo
1 << 4          => 16
bitshl(1, 4)    => 16
```

**Errors:**
- `E_INVARG`: Negative shift count

### 8.6 Right Shift (`>>`)

```moo
value >> count
bitshr(value, count)  // Function form (ToastStunt)
```

**Semantics:** Arithmetic right shift (preserves sign)

**Examples:**
```moo
8 >> 1          => 4       // Positive: fills with 0
-8 >> 1         => -4      // Negative: sign-extended (fills with 1)
-1 >> 10        => -1      // All-ones pattern remains all-ones
bitshr(16, 4)   => 1
```

**Errors:**
- `E_TYPE`: Non-integer operand
- `E_INVARG`: Negative shift count

---

## 9. Comparison Operators

All comparison operators return `1` (true) or `0` (false).

### 9.1 Equality (`==`, `!=`)

```moo
left == right
left != right
```

**Semantics:**
- Same type required for equality
- Different types are never equal
- Lists/maps compared by value (deep)

**Type strictness:** Equality requires **exact type match**. INT and FLOAT are never equal.

**List equality:** List equality is **recursive structural comparison**. Two lists are equal if they have the same length and all corresponding elements are equal (using `==` recursively). **Comparison is O(n)** in total element count including nested lists. This is not reference equality - two separately created lists with the same values are equal.

**Map equality:** Two maps are equal if they have the **same set of (key, value) pairs**:
- **Iteration order is IGNORED** (order is implementation-defined)
- Key and value comparison uses deep equality recursively
- **Comparison is O(n)** in total entry count including nested structures

```moo
["a" -> 1, "b" -> 2] == ["b" -> 2, "a" -> 1]   => 1 (true)
["a" -> 1] == ["a" -> 1.0]                     => 0 (value types differ)
```

**Examples:**
```moo
// Basic equality
1 == 1              => 1
1 == 1.0            => 0 (different types!)
"a" == "a"          => 1

// Deep equality for collections
{1,2} == {1,2}      => 1
{1, {2, 3}} == {1, {2, 3}}   => 1      // Recursive deep comparison
["a" -> 1] == ["a" -> 1]     => 1      // Maps compared by value

// Float equality (bitwise, no epsilon tolerance)
1.0 == 1.0          => 1      // Exact bitwise match
0.1 + 0.2 == 0.3    => 0      // Float precision issue
                              // Use abs(a - b) < epsilon for tolerance

// Invalid objects
#-1 == #-1          => 1      // Invalid objects can be compared by ID
#0 == #-1           => 0      // Different IDs
```

### 9.2 Ordering (`<`, `<=`, `>`, `>=`)

```moo
left < right
left <= right
left > right
left >= right
```

**Valid types:** INT, FLOAT, STR

**Type strictness:** Both operands must be the **same type**. No INT/FLOAT coercion.

**String comparison:** Lexicographic (case-sensitive, ASCII ordering)

**Examples:**
```moo
// Numeric comparison
1 < 2           => 1
1.0 < 2.0       => 1
1 < 1.0         => E_TYPE  // Type mismatch

// String comparison (ASCII: uppercase 65-90, lowercase 97-122)
"A" < "a"       => 1       // Uppercase sorts before lowercase
"Z" < "a"       => 1       // All uppercase before lowercase
"abc" < "abd"   => 1       // Lexicographic

// Comparison chaining (C-style, not Python)
1 < 2 < 3       // Equivalent to: (1 < 2) < 3
                // Evaluates: 1 < 3 => 1
                // NOT: 1 < 2 AND 2 < 3 (Python-style)
```

**Errors:**
- `E_TYPE`: Incompatible types (including INT vs FLOAT)

### 9.3 Membership (`in`)

```moo
element in collection
```

**Semantics by collection type:**

| Collection | Returns |
|------------|---------|
| LIST | 1-based index if found, 0 if not |
| MAP | 1 if key exists, 0 if not |
| STR | 1-based index of substring, 0 if not |

**Examples:**
```moo
// List membership (returns index)
2 in {1, 2, 3}        => 2 (index)
5 in {1, 2, 3}        => 0 (not found)

// Map membership (returns boolean)
"key" in ["key" -> 1] => 1 (key exists)
1 in [1 -> "one"]     => 1 (integer keys supported)
"x" in [1 -> "one"]   => 0 (key type must match)

// String membership (returns index)
"bc" in "abcd"        => 2 (index of substring)
"z" in "abcd"         => 0 (not found)
```

---

## 10. Shift Operators

```moo
value << count
value >> count
```

**Semantics:**
- `<<`: Left shift, fill with zeros
- `>>`: Arithmetic right shift (sign-extended)

**Operands:** Both must be INT

**Examples:**
```moo
1 << 4      => 16
16 >> 2     => 4
-8 >> 1     => -4 (sign preserved)
```

---

## 11. Arithmetic Operators

### 11.1 Addition (`+`)

```moo
left + right
```

**By types:**

| Left | Right | Result |
|------|-------|--------|
| INT | INT | INT (sum) |
| FLOAT | FLOAT | FLOAT (sum) |
| INT | FLOAT | `E_TYPE` (no coercion) |
| STR | STR | STR (concatenation) |
| LIST | LIST | LIST (concatenation) |

**Examples:**
```moo
1 + 2              => 3
1.5 + 2.5          => 4.0
"hello" + " world" => "hello world"
{1, 2} + {3, 4}    => {1, 2, 3, 4}
```

**Errors:**
- `E_TYPE`: Incompatible types

### 11.2 Subtraction (`-`)

```moo
left - right
```

**By types:**

| Left | Right | Result |
|------|-------|--------|
| INT | INT | INT |
| FLOAT | FLOAT | FLOAT |
| INT | FLOAT | `E_TYPE` (no coercion) |

### 11.3 Multiplication (`*`)

```moo
left * right
```

**By types:**

| Left | Right | Result |
|------|-------|--------|
| INT | INT | INT |
| FLOAT | FLOAT | FLOAT |
| INT | FLOAT | `E_TYPE` (no coercion) |
| STR | INT | STR (repeated) |

**Examples:**
```moo
3 * 4       => 12
"ab" * 3    => "ababab"
```

### 11.4 Division (`/`)

```moo
left / right
```

**By types:**

| Left | Right | Result |
|------|-------|--------|
| INT | INT | INT (truncated toward zero) |
| FLOAT | FLOAT | FLOAT |
| INT | FLOAT | `E_TYPE` (no coercion) |

**Integer division:** Truncates toward zero (C semantics), not floor division.

**Examples:**
```moo
7 / 2           => 3       // Positive: truncate toward zero
-7 / 2          => -3      // Negative: truncate toward zero (NOT -4)
-7 / -2         => 3       // Both negative: result positive
7 / -2          => -3      // Mixed signs: truncate toward zero
```

**Errors:**
- `E_DIV`: Division by zero (int or float)

### 11.5 Modulo (`%`)

```moo
left % right
```

**Semantics:** Remainder after integer division

**Sign:** Result has same sign as dividend (left) - C modulo semantics

**Examples:**
```moo
7 % 3           => 1       // Positive dividend: positive result
-7 % 3          => -1      // Negative dividend: negative result
7 % -3          => 1       // Positive dividend: positive result
-7 % -3         => -1      // Negative dividend: negative result

// Invariant: (a / b) * b + (a % b) == a
```

**Errors:**
- `E_DIV`: Modulo by zero
- `E_TYPE`: Non-integer operand

---

## 12. Power Operator (`^`)

```moo
base ^ exponent
```

**Semantics:**
- INT ^ INT → INT (integer exponentiation)
- FLOAT ^ INT → FLOAT (float exponentiation)
- FLOAT ^ FLOAT → FLOAT (float exponentiation)
- INT ^ FLOAT → `E_TYPE` (no mixed type exponentiation)
- Right-associative: `2^3^2` = `2^(3^2)` = `2^9` = `512`

**Examples:**
```moo
// Basic exponentiation
2 ^ 10          => 1024 (INT)
2.0 ^ 10        => 1024.0 (FLOAT)
4.0 ^ 0.5       => 2.0

// Negative base
(-2) ^ 3        => -8      // Odd exponent: negative result
(-2) ^ 4        => 16      // Even exponent: positive result

// Zero exponent
2 ^ 0           => 1       // Any base to power 0 is 1
0 ^ 0           => 1       // By convention

// Negative exponent (INT base)
2 ^ (-1)        => E_TYPE  // Would require FLOAT result
2.0 ^ (-1)      => 0.5     // FLOAT base supports negative exponents

// Type mixing
2 ^ 3.5         => E_TYPE  // INT base with FLOAT exponent not allowed
                           // Use 2.0 ^ 3.5 instead

// Domain errors
(-1.0) ^ 0.5    => E_FLOAT // Imaginary result
```

---

## 13. Unary Operators

### 13.1 Logical NOT (`!`)

```moo
!expression
```

Returns `1` if falsy, `0` if truthy.

### 13.2 Bitwise NOT (`~`)

```moo
~expression
```

Returns ones complement. Requires INT.

### 13.3 Negation (`-`)

```moo
-expression
```

Returns numeric negation. Requires INT or FLOAT.

---

## 14. Postfix Operators

### 14.1 Property Access (`.`)

```moo
object.property
object.(expression)
```

**Semantics:**
- Access property on object
- Dynamic form evaluates expression to property name

**Error precedence:** Object validity checked BEFORE property existence.

**Examples:**
```moo
#0.name         => "System Object" // Valid object
#0.missing      => E_PROPNF        // Property not found
#-1.name        => E_INVIND        // Invalid object checked FIRST
#-1.anything    => E_INVIND        // Not E_PROPNF
```

**Errors:**
- `E_INVIND`: Invalid object (checked first)
- `E_PROPNF`: Property not found
- `E_PERM`: Property not readable

### 14.2 Waif Property Access (`.:`)

```moo
waif.:property
```

**Semantics:** Access property on waif object

### 14.3 Indexing (`[ ]`)

```moo
collection[index]
collection[start..end]
```

**Semantics:**
- Single index: get element (1-based)
- Range: get sublist/substring (inclusive)

**Special indices:**
- `^` = 1 (first)
- `$` = length (last)

**Errors:**
- `E_RANGE`: Index out of bounds
- `E_TYPE`: Invalid index type

### 14.4 Verb Call (`:`)

```moo
object:verb(args)
object:(expression)(args)
```

**Semantics:**
- Call verb on object
- Dynamic form evaluates expression to verb name

**Errors:**
- `E_VERBNF`: Verb not found
- `E_PERM`: Verb not executable
- `E_INVIND`: Invalid object

---

## 15. Special Expressions

### 15.1 Dollar Property (`$`)

```moo
$property
$verb(args)
```

**Semantics:**
- `$property` = `#0.property`
- `$verb(args)` = `#0:verb(args)`

### 15.2 Index Markers

```moo
list[^]     // First element (same as list[1])
list[$]     // Last element (same as list[length(list)])
list[^..$]  // Entire list
```

---

## 16. Operator Summary by Type

### 16.1 INT Operations

| Operator | Result |
|----------|--------|
| `+ - * / %` | INT (div truncates) |
| `^` | FLOAT |
| `< <= > >= == !=` | BOOL |
| `\|. &. ^. ~ << >>` | INT |

### 16.2 FLOAT Operations

| Operator | Result |
|----------|--------|
| `+ - * / ^` | FLOAT |
| `< <= > >= == !=` | BOOL |

### 16.3 STR Operations

| Operator | Result |
|----------|--------|
| `+` | STR (concat) |
| `*` (with INT) | STR (repeat) |
| `< <= > >= == !=` | BOOL |
| `in` | INT (index) |
| `[ ]` | STR (substring) |

### 16.4 LIST Operations

| Operator | Result |
|----------|--------|
| `+` | LIST (concat) |
| `== !=` | BOOL |
| `in` | INT (index) |
| `[ ]` | Value or LIST |

### 16.5 MAP Operations

| Operator | Result |
|----------|--------|
| `== !=` | BOOL |
| `in` | BOOL |
| `[ ]` | Value |

---

## 17. Truthiness Summary

| Value | Truthy? |
|-------|---------|
| `0` | No |
| `0.0` | No |
| `""` | No |
| `{}` | No |
| `[]` | No |
| `false` | No |
| Everything else | Yes |
