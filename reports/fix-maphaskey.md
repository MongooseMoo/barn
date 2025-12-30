# Fix maphaskey() to Return Integer Instead of String

## Problem

The `maphaskey()` builtin was returning Go boolean values which serialized to MOO strings `"true"` and `"false"` instead of MOO integers `1` and `0`.

## Root Cause

In `builtins/maps.go` at line 208, the function was returning:
```go
return types.Ok(types.BoolValue{Val: found})
```

This returned a Go `BoolValue` type which serializes to the MOO strings `"true"` and `"false"` rather than the MOO integers `1` and `0` that are standard for boolean results in MOO.

## Solution

Changed the return to follow the pattern used by other boolean builtins like `is_player()` and `valid()`:

```go
_, found := m.Get(key)
if found {
    return types.Ok(types.NewInt(1))
}
return types.Ok(types.NewInt(0))
```

Also updated the function comment from `maphaskey(map, key) -> bool` to `maphaskey(map, key) -> int (1 if found, 0 if not)`.

## Testing

### Before Fix
```moo
; return maphaskey(["a" -> 1], "a");
=> {1, true}  // String "true"

; return maphaskey(["a" -> 1], "b");
=> {1, false}  // String "false"
```

### After Fix
```moo
; return maphaskey(["a" -> 1], "a");
=> {1, 1}  // Integer 1

; return maphaskey(["a" -> 1], "b");
=> {1, 0}  // Integer 0

; return typeof(maphaskey(["a" -> 1], "a"));
=> {1, 0}  // TYPE_INT (0)
```

## Files Changed

- `builtins/maps.go`: Updated `builtinMaphaskey()` to return integers instead of booleans

## Commit

```
commit fc0afa4
Fix maphaskey() to return integer instead of string
```
