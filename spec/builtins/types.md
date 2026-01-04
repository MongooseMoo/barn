# MOO Type Conversion Built-ins

## Overview

Type conversion functions transform values between MOO types.

---

## 1. typeof

**Signature:** `typeof(value) → INT`

**Description:** Returns the type code of a value.

**Type codes:**

| Type | Code | Constant |
|------|------|----------|
| INT | 0 | TYPE_INT |
| OBJ | 1 | TYPE_OBJ |
| STR | 2 | TYPE_STR |
| ERR | 3 | TYPE_ERR |
| LIST | 4 | TYPE_LIST |
| FLOAT | 9 | TYPE_FLOAT |
| MAP | 10 | TYPE_MAP |
| WAIF | 12 | TYPE_WAIF |
| BOOL | 14 | TYPE_BOOL |

**Examples:**
```moo
typeof(42)        => 0  (TYPE_INT)
typeof(#0)        => 1  (TYPE_OBJ)
typeof("hello")   => 2  (TYPE_STR)
typeof(E_TYPE)    => 3  (TYPE_ERR)
typeof({1,2,3})   => 4  (TYPE_LIST)
typeof(3.14)      => 9  (TYPE_FLOAT)
typeof(["a"->1])  => 10 (TYPE_MAP)
typeof(true)      => 14 (TYPE_BOOL)
```

**Errors:** None

---

## 2. tostr

**Signature:** `tostr(value, ...) → STR`

**Description:** Converts values to string representation and concatenates.

**Conversion rules:**

| Type | Conversion |
|------|------------|
| INT | Decimal representation |
| FLOAT | Decimal with precision |
| STR | Identity |
| OBJ | "#N" format |
| ERR | Error name (E_TYPE, etc.) |
| LIST | "{...}" format |
| MAP | "[...]" format |
| BOOL | "true" or "false" |

**Examples:**
```moo
tostr()                => ""  (empty string, not E_ARGS)
tostr(42)              => "42"
tostr(3.14159)         => "3.14159"
tostr(#0)              => "#0"
tostr(E_TYPE)          => "Type mismatch"
tostr({1, 2})          => "{list}"  (not expanded)
tostr(["a" -> 1])      => "[map]"   (not expanded)
tostr(true)            => "true"
tostr("a", 1, "b")     => "a1b"
```

**Note:** Collections (lists, maps) are formatted as `"{list}"` and `"[map]"` rather than being fully expanded. Use `toliteral()` for expanded representation.

**Errors:** None

---

## 3. toint

**Signature:** `toint(value) → INT`

**Description:** Converts value to integer.

**Conversion rules:**

| Type | Conversion |
|------|------------|
| INT | Identity |
| FLOAT | Truncate toward zero |
| STR | Parse as integer |
| OBJ | Object ID number |
| ERR | Error code number |
| BOOL | 1 or 0 |

**Examples:**
```moo
toint(42)        => 42
toint(3.7)       => 3
toint(-3.7)      => -3
toint("123")     => 123
toint("-45")     => -45
toint("3.14")    => 3
toint(#5)        => 5
toint(E_TYPE)    => 1
toint(true)      => 1
toint(false)     => 0
toint("abc")     => 0  (non-numeric string)
```

**Errors:**
- E_TYPE: LIST, MAP, WAIF

---

## 4. tofloat

**Signature:** `tofloat(value) → FLOAT`

**Description:** Converts value to floating point.

**Conversion rules:**

| Type | Conversion |
|------|------------|
| INT | Exact conversion |
| FLOAT | Identity |
| STR | Parse as float |

**Examples:**
```moo
tofloat(42)       => 42.0
tofloat(3.14)     => 3.14
tofloat("3.14")   => 3.14
tofloat("-1e10")  => -10000000000.0
tofloat("abc")    => 0.0  (non-numeric string)
```

**Errors:**
- E_TYPE: OBJ, ERR, LIST, MAP, WAIF, BOOL

---

## 5. toobj

**Signature:** `toobj(value) → OBJ`

**Description:** Converts value to object reference.

**Conversion rules:**

| Type | Conversion |
|------|------------|
| INT | Object with that ID |
| OBJ | Identity |
| STR | Parse "#N" format |

**Examples:**
```moo
toobj(5)      => #5
toobj("#5")   => #5
toobj(#5)     => #5
toobj(-1)     => #-1
```

**Errors:**
- E_TYPE: FLOAT, ERR, LIST, MAP, BOOL, WAIF

**Note:** Spec says E_INVARG for invalid string format, but actual behavior returns `#0` for unparseable strings (e.g., `toobj("abc")` → `#0`, `toobj("")` → `#0`).

---

## 6. toerr [Not Implemented]

**Status:** This function is documented but **NOT IMPLEMENTED** in ToastStunt.

**Original Signature:** `toerr(value) → ERR`

**Note:** Testing confirms Toast returns "Unknown built-in function: toerr". This function should not be used.

---

## 7. tonum [Not Implemented]

**Status:** This function is documented as an alias for `toint()`, but **NOT IMPLEMENTED** in ToastStunt.

**Note:** Testing confirms Toast returns "Unknown built-in function: tonum". Use `toint()` instead.

---

## 8. value_hash

**Signature:** `value_hash(value) → STR`

**Description:** Returns a hash code for any value. Equal values produce equal hashes.

**Implementation:**
- Returns 64-character hexadecimal SHA-256 hash string

**Properties:**
- Consistent within session
- May vary between server restarts
- Suitable for hash table keys

**Examples:**
```moo
value_hash("hello")      => "5AA762AE383FBB727AF3C7A36D4940A5B8C40A989452D2304FC958FF3F354E7A"
value_hash(#0) == value_hash(#0)   => 1
```

**Errors:** None

---

## 9. value_bytes

**Signature:** `value_bytes(value) → INT`

**Description:** Returns approximate memory size of a value in bytes.

**Behavior:**
- Includes structural overhead
- Recursive for lists/maps
- Useful for quota management

**Examples:**
```moo
value_bytes(0)           => 8    (approximate)
value_bytes("hello")     => 13   (approximate)
value_bytes({1, 2, 3})   => 40   (approximate)
```

**Errors:** None

---

## 10. typename [Not Implemented]

**Status:** This function is documented but **NOT IMPLEMENTED** in ToastStunt.

**Original Signature:** `typename(value) → STR`

**Note:** Testing confirms Toast returns "Unknown built-in function: typename". This function should not be used.

---

## 11. is_type [Not Implemented]

**Status:** This function is documented as a ToastStunt extension but **NOT IMPLEMENTED** in ToastStunt.

**Original Signature:** `is_type(value, type_code) → BOOL`

**Note:** Despite being labeled "ToastStunt extension", this function does not exist in the reference implementation. Use `typeof(value) == TYPE_CODE` instead.

---

## Go Implementation Notes

```go
type TypeCode int

const (
    TYPE_INT   TypeCode = 0
    TYPE_OBJ   TypeCode = 1
    TYPE_STR   TypeCode = 2
    TYPE_ERR   TypeCode = 3
    TYPE_LIST  TypeCode = 4
    TYPE_FLOAT TypeCode = 9
    TYPE_MAP   TypeCode = 10
    TYPE_WAIF  TypeCode = 12
    TYPE_BOOL  TypeCode = 14
)

func Typeof(v Value) int64 {
    return int64(v.Type())
}

func Tostr(args []Value) (string, error) {
    var sb strings.Builder
    for _, v := range args {
        sb.WriteString(v.String())
    }
    return sb.String(), nil
}

func Toint(v Value) (int64, error) {
    switch val := v.(type) {
    case IntValue:
        return int64(val), nil
    case FloatValue:
        return int64(val), nil  // Truncate
    case StringValue:
        n, _ := strconv.ParseInt(string(val), 10, 64)
        return n, nil
    case ObjValue:
        return int64(val), nil
    case ErrValue:
        return int64(val), nil
    case BoolValue:
        if val { return 1, nil }
        return 0, nil
    default:
        return 0, E_TYPE
    }
}
```
