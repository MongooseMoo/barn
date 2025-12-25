# MOO Type System Specification

## Overview

MOO is a dynamically-typed language with 9 distinct value types. All collections use **1-based indexing**.

---

## 1. Type Enumeration

| Type Code | Name | Description |
|-----------|------|-------------|
| 0 | INT | 64-bit signed integer |
| 1 | OBJ | Object reference (object ID) |
| 2 | STR | Immutable string |
| 3 | ERR | Error code |
| 4 | LIST | Heterogeneous ordered collection |
| 9 | FLOAT | 64-bit IEEE 754 floating point |
| 10 | MAP | Key-value dictionary |
| 13 | WAIF | Lightweight prototype object |
| 14 | BOOL | Boolean (true/false) |

**Note:** Type codes 5-8, 11-12 are reserved for internal use.

---

## 2. Type Details

### 2.1 INT (Integer)

**Range:** -9,223,372,036,854,775,808 to 9,223,372,036,854,775,807 (64-bit signed)

**Overflow behavior:** Undefined. Integer overflow in arithmetic operations (`+`, `-`, `*`) is not checked and may wrap, saturate, or produce unpredictable results. Portable code must avoid overflow.

**Literals:**
```moo
42
-17
0
9223372036854775807
```

**Go mapping:** `int64`

### 2.2 FLOAT (Floating Point)

**Precision:** IEEE 754 double (64-bit)

**Literals:**
```moo
3.14
-2.5
1.0e10
6.022e23
1e-9
```

**Special values:** NaN and Infinity **cannot exist** in MOO.
- Division by zero (int or float): raises `E_DIV` before computation
- Operations that would produce NaN/Infinity: raise `E_FLOAT`
  - Example: `sqrt(-1.0)` → `E_FLOAT`
  - Example: `1.0e308 * 1.0e308` → `E_FLOAT` (overflow)
- Any result outside `[-DBL_MAX, DBL_MAX]` raises `E_FLOAT`

**Go mapping:** `float64`

### 2.3 STR (String)

**Encoding:** Binary (byte sequences). Strings are **not** character-aware.

**Indexing:** 1-based **byte indexing**. Each index accesses one byte, not one character.
- UTF-8 text: `"日"` is 3 bytes, `"日"[1]` returns the first byte (0xE6)
- `length("日")` returns `3` (byte count, not character count)

**Literals:**
```moo
"hello"
"line1\nline2"
"quote: \"nested\""
"tab:\there"
""
```

**Escape sequences:**
| Sequence | Meaning |
|----------|---------|
| `\\` | Backslash |
| `\"` | Double quote |
| `\n` | Newline |
| `\t` | Tab |
| `\r` | Carriage return |
| `\xHH` | Hex byte |

**Go mapping:** Custom `MOOString` with 1-based indexing

### 2.4 OBJ (Object Reference)

**Format:** Object ID as signed integer

**Literals:**
```moo
#0        // System object
#1        // First user object
#-1       // Invalid object (common sentinel)
#12345    // Any valid object ID
```

**Special objects:**
- `#0` - System object (accessed via `$`)
- `#-1` - Common "nothing" sentinel
- `$nothing` - Usually `#-1`

**Go mapping:** `int64` (object ID)

### 2.5 ERR (Error Code)

**18 standard error codes:**

| Code | Name | Value | Description |
|------|------|-------|-------------|
| E_NONE | No error | 0 | Success |
| E_TYPE | Type mismatch | 1 | Wrong type for operation |
| E_DIV | Division by zero | 2 | Division or modulo by zero |
| E_PERM | Permission denied | 3 | Insufficient privileges |
| E_PROPNF | Property not found | 4 | Property doesn't exist |
| E_VERBNF | Verb not found | 5 | Verb doesn't exist |
| E_VARNF | Variable not found | 6 | Undefined variable |
| E_INVIND | Invalid index | 7 | Index out of bounds |
| E_RECMOVE | Recursive move | 8 | Object can't contain itself |
| E_MAXREC | Max recursion | 9 | Stack overflow |
| E_RANGE | Range error | 10 | Value out of range |
| E_ARGS | Wrong arg count | 11 | Function called with wrong arity |
| E_NACC | Not accessible | 12 | Value not readable |
| E_INVARG | Invalid argument | 13 | Argument value invalid |
| E_QUOTA | Quota exceeded | 14 | Resource limit hit |
| E_FLOAT | Float error | 15 | Invalid float operation |
| E_FILE | File error | 16 | File I/O failure |
| E_EXEC | Exec error | 17 | Process execution failure |

**Literals:**
```moo
E_TYPE
E_RANGE
E_PERM
```

**Go mapping:** Custom `ErrorCode` enum

### 2.6 LIST (List)

**Characteristics:**
- Heterogeneous (mixed types allowed)
- 1-based indexing
- Copy-on-write semantics
- Nested lists allowed to arbitrary depth

**Literals:**
```moo
{}                      // Empty list
{1, 2, 3}              // Integers
{"a", "b", "c"}        // Strings
{1, "two", #3}         // Mixed types
{{1, 2}, {3, 4}}       // Nested
{@other, 5}            // Splice
```

**Indexing:**
```moo
list[1]                // First element
list[$]                // Last element ($ = length)
list[^]                // First element (^ = 1)
list[2..4]             // Sublist (inclusive)
list[^..$]             // Entire list
```

**Limits:**
- **Nesting depth:** Lists can be nested to arbitrary depth, limited only by available memory. Operations on deeply nested lists may raise E_QUOTA if resource limits are exceeded.
- **Maximum length:** List length is limited only by available memory and quota settings. Implementations should raise E_QUOTA when list operations would exceed configured resource limits. **Minimum supported length is 2^31-1 elements** on 64-bit systems.

**Go mapping:** Custom `MOOList` with 1-based indexing and COW

### 2.7 MAP (Dictionary)

**Characteristics:**
- Keys: Any hashable type (INT, FLOAT, STR, OBJ, ERR, BOOL, LIST, MAP, ANON, WAIF)
- Values: Any type
- Unordered (implementation-defined iteration order)
- Copy-on-write semantics

**Literals:**
```moo
[]                          // Empty map
["name" -> "Alice"]         // String key
[1 -> "one", 2 -> "two"]    // Integer keys
["nested" -> ["a" -> 1]]    // Nested maps
```

**Access:**
```moo
map["key"]                  // Get value
map["key"] = value          // Set value
"key" in map                // Check existence
```

**Go mapping:** Custom `MOOMap` with COW

### 2.8 BOOL (Boolean)

**Values:** `true` or `false`

**Literals:**
```moo
true
false
```

**Truthiness:**
- False: `0`, `0.0`, `""`, `{}`, `[]`, `false`
- True: Everything else

**Go mapping:** `bool`

### 2.9 WAIF (Lightweight Object)

**Characteristics:**
- Prototype-based (no full object overhead)
- Properties accessed via `.:` syntax
- Garbage collected
- Used for temporary/lightweight data

**Creation:**
```moo
w = new_waif();
w.:name = "example";
```

**Go mapping:** Custom `MOOWaif` struct

---

## 3. Type Coercion

### 3.1 Automatic Coercion

MOO performs minimal automatic coercion:

| Context | Rule |
|---------|------|
| Boolean | See truthiness above |
| String concat | Numbers NOT auto-converted |
| Arithmetic | INT + FLOAT → `E_TYPE` (no coercion) |
| Comparison | INT vs FLOAT → `0` for `==`, `E_TYPE` for `<`/`>` |

### 3.2 Explicit Conversion Functions

| Function | Input | Output |
|----------|-------|--------|
| `toint(x)` | STR, FLOAT, OBJ, ERR | INT |
| `tofloat(x)` | STR, INT | FLOAT |
| `tostr(x)` | Any | STR |
| `toobj(x)` | INT, STR | OBJ |
| `toliteral(x)` | Any | STR (MOO literal) |

**Conversion rules:**

```moo
toint("42")      => 42
toint(3.7)       => 3         // Truncates
toint(#5)        => 5
toint(E_TYPE)    => 1

tofloat("3.14")  => 3.14
tofloat(42)      => 42.0

tostr(42)        => "42"
tostr(3.14)      => "3.14"
tostr(#5)        => "#5"
tostr(E_TYPE)    => "E_TYPE"
tostr({1,2})     => "{1, 2}"

toobj(5)         => #5
toobj("#5")      => #5

toliteral(42)    => "42"
toliteral("hi")  => "\"hi\""
toliteral({1})   => "{1}"
```

---

## 4. Operator Type Rules

### 4.1 Arithmetic Operators

| Operator | Left | Right | Result |
|----------|------|-------|--------|
| `+` | INT | INT | INT |
| `+` | FLOAT | FLOAT | FLOAT |
| `+` | STR | STR | STR (concat) |
| `+` | LIST | LIST | LIST (concat) |
| `-` | INT | INT | INT |
| `-` | FLOAT | FLOAT | FLOAT |
| `*` | INT | INT | INT |
| `*` | FLOAT | FLOAT | FLOAT |
| `*` | STR | INT | STR (repeat) |
| `/` | INT | INT | INT (truncate) |
| `/` | FLOAT | FLOAT | FLOAT |
| `%` | INT | INT | INT |
| `^` | INT | INT | INT |
| `^` | FLOAT | INT | FLOAT |
| `^` | FLOAT | FLOAT | FLOAT |

**Note:** Power (`^`) allows FLOAT base with INT/FLOAT exponent. All other operators require same types.

**Errors:**
- `E_TYPE`: Incompatible operand types
- `E_DIV`: Division or modulo by zero

### 4.2 Comparison Operators

All comparison operators return BOOL (0 or 1 in classic MOO).

| Operator | Types | Behavior |
|----------|-------|----------|
| `==` | Any | Equal (type-sensitive) |
| `!=` | Any | Not equal |
| `<` | INT, FLOAT, STR | Less than |
| `<=` | INT, FLOAT, STR | Less or equal |
| `>` | INT, FLOAT, STR | Greater than |
| `>=` | INT, FLOAT, STR | Greater or equal |
| `in` | Any, LIST/MAP/STR | Membership |

**`in` operator:**
```moo
2 in {1, 2, 3}        => 1 (true, returns index)
"x" in "xyz"          => 1 (true, returns index)
"key" in ["key" -> 1] => 1 (true)
5 in {1, 2, 3}        => 0 (false)
```

### 4.3 Logical Operators

| Operator | Behavior |
|----------|----------|
| `\|\|` | Short-circuit OR, returns first truthy or last value |
| `&&` | Short-circuit AND, returns first falsy or last value |
| `!` | Logical NOT, returns 0 or 1 |

### 4.4 Bitwise Operators

Only valid on INT:

| Operator | Name |
|----------|------|
| `\|.` | Bitwise OR |
| `&.` | Bitwise AND |
| `^.` | Bitwise XOR |
| `~` | Bitwise NOT (unary) |
| `<<` | Left shift |
| `>>` | Right shift (arithmetic) |

---

## 5. Collection Semantics

### 5.1 1-Based Indexing

All collections use 1-based indexing:

```moo
list = {10, 20, 30};
list[1]   => 10      // First element
list[3]   => 30      // Third element
list[0]   => E_RANGE // Error!
list[4]   => E_RANGE // Error!

str = "hello";
str[1]    => "h"     // First character
str[5]    => "o"     // Fifth character
```

**Index validity:**
- **Index 0 is always invalid** and raises E_RANGE
- **Negative indices are not supported** and always raise E_RANGE
- Valid indices are 1 through length(list)
- Use `list[$]` to access the last element (where `$` equals `length(list)`)

### 5.2 Range Indexing

Ranges are inclusive on both ends:

```moo
list = {1, 2, 3, 4, 5};
list[2..4]    => {2, 3, 4}
list[1..1]    => {1}           // Single-element range
list[3..1]    => {}            // Reverse range returns empty list
list[^..$]    => {1, 2, 3, 4, 5}  // Entire list

str = "hello";
str[2..4]     => "ell"
```

**Range bounds checking:**

For range indexing `list[start..end]`:
- If `start < 1` or `start > length(list)`, raise E_RANGE
- If `end < 1` or `end > length(list)`, raise E_RANGE
- If `start > end`, return empty list `{}` (not an error)
- Both bounds are checked strictly; **no clamping occurs**
- Example: `list[2..100]` on a 5-element list raises E_RANGE (not clamped to `list[2..5]`)

### 5.3 Special Index Markers

| Marker | Meaning | Context |
|--------|---------|---------|
| `^` | 1 (first) | Index only |
| `$` | length (last) | Index and range end |

**Evaluation order:** Special markers `^` and `$` are substituted before range bounds checking. `$` always evaluates to `length(list)` at the time of range evaluation. If substitution results in `start > end`, an empty list is returned.

Example:
```moo
list = {1, 2, 3};
list[2..$]    => {2, 3}      // $ = 3
list[5..$]    => E_RANGE     // start > length, even though $ = 3
```

### 5.4 Copy-on-Write

Lists and maps use copy-on-write (COW) for efficiency:

```moo
a = {1, 2, 3};
b = a;           // b shares storage with a
b[1] = 99;       // b gets its own copy, a unchanged
// a = {1, 2, 3}, b = {99, 2, 3}
```

**When COW triggers:**

Copy-on-write triggers on **mutation operations**:
- Indexed assignment: `list[i] = x`
- Range assignment: `list[i..j] = x`
- List modification builtins: `listappend()`, `listset()`, `listinsert()`, `listdelete()`

Copy-on-write does **not** trigger for:
- List literals: always create new lists
- Splice operations: `{@list, x}` always creates a new list

**COW is shallow:**

When a list is copied due to mutation, only the top-level list structure is duplicated. Nested lists continue to share references until individually mutated.

Example:
```moo
a = {{1, 2}, {3, 4}};
b = a;              // b shares storage with a
b[1] = {5, 6};      // b gets new top-level list, a[1] unchanged
// a = {{1, 2}, {3, 4}}, b = {{5, 6}, {3, 4}}

c = a;
c[1][1] = 99;       // c[1] gets copied (nested COW), a[1] also modified initially
                    // until c[1] is mutated
```

**Go implementation note:** Track reference count; copy on mutation if refcount > 1.

---

## 6. Type Checking

### 6.1 typeof() Function

```moo
typeof(42)       => 0  (INT)
typeof(#5)       => 1  (OBJ)
typeof("hi")     => 2  (STR)
typeof(E_TYPE)   => 3  (ERR)
typeof({1,2})    => 4  (LIST)
typeof(3.14)     => 9  (FLOAT)
typeof([])       => 10 (MAP)
typeof(waif)     => 13 (WAIF)
typeof(true)     => 14 (BOOL)
```

### 6.2 Type Constants

For comparison with `typeof()`:

```moo
INT   = 0
OBJ   = 1
STR   = 2
ERR   = 3
LIST  = 4
FLOAT = 9
MAP   = 10
WAIF  = 13
BOOL  = 14
```

---

## 7. Go Implementation Notes

### 7.1 Value Interface

```go
type Value interface {
    Type() TypeCode
    String() string
    Equal(Value) bool
    Truthy() bool
}
```

### 7.2 Concrete Types

```go
type Int int64
type Float float64
type Str struct { data string }  // 1-based methods
type Obj int64
type Err ErrorCode
type List struct {
    elements []Value
    refcount *int32
}
type Map struct {
    data map[Value]Value
    refcount *int32
}
type Bool bool
```

### 7.3 1-Based Indexing

All collection accessor methods handle the 1-based conversion:

```go
func (l *List) Get(idx int) (Value, error) {
    if idx < 1 || idx > len(l.elements) {
        return nil, ErrRange
    }
    return l.elements[idx-1], nil
}
```
