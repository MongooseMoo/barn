# Fix Login Flow Report

## Summary

Fixed the login flow for toastcore.db. The "connect wizard" command now successfully authenticates and logs the user in as the wizard player (#2).

## Bugs Fixed

### 1. `in` Operator Returning Boolean Instead of Index

**Files:** `vm/operations.go`, `vm/operators.go`

The `in` operator was returning a boolean (true/false) instead of the 1-based index of the element in the list. This broke the `find_exact` verb which relies on the index:

```moo
if (i = search in info[3])
    return info[this.data][i];
```

**Fix:** Changed both VM and interpreter paths to return `IntValue{Val: i+1}` for found elements and `IntValue{Val: 0}` for not found.

### 2. CLEAR Properties Not Setting Clear Flag

**File:** `db/reader.go`

When loading the database, properties with type code 5 (CLEAR) were being read but the `prop.Clear` flag was not being set. This caused inherited property values to return nil instead of inheriting from the parent.

For example, `#39.data` was returning nil instead of inheriting the value 4 from its parent #37.

**Fix:** Added check after reading property value:
```go
if prop.Value == nil {
    prop.Clear = true
}
```

### 3. Builtin Properties Taking Precedence Over Defined Properties

**File:** `vm/properties.go`

The property lookup was checking builtin properties (like `.player`, `.wizard`) BEFORE checking for defined properties. This caused `$player` (which is `#0.player`) to return 0 (the builtin flag value) instead of the player prototype object (#6).

**Fix:** Reversed the order - now defined properties are checked first, falling back to builtin flag properties only if no defined property exists.

## Verification

After these fixes:
1. `connect wizard` successfully authenticates
2. `switch_player` is called to associate the connection with player #2
3. The connection enters the command loop as player 2
4. Commands like `; return player;` execute and return `{1, #2}` (success, wizard object)

## Remaining Issues

Some command output is not being delivered to the client after login. This appears to be a separate issue with how notifications are routed post-login, not related to the login flow itself.

## Files Modified

- `vm/operations.go` - Fixed `in` operator in VM path
- `vm/operators.go` - Fixed `in` operator in interpreter path
- `db/reader.go` - Set Clear flag for CLEAR properties (two locations: v4 and v17 parsing)
- `vm/properties.go` - Defined properties now take precedence over builtin flag properties
