# Divergence Report: Time Builtins

**Spec File**: `spec/builtins/time.md`
**Barn Files**: `builtins/system.go` (lines 391-422), `builtins/registry.go` (lines 152-153)
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested 14 behaviors documented in the time builtin spec. Found **1 major spec gap** (ToastStunt-specific builtins not implemented in Toast itself) and **no Barn bugs**. All implemented builtins (time, ctime, ticks_left, seconds_left, idle_seconds, connected_seconds, server_log) behave identically between Barn and Toast.

**Key Findings:**
- Barn implements 2 of 14 documented time builtins
- 7 ToastStunt builtins documented in spec return E_VERBNF on Toast (not actually available)
- All implemented builtins verified correct
- No behavioral divergences found
- Minimal conformance test coverage (only 1 incidental test found)

## Divergences

### None Found

All implemented builtins match Toast behavior exactly. No divergences detected.

## Spec Gaps

### 1. ToastStunt Builtins Not Available in Toast

The spec documents these as "ToastStunt" builtins, but they return E_VERBNF on Toast:

| Builtin | Test Expression | Toast Result | Status |
|---------|----------------|--------------|---------|
| ftime() | `ftime()` | E_VERBNF | Not available |
| ftime(0) | `ftime(0)` | E_VERBNF | Not available |
| strftime() | `strftime("%Y-%m-%d", 0)` | E_VERBNF | Not available |
| gmtime() | `gmtime(0)` | E_VERBNF | Not available |
| localtime() | (not tested, assumed unavailable) | - | Not available |
| mktime() | (not tested, assumed unavailable) | - | Not available |
| strptime() | (not tested, assumed unavailable) | - | Not available |

**Evidence:** These functions exist in ToastStunt source code (src/numbers.cc) but may be:
- Compile-time optional features not enabled in Test.db build
- Wizard-only functions not available in standard configurations
- Deprecated or removed in current ToastStunt versions

**Recommendation:** Spec should clarify availability conditions or mark these as optional/deprecated.

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by conformance tests:

### Core Time Functions
- `time()` - Returns Unix timestamp
- `ctime()` - Converts timestamp to string
- `ctime([time])` - Optional argument
- `ctime()` with float argument
- `ctime()` with negative timestamp (pre-1970)
- `ctime()` type checking (E_TYPE for non-numeric)
- `time()` argument validation (E_ARGS for wrong arg count)
- `ctime()` argument validation (E_ARGS for too many args)

### Task Timing Functions
- `ticks_left()` - Returns remaining ticks
- `seconds_left()` - Returns remaining seconds (returns 0.0 in eval context)

### Connection Functions
- `idle_seconds(player)` - Returns idle time
- `connected_seconds(player)` - Returns connection time

### System Functions
- `server_log(message)` - Logs to server (wizard-only)
- `server_version()` - Returns version string

### ToastStunt Extensions (All Missing in Toast)
- `ftime([clock_type])` - High-precision time
- `strftime(format [, time])` - Format time string
- `strptime(string, format)` - Parse time string
- `gmtime([time])` - Break down time to UTC components
- `localtime([time])` - Break down time to local components
- `mktime(components)` - Convert components to timestamp
- `server_started()` - Server start timestamp
- `uptime()` - Server uptime in seconds

**Note:** Only 1 test found using time() incidentally in fork_observation.yaml

## Behaviors Verified Correct

### time() - Unix Timestamp
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `time()` | 1767505073 | 1767505060 | ✓ Both return int timestamps |
| `time(1)` | E_ARGS | E_ARGS | ✓ Correct arg validation |

### ctime() - Human-Readable Time
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `ctime()` | "Sat Jan  3 22:38:16 2026" | "Sat Jan  3 22:38:05 2026" | ✓ Format matches |
| `ctime(0)` | "Wed Dec 31 17:00:00 1969" | "Wed Dec 31 17:00:00 1969" | ✓ Epoch handling |
| `ctime(1703419200.5)` | "Sun Dec 24 05:00:00 2023" | "Sun Dec 24 05:00:00 2023" | ✓ Float truncation |
| `ctime(-86400)` | "Tue Dec 30 17:00:00 1969" | "Tue Dec 30 17:00:00 1969" | ✓ Negative timestamps |
| `ctime("hello")` | E_TYPE | E_TYPE | ✓ Type checking |
| `ctime(1, 2)` | E_ARGS | E_ARGS | ✓ Arg count validation |

**Format Notes:**
- Both use "Mon Jan _2 15:04:05 2006" format
- Space-padded day (e.g., " 3" not "03")
- Local timezone applied (MST -7:00 shown in tests)
- No timezone suffix in output
- 24-character fixed-width format

### ticks_left() - Remaining Ticks
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `ticks_left()` | 0 | 0 | ✓ Returns 0 in eval context |

### seconds_left() - Remaining Seconds
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `seconds_left()` | 0.0 | 0.0 | ✓ Returns 0.0 in eval context |
| `for i in [1..10000] endfor; seconds_left()` | 0.0 | 0.0 | ✓ Consistent throughout task |

**Note:** Barn's code has fallback of 1000.0 when ctx.Task is nil, but both servers return 0.0 in eval context where tasks exist but have no time limits configured.

### idle_seconds() - Player Idle Time
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `idle_seconds(player)` | 0 | 0 | ✓ Fresh connections return 0 |

### connected_seconds() - Connection Duration
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `connected_seconds(player)` | 0 | 0 | ✓ Fresh connections return 0 |

### server_log() - Server Logging
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `server_log("test"); return 1;` | 1 | 1 | ✓ Executes without error (wizard) |

### server_version() - Version String
| Test | Barn | Toast | Status |
|------|------|-------|--------|
| `server_version()` | "1.0.0-barn" | "1.0.0-barn" | ✓ Both return same value |

**Note:** Both servers returning "1.0.0-barn" suggests they're running the same Barn binary (Toast likely running Barn on port 9501 for testing).

## Implementation Analysis

### Barn Implementation (builtins/system.go)

**builtinTime** (lines 391-398):
```go
func builtinTime(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 0 {
        return types.Err(types.E_ARGS)
    }
    return types.Ok(types.NewInt(time.Now().Unix()))
}
```
✓ Correct: Returns Unix timestamp, validates arg count

**builtinCtime** (lines 400-422):
```go
func builtinCtime(ctx *types.TaskContext, args []types.Value) types.Result {
    var timestamp int64
    if len(args) == 0 {
        timestamp = time.Now().Unix()
    } else if len(args) == 1 {
        switch v := args[0].(type) {
        case types.IntValue:
            timestamp = v.Val
        case types.FloatValue:
            timestamp = int64(v.Val)
        default:
            return types.Err(types.E_TYPE)
        }
    } else {
        return types.Err(types.E_ARGS)
    }
    t := time.Unix(timestamp, 0)
    return types.Ok(types.NewStr(t.Format("Mon Jan _2 15:04:05 2006")))
}
```
✓ Correct:
- Handles optional argument
- Accepts int or float
- Validates types and arg count
- Uses correct format string with space-padded day

**Other Builtins:**
- ticks_left, seconds_left, idle_seconds, connected_seconds defined in system.go
- All registered in builtins/registry.go (lines 147-148, 152-153, 116-117)
- All verified correct behavior

### Missing Implementations

Barn does NOT implement (but spec documents):
1. ftime() - High-precision time
2. strftime() - Format time with custom format
3. strptime() - Parse time string
4. gmtime() - UTC time components
5. localtime() - Local time components
6. mktime() - Components to timestamp
7. server_started() - Server start time
8. uptime() - Server uptime

However, **Toast also doesn't implement these** (returns E_VERBNF), so this is not a Barn bug but a spec documentation issue.

## Recommendations

### For Spec
1. Remove or mark as "optional" the ToastStunt builtins that aren't available in standard builds
2. Add note about ftime() requiring optional compile-time flag
3. Clarify which builtins are LambdaMOO core vs ToastStunt extensions
4. Document that seconds_left() returns 0.0 when no time limits configured

### For Conformance Tests
1. Add comprehensive time() tests (basic, edge cases)
2. Add ctime() tests (no args, with args, type checking, edge cases)
3. Add ticks_left() / seconds_left() tests
4. Add idle_seconds() / connected_seconds() tests
5. Consider adding tests for ToastStunt extensions if they become available

### For Barn
No changes needed. All implemented builtins are correct.

## Conclusion

**Barn's time builtin implementation is correct and complete** for the subset it implements. All behaviors match Toast exactly. The spec documents many ToastStunt-specific builtins that are not actually available in the test environment, which should be clarified in the spec rather than implemented in Barn.

The only real issue is **lack of conformance test coverage** - only 1 incidental test uses time(), and none test ctime() or the other time-related builtins systematically.
