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

**Signature:** `frandom(max) → FLOAT`
**Signature:** `frandom(min, max) → FLOAT`

**Description:** Returns random float.

**Behavior:**
- `frandom(max)` → float in [0.0, max)
- `frandom(min, max)` → float in [min, max)

**Examples:**
```moo
frandom(10.0)      => 7.695...  (varies, 0.0-10.0)
frandom(1.0, 5.0)  => 3.638...  (varies, 1.0-5.0)
```

**Errors:**
- E_TYPE: Non-numeric argument
- E_ARGS: No arguments (zero-arg form does not exist)

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
- E_TYPE: Non-float argument (integers not accepted)

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
- E_TYPE: Non-float argument (integers not accepted)

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
- E_TYPE: Non-float argument (integers not accepted)

---

## 3. Trigonometric Functions

All angles in radians. All functions require FLOAT arguments (integers not accepted).

### 3.1 sin

**Signature:** `sin(angle) → FLOAT`

**Examples:**
```moo
sin(0.0)                => 0.0
sin(3.14159265359/2.0)  => 1.0  (approximately)
```

**Errors:**
- E_TYPE: Non-float argument

---

### 3.2 cos

**Signature:** `cos(angle) → FLOAT`

**Examples:**
```moo
cos(0.0)          => 1.0
cos(3.14159265)   => -1.0  (approximately)
```

**Errors:**
- E_TYPE: Non-float argument

---

### 3.3 tan

**Signature:** `tan(angle) → FLOAT`

**Examples:**
```moo
tan(0.0)                 => 0.0
tan(3.14159265359/4.0)   => 1.0  (approximately)
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: At asymptotes (π/2, 3π/2, etc.)

---

### 3.4 asin

**Signature:** `asin(value) → FLOAT`

**Description:** Arc sine (inverse sine).

**Domain:** [-1, 1]
**Range:** [-π/2, π/2]

**Examples:**
```moo
asin(0.0)  => 0.0
asin(1.0)  => 1.5707963...  (π/2)
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: |value| > 1

---

### 3.5 acos

**Signature:** `acos(value) → FLOAT`

**Description:** Arc cosine (inverse cosine).

**Domain:** [-1, 1]
**Range:** [0, π]

**Examples:**
```moo
acos(1.0)  => 0.0
acos(0.0)  => 1.5707963...  (π/2)
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: |value| > 1

---

### 3.6 atan

**Signature:** `atan(value) → FLOAT`
**Signature:** `atan(y, x) → FLOAT`

**Description:** Arc tangent. Two-argument form gives angle of point (x, y).

**Examples:**
```moo
atan(1.0)       => 0.785398...  (π/4)
atan(1.0, 1.0)  => 0.785398...  (π/4)
atan(1.0, -1.0) => 2.356194...  (3π/4)
```

**Errors:**
- E_TYPE: Non-float argument

---

### 3.7 sinh

**Signature:** `sinh(value) → FLOAT`

**Description:** Hyperbolic sine.

**Errors:**
- E_TYPE: Non-float argument

---

### 3.8 cosh

**Signature:** `cosh(value) → FLOAT`

**Description:** Hyperbolic cosine.

**Errors:**
- E_TYPE: Non-float argument

---

### 3.9 tanh

**Signature:** `tanh(value) → FLOAT`

**Description:** Hyperbolic tangent.

**Errors:**
- E_TYPE: Non-float argument

---

## 4. Exponential and Logarithmic

All functions require FLOAT arguments (integers not accepted).

### 4.1 sqrt

**Signature:** `sqrt(value) → FLOAT`

**Description:** Square root.

**Examples:**
```moo
sqrt(4.0)   => 2.0
sqrt(2.0)   => 1.4142135...
sqrt(0.0)   => 0.0
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: Negative argument

---

### 4.2 exp

**Signature:** `exp(value) → FLOAT`

**Description:** e raised to power.

**Examples:**
```moo
exp(0.0)  => 1.0
exp(1.0)  => 2.718281828...  (e)
exp(2.0)  => 7.389056...
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: Overflow

---

### 4.3 log

**Signature:** `log(value) → FLOAT`

**Description:** Natural logarithm (base e).

**Examples:**
```moo
log(1.0)       => 0.0
log(2.718281)  => 1.0  (approximately)
log(10.0)      => 2.302585...
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: value ≤ 0

---

### 4.4 log10

**Signature:** `log10(value) → FLOAT`

**Description:** Base-10 logarithm.

**Examples:**
```moo
log10(1.0)    => 0.0
log10(10.0)   => 1.0
log10(100.0)  => 2.0
```

**Errors:**
- E_TYPE: Non-float argument
- E_FLOAT: value ≤ 0

---

## 5. Advanced Math (ToastStunt)

### 5.1 cbrt

**Signature:** `cbrt(value) → FLOAT`

**Description:** Cube root.

**Examples:**
```moo
cbrt(8.0)   => 2.0
cbrt(-8.0)  => -2.0
```

**Errors:**
- E_TYPE: Non-float argument

---

### 5.2 simplex_noise (ToastStunt)

**Signature:** `simplex_noise(coords) → FLOAT`

**Description:** Returns simplex noise for 1D to 4D coordinates.

**Parameters:**
- `coords`: LIST of 1 to 4 FLOAT values

**Returns:** FLOAT in approximately the range [-1.0, 1.0].

**Errors:**
- E_TYPE: `coords` is not a list of 1 to 4 floats

---

## 6. Error Handling

**Type Errors (E_TYPE):**
- All transcendental functions (sin, cos, tan, asin, acos, atan, sinh, cosh, tanh) require FLOAT arguments
- Rounding functions (ceil, floor, trunc) require FLOAT arguments
- Exponential/logarithmic functions (sqrt, exp, log, log10) require FLOAT arguments
- Passing integers to these functions raises E_TYPE
- abs(), min(), max() accept both INT and FLOAT

**Domain Errors (E_FLOAT):**
- sqrt(x) where x < 0
- log(x) where x ≤ 0
- log10(x) where x ≤ 0
- asin(x) where |x| > 1
- acos(x) where |x| > 1

**Argument Count Errors (E_ARGS):**
- min() with no arguments
- max() with no arguments
- Any function called with wrong number of arguments

**Overflow:**
- May raise E_FLOAT or return infinity depending on implementation

---

## 7. Go Implementation Notes

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
