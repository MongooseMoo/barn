# MOO Math Built-ins

## Overview

Mathematical functions for arithmetic, trigonometry, and numeric operations.

---

## 1. Basic Arithmetic

### 1.1 abs

**Signature:** `abs(number) → INT|FLOAT`

**Description:** Returns absolute value.

**Examples:**
```moo
abs(-5)     => 5
abs(5)      => 5
abs(-3.14)  => 3.14
```

**Errors:**
- E_TYPE: Non-numeric argument

---

### 1.2 min

**Signature:** `min(num1, num2, ...) → INT|FLOAT`

**Description:** Returns smallest value.

**Examples:**
```moo
min(3, 1, 4, 1, 5)  => 1
min(-5, 0, 5)       => -5
min(3.14, 2.71)     => 2.71
```

**Errors:**
- E_ARGS: No arguments
- E_TYPE: Non-numeric argument

---

### 1.3 max

**Signature:** `max(num1, num2, ...) → INT|FLOAT`

**Description:** Returns largest value.

**Examples:**
```moo
max(3, 1, 4, 1, 5)  => 5
max(-5, 0, 5)       => 5
max(3.14, 2.71)     => 3.14
```

**Errors:**
- E_ARGS: No arguments
- E_TYPE: Non-numeric argument

---

### 1.4 random

**Signature:** `random([max]) → INT`
**Signature:** `random(min, max) → INT`

**Description:** Returns random integer.

**Behavior:**
- `random()` → random 32-bit integer
- `random(max)` → integer in [1, max]
- `random(min, max)` → integer in [min, max]

**Examples:**
```moo
random()         => 1234567890  (varies)
random(6)        => 1-6 (dice roll)
random(1, 100)   => 1-100
```

**Errors:**
- E_RANGE: max ≤ 0
- E_RANGE: min > max
- E_TYPE: Non-integer argument

---

### 1.5 frandom (ToastStunt)

**Signature:** `frandom() → FLOAT`
**Signature:** `frandom(max) → FLOAT`
**Signature:** `frandom(min, max) → FLOAT`

**Description:** Returns random float.

**Behavior:**
- `frandom()` → float in [0.0, 1.0)
- `frandom(max)` → float in [0.0, max)
- `frandom(min, max)` → float in [min, max)

**Examples:**
```moo
frandom()        => 0.7234...  (varies)
frandom(10.0)    => 0.0-10.0
```

**Errors:**
- E_TYPE: Non-numeric argument

---

## 2. Integer Operations

### 2.1 floatstr

**Signature:** `floatstr(float, precision [, scientific]) → STR`

**Description:** Formats float as string with specified precision.

**Parameters:**
- `float`: Number to format
- `precision`: Decimal places (0-19)
- `scientific`: If true, use scientific notation

**Examples:**
```moo
floatstr(3.14159, 2)         => "3.14"
floatstr(3.14159, 4)         => "3.1416"
floatstr(1234567.89, 2, 1)   => "1.23e+06"
```

**Errors:**
- E_TYPE: First arg not numeric
- E_INVARG: Precision out of range

---

### 2.2 ceil

**Signature:** `ceil(float) → FLOAT`

**Description:** Returns smallest integer ≥ argument.

**Examples:**
```moo
ceil(3.2)   => 4.0
ceil(3.0)   => 3.0
ceil(-3.2)  => -3.0
```

**Errors:**
- E_TYPE: Non-numeric argument

---

### 2.3 floor

**Signature:** `floor(float) → FLOAT`

**Description:** Returns largest integer ≤ argument.

**Examples:**
```moo
floor(3.8)   => 3.0
floor(3.0)   => 3.0
floor(-3.2)  => -4.0
```

**Errors:**
- E_TYPE: Non-numeric argument

---

### 2.4 trunc

**Signature:** `trunc(float) → FLOAT`

**Description:** Truncates toward zero.

**Examples:**
```moo
trunc(3.8)   => 3.0
trunc(-3.8)  => -3.0
```

**Errors:**
- E_TYPE: Non-numeric argument

---

## 3. Trigonometric Functions

All angles in radians.

### 3.1 sin

**Signature:** `sin(angle) → FLOAT`

**Examples:**
```moo
sin(0)           => 0.0
sin(3.14159/2)   => 1.0  (approximately)
```

---

### 3.2 cos

**Signature:** `cos(angle) → FLOAT`

**Examples:**
```moo
cos(0)           => 1.0
cos(3.14159)     => -1.0  (approximately)
```

---

### 3.3 tan

**Signature:** `tan(angle) → FLOAT`

**Examples:**
```moo
tan(0)           => 0.0
tan(3.14159/4)   => 1.0  (approximately)
```

**Errors:**
- E_FLOAT: At asymptotes (π/2, 3π/2, etc.)

---

### 3.4 asin

**Signature:** `asin(value) → FLOAT`

**Description:** Arc sine (inverse sine).

**Domain:** [-1, 1]
**Range:** [-π/2, π/2]

**Examples:**
```moo
asin(0)    => 0.0
asin(1)    => 1.5707...  (π/2)
```

**Errors:**
- E_FLOAT: |value| > 1

---

### 3.5 acos

**Signature:** `acos(value) → FLOAT`

**Description:** Arc cosine (inverse cosine).

**Domain:** [-1, 1]
**Range:** [0, π]

**Examples:**
```moo
acos(1)    => 0.0
acos(0)    => 1.5707...  (π/2)
```

**Errors:**
- E_FLOAT: |value| > 1

---

### 3.6 atan

**Signature:** `atan(value) → FLOAT`
**Signature:** `atan(y, x) → FLOAT`

**Description:** Arc tangent. Two-argument form gives angle of point (x, y).

**Examples:**
```moo
atan(1)      => 0.7853...  (π/4)
atan(1, 1)   => 0.7853...  (π/4)
atan(1, -1)  => 2.3561...  (3π/4)
```

---

### 3.7 sinh

**Signature:** `sinh(value) → FLOAT`

**Description:** Hyperbolic sine.

---

### 3.8 cosh

**Signature:** `cosh(value) → FLOAT`

**Description:** Hyperbolic cosine.

---

### 3.9 tanh

**Signature:** `tanh(value) → FLOAT`

**Description:** Hyperbolic tangent.

---

## 4. Exponential and Logarithmic

### 4.1 sqrt

**Signature:** `sqrt(value) → FLOAT`

**Description:** Square root.

**Examples:**
```moo
sqrt(4)     => 2.0
sqrt(2)     => 1.4142...
sqrt(0)     => 0.0
```

**Errors:**
- E_FLOAT: Negative argument

---

### 4.2 exp

**Signature:** `exp(value) → FLOAT`

**Description:** e raised to power.

**Examples:**
```moo
exp(0)    => 1.0
exp(1)    => 2.7182...  (e)
exp(2)    => 7.3890...
```

**Errors:**
- E_FLOAT: Overflow

---

### 4.3 log

**Signature:** `log(value) → FLOAT`

**Description:** Natural logarithm (base e).

**Examples:**
```moo
log(1)        => 0.0
log(2.7182)   => 1.0  (approximately)
log(10)       => 2.3025...
```

**Errors:**
- E_FLOAT: value ≤ 0

---

### 4.4 log10

**Signature:** `log10(value) → FLOAT`

**Description:** Base-10 logarithm.

**Examples:**
```moo
log10(1)      => 0.0
log10(10)     => 1.0
log10(100)    => 2.0
```

**Errors:**
- E_FLOAT: value ≤ 0

---

## 5. Bitwise Operations

### 5.1 bitand

**Signature:** `bitand(int1, int2) → INT`

**Description:** Bitwise AND.

**Examples:**
```moo
bitand(12, 10)   => 8    (1100 & 1010 = 1000)
bitand(255, 15)  => 15
```

**Errors:**
- E_TYPE: Non-integer arguments

---

### 5.2 bitor

**Signature:** `bitor(int1, int2) → INT`

**Description:** Bitwise OR.

**Examples:**
```moo
bitor(12, 10)   => 14   (1100 | 1010 = 1110)
bitor(8, 1)     => 9
```

**Errors:**
- E_TYPE: Non-integer arguments

---

### 5.3 bitxor

**Signature:** `bitxor(int1, int2) → INT`

**Description:** Bitwise XOR.

**Examples:**
```moo
bitxor(12, 10)   => 6    (1100 ^ 1010 = 0110)
bitxor(255, 255) => 0
```

**Errors:**
- E_TYPE: Non-integer arguments

---

### 5.4 bitnot

**Signature:** `bitnot(int) → INT`

**Description:** Bitwise NOT (complement).

**Examples:**
```moo
bitnot(0)    => -1
bitnot(-1)   => 0
```

**Errors:**
- E_TYPE: Non-integer argument

---

### 5.5 bitshl (ToastStunt)

**Signature:** `bitshl(int, count) → INT`

**Description:** Bitwise left shift.

**Examples:**
```moo
bitshl(1, 4)    => 16
bitshl(5, 2)    => 20
```

**Errors:**
- E_TYPE: Non-integer arguments
- E_INVARG: Negative shift count

---

### 5.6 bitshr (ToastStunt)

**Signature:** `bitshr(int, count) → INT`

**Description:** Bitwise right shift (arithmetic).

**Examples:**
```moo
bitshr(16, 4)   => 1
bitshr(20, 2)   => 5
```

**Errors:**
- E_TYPE: Non-integer arguments
- E_INVARG: Negative shift count

---

## 6. Special Values

### 6.1 floatinfo (ToastStunt)

**Signature:** `floatinfo() → LIST`

**Description:** Returns floating-point implementation details.

**Returns:**
```moo
{max_float, min_positive_float, max_exponent, min_exponent, digits, epsilon}
```

---

### 6.2 intinfo (ToastStunt)

**Signature:** `intinfo() → LIST`

**Description:** Returns integer implementation details.

**Returns:**
```moo
{min_int, max_int, bytes}
```

---

## 7. Advanced Math (ToastStunt)

### 7.1 cbrt

**Signature:** `cbrt(value) → FLOAT`

**Description:** Cube root.

**Examples:**
```moo
cbrt(8)     => 2.0
cbrt(-8)    => -2.0
```

---

### 7.2 log2

**Signature:** `log2(value) → FLOAT`

**Description:** Base-2 logarithm.

**Examples:**
```moo
log2(8)     => 3.0
log2(1024)  => 10.0
```

---

### 7.3 hypot

**Signature:** `hypot(x, y) → FLOAT`

**Description:** Euclidean distance: sqrt(x² + y²).

**Examples:**
```moo
hypot(3, 4)   => 5.0
hypot(1, 1)   => 1.4142...
```

---

### 7.4 fmod

**Signature:** `fmod(x, y) → FLOAT`

**Description:** Floating-point modulo.

**Examples:**
```moo
fmod(5.5, 2.0)   => 1.5
fmod(-5.5, 2.0)  => -1.5
```

---

### 7.5 remainder

**Signature:** `remainder(x, y) → FLOAT`

**Description:** IEEE remainder (rounds to nearest).

---

### 7.6 copysign

**Signature:** `copysign(magnitude, sign) → FLOAT`

**Description:** Returns magnitude with sign of second argument.

**Examples:**
```moo
copysign(5.0, -1.0)   => -5.0
copysign(-5.0, 1.0)   => 5.0
```

---

### 7.7 ldexp

**Signature:** `ldexp(x, exp) → FLOAT`

**Description:** Returns x × 2^exp.

---

### 7.8 frexp

**Signature:** `frexp(x) → LIST`

**Description:** Decomposes float into mantissa and exponent.

**Returns:** `{mantissa, exponent}` where x = mantissa × 2^exponent

---

### 7.9 modf

**Signature:** `modf(x) → LIST`

**Description:** Splits into integer and fractional parts.

**Returns:** `{integer_part, fractional_part}`

**Examples:**
```moo
modf(3.14)   => {3.0, 0.14}
modf(-3.14)  => {-3.0, -0.14}
```

---

### 7.10 isinf

**Signature:** `isinf(value) → BOOL`

**Description:** Tests if value is infinity.

---

### 7.11 isnan

**Signature:** `isnan(value) → BOOL`

**Description:** Tests if value is NaN.

---

### 7.12 isfinite

**Signature:** `isfinite(value) → BOOL`

**Description:** Tests if value is finite (not inf, not nan).

---

## 8. Error Handling

All math functions raise E_TYPE for wrong argument types.

Domain errors (sqrt(-1), log(0), asin(2)) raise E_FLOAT.

Overflow may raise E_FLOAT or return infinity depending on implementation.

---

## Go Implementation Notes

```go
import "math"

func builtinSqrt(args []Value) (Value, error) {
    if len(args) != 1 {
        return nil, E_ARGS
    }
    f, err := toFloat(args[0])
    if err != nil {
        return nil, E_TYPE
    }
    if f < 0 {
        return nil, E_FLOAT
    }
    return FloatValue(math.Sqrt(f)), nil
}

func builtinRandom(args []Value) (Value, error) {
    switch len(args) {
    case 0:
        return IntValue(rand.Int63()), nil
    case 1:
        max, err := toInt(args[0])
        if err != nil {
            return nil, E_TYPE
        }
        if max <= 0 {
            return nil, E_RANGE
        }
        return IntValue(rand.Int63n(max) + 1), nil
    case 2:
        min, _ := toInt(args[0])
        max, _ := toInt(args[1])
        if min > max {
            return nil, E_RANGE
        }
        return IntValue(min + rand.Int63n(max-min+1)), nil
    default:
        return nil, E_ARGS
    }
}
```
