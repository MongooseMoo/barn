# MOO Time Built-ins

## Overview

Functions for time and date operations.

---

## 1. Current Time

### 1.1 time

**Signature:** `time() → INT`

**Description:** Returns current Unix timestamp (seconds since 1970-01-01 00:00:00 UTC).

**Examples:**
```moo
time()   => 1703419200 (varies)
```

---

### 1.2 ftime (ToastStunt)

**Signature:** `ftime() → FLOAT`

**Description:** Returns current time with sub-second precision.

**Examples:**
```moo
ftime()   => 1703419200.123456
```

---

## 2. Time Formatting

### 2.1 ctime

**Signature:** `ctime([time]) → STR`

**Description:** Converts timestamp to human-readable string.

**Examples:**
```moo
ctime()              => "Mon Dec 25 12:00:00 2023"
ctime(0)             => "Thu Jan  1 00:00:00 1970"
ctime(time())        => current time string
```

---

## 3. Timing

### 3.1 ticks_left

**Signature:** `ticks_left() → INT`

**Description:** Returns remaining ticks for current task.

See [tasks.md](tasks.md).

---

### 3.2 seconds_left

**Signature:** `seconds_left() → INT`

**Description:** Returns remaining time for current task.

See [tasks.md](tasks.md).

---

## 4. Idle Time

### 4.1 idle_seconds

**Signature:** `idle_seconds(player) → INT`

**Description:** Returns seconds since player's last input.

**Examples:**
```moo
idle_seconds(player)   => 120 (2 minutes idle)
```

**Errors:**
- E_INVARG: Not a connected player

---

### 4.2 connected_seconds

**Signature:** `connected_seconds(player) → INT`

**Description:** Returns seconds since player connected.

---

## 5. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Non-numeric timestamp |
| E_INVARG | Invalid format/components |

---

## 6. Go Implementation Notes

```go
import "time"

func builtinTime(args []Value) (Value, error) {
    return IntValue(time.Now().Unix()), nil
}

func builtinFtime(args []Value) (Value, error) {
    now := time.Now()
    secs := float64(now.Unix()) + float64(now.Nanosecond())/1e9
    return FloatValue(secs), nil
}

func builtinCtime(args []Value) (Value, error) {
    var t time.Time
    if len(args) > 0 {
        ts, _ := toInt(args[0])
        t = time.Unix(ts, 0)
    } else {
        t = time.Now()
    }
    return StringValue(t.Format(time.ANSIC)), nil
}
```
