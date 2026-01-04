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

**Signature:** `ftime([clock_type]) → FLOAT`

**Description:** Returns current time with sub-second precision.

**Parameters:**
- `clock_type` (optional INT): Clock source
  - 0 or omitted: Real-time clock (wall clock time)
  - 1: Monotonic clock (time since system boot, not affected by time adjustments)
  - 2: Monotonic raw clock (monotonic, not adjusted by NTP)

**Examples:**
```moo
ftime()      => 1703419200.123456  (realtime)
ftime(0)     => 1703419200.123456  (realtime)
ftime(1)     => 433501.445197      (monotonic)
ftime(2)     => 433635.761586      (monotonic raw)
```

**Notes:**
- Monotonic clocks are useful for measuring elapsed time intervals
- Realtime clock can jump backwards if system time is adjusted

---

## 2. Time Formatting

### 2.1 ctime

**Signature:** `ctime([time]) → STR`

**Description:** Converts Unix timestamp to human-readable local time string.

**Parameters:**
- `time` (optional INT): Unix timestamp. If omitted, uses current time.

**Returns:** String in format `"Www Mmm dd hh:mm:ss yyyy TZ"` where:
- Www = day of week (3 chars)
- Mmm = month (3 chars)
- dd = day of month (2 digits, space-padded)
- hh:mm:ss = time in 24-hour format
- yyyy = year (4 digits)
- TZ = timezone name (system-dependent)

**Examples:**
```moo
ctime()              => "Sun Jan  4 00:10:54 2026 Mountain Standard Time"
ctime(1000000000)    => "Sat Sep  8 19:46:40 2001 Mountain Daylight Time"
ctime(time())        => current time string
```

**Notes:**
- Uses local timezone (result varies by server location)
- Format is system-dependent
- Years beyond max int are clamped to prevent overflow

**Errors:**
- E_TYPE: Non-integer timestamp

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
- E_TYPE: Argument is not an object
- E_INVARG: Not a connected player

---

### 4.2 connected_seconds

**Signature:** `connected_seconds(player) → INT`

**Description:** Returns seconds since player connected.

**Examples:**
```moo
connected_seconds(player)   => 3600 (1 hour connected)
```

**Errors:**
- E_TYPE: Argument is not an object
- E_INVARG: Not a connected player

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
