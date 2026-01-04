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

### 1.2 ftime (ToastStunt) [Not Implemented]

**Signature:** `ftime() → FLOAT`

**Description:** Returns current time with sub-second precision.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds. It may require optional compile-time flags or specific configuration.

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

### 2.2 strftime (ToastStunt) [Not Implemented]

**Signature:** `strftime(format [, time]) → STR`

**Description:** Formats time according to format string.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

**Format codes:**
| Code | Meaning |
|------|---------|
| %Y | Year (4 digits) |
| %m | Month (01-12) |
| %d | Day (01-31) |
| %H | Hour (00-23) |
| %M | Minute (00-59) |
| %S | Second (00-59) |
| %a | Weekday abbrev |
| %A | Weekday full |
| %b | Month abbrev |
| %B | Month full |
| %c | Locale datetime |
| %x | Locale date |
| %X | Locale time |
| %Z | Timezone |
| %% | Literal % |

**Examples:**
```moo
strftime("%Y-%m-%d")           => "2023-12-25"
strftime("%H:%M:%S")           => "12:30:45"
strftime("%A, %B %d, %Y")      => "Monday, December 25, 2023"
```

---

### 2.3 strptime (ToastStunt) [Not Implemented]

**Signature:** `strptime(string, format) → INT`

**Description:** Parses time string according to format.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

**Examples:**
```moo
strptime("2023-12-25", "%Y-%m-%d")   => 1703462400
```

**Errors:**
- E_INVARG: Parse failed

---

## 3. Time Components

### 3.1 gmtime (ToastStunt) [Not Implemented]

**Signature:** `gmtime([time]) → LIST`

**Description:** Breaks down time into UTC components.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

**Returns:**
```moo
{seconds, minutes, hours, day, month, year, weekday, yearday, isdst}
```

**Fields (0-based except as noted):**
| Index | Field | Range |
|-------|-------|-------|
| 1 | seconds | 0-59 |
| 2 | minutes | 0-59 |
| 3 | hours | 0-23 |
| 4 | day | 1-31 |
| 5 | month | 0-11 |
| 6 | year | years since 1900 |
| 7 | weekday | 0-6 (Sun=0) |
| 8 | yearday | 0-365 |
| 9 | isdst | daylight saving |

**Examples:**
```moo
gmtime(0)   => {0, 0, 0, 1, 0, 70, 4, 0, 0}
// Thu Jan 1, 1970 00:00:00 UTC
```

---

### 3.2 localtime (ToastStunt) [Not Implemented]

**Signature:** `localtime([time]) → LIST`

**Description:** Like gmtime but in local timezone.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

---

### 3.3 mktime (ToastStunt) [Not Implemented]

**Signature:** `mktime(components) → INT`

**Description:** Converts components back to timestamp.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

**Examples:**
```moo
mktime({0, 0, 0, 1, 0, 70, 0, 0, 0})   => 0
```

---

## 4. Timing

### 4.1 ticks_left

**Signature:** `ticks_left() → INT`

**Description:** Returns remaining ticks for current task.

See [tasks.md](tasks.md).

---

### 4.2 seconds_left

**Signature:** `seconds_left() → INT`

**Description:** Returns remaining time for current task.

See [tasks.md](tasks.md).

---

## 5. Idle Time

### 5.1 idle_seconds

**Signature:** `idle_seconds(player) → INT`

**Description:** Returns seconds since player's last input.

**Examples:**
```moo
idle_seconds(player)   => 120 (2 minutes idle)
```

**Errors:**
- E_INVARG: Not a connected player

---

### 5.2 connected_seconds

**Signature:** `connected_seconds(player) → INT`

**Description:** Returns seconds since player connected.

---

## 6. Server Uptime

### 6.1 server_started (ToastStunt) [Not Implemented]

**Signature:** `server_started() → INT`

**Description:** Returns timestamp when server started.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

---

### 6.2 uptime (ToastStunt) [Not Implemented]

**Signature:** `uptime() → INT`

**Description:** Returns seconds since server started.

**Availability:** This builtin is documented in ToastStunt source code but is not available in standard Toast builds.

**Examples:**
```moo
uptime()   => 86400 (1 day)
```

---

## 7. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Non-numeric timestamp |
| E_INVARG | Invalid format/components |

---

## Go Implementation Notes

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

func builtinStrftime(args []Value) (Value, error) {
    format := string(args[0].(StringValue))

    var t time.Time
    if len(args) > 1 {
        ts, _ := toInt(args[1])
        t = time.Unix(ts, 0)
    } else {
        t = time.Now()
    }

    // Convert MOO format to Go format
    goFormat := convertFormat(format)
    return StringValue(t.Format(goFormat)), nil
}

func builtinGmtime(args []Value) (Value, error) {
    var t time.Time
    if len(args) > 0 {
        ts, _ := toInt(args[0])
        t = time.Unix(ts, 0).UTC()
    } else {
        t = time.Now().UTC()
    }

    return &MOOList{data: []Value{
        IntValue(t.Second()),
        IntValue(t.Minute()),
        IntValue(t.Hour()),
        IntValue(t.Day()),
        IntValue(int(t.Month()) - 1),  // 0-based
        IntValue(t.Year() - 1900),
        IntValue(int(t.Weekday())),
        IntValue(t.YearDay() - 1),
        IntValue(0),  // UTC has no DST
    }}, nil
}
```
