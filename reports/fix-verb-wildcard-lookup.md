# Fix Verb Wildcard Lookup - Task Report

## Summary

Successfully fixed the barn CLI verb lookup to support MOO wildcard pattern matching. The `-verb-code` flag now correctly resolves verbs with wildcard names like `co*nnect` when searching with names like `connect`.

## Problem

The barn CLI has a `-verb-code "#10:connect"` flag for inspecting verb code. However, it was failing with "verb not found" because MOO verbs use wildcard patterns in their names (e.g., `co*nnect @co*nnect`), and the lookup was doing exact string matching instead of wildcard matching.

## Solution

### 1. Implemented MOO Wildcard Matching Function

Added `matchVerbName()` function to `db/store.go` (lines 335-372):

```go
func matchVerbName(verbPattern, searchName string) bool {
    // Case-insensitive matching
    pattern := strings.ToLower(verbPattern)
    search := strings.ToLower(searchName)

    // Find the wildcard position
    starPos := strings.Index(pattern, "*")
    if starPos == -1 {
        // No wildcard, exact match required
        return pattern == search
    }

    // Split pattern at wildcard: "co*nnect" -> prefix="co", suffix="nnect"
    prefix := pattern[:starPos]
    suffix := pattern[starPos+1:]

    // Check if search string has the required prefix and suffix
    if !strings.HasPrefix(search, prefix) {
        return false
    }
    if !strings.HasSuffix(search, suffix) {
        return false
    }

    // Ensure prefix and suffix don't overlap
    return len(search) >= len(prefix)+len(suffix)
}
```

**How it works:**
- Pattern `co*nnect` splits into prefix `co` and suffix `nnect`
- Search string must start with prefix and end with suffix
- Zero or more characters can appear where `*` is located
- Case-insensitive matching (MOO convention)

### 2. Updated FindVerb to Use Wildcard Matching

Modified `db.Store.FindVerb()` at line 419 to use the new matching function:

**Before:**
```go
if alias == verbName {
    return verb, current, nil
}
```

**After:**
```go
if matchVerbName(alias, verbName) {
    return verb, current, nil
}
```

### 3. Added strings import

Added `"strings"` to imports in `db/store.go` (line 6).

## Testing

All test cases passed:

### Test 1: Wildcard pattern `co*nnect`
```bash
$ ./barn.exe -db toastcore.db -verb-code "#10:connect"
=== #10:connect ===
Names: co*nnect @co*nnect
Owner: #2
Perms: rxd
--- Code (117 lines) ---
```
✓ "connect" correctly matches "co*nnect" with zero chars where * is

### Test 2: Wildcard pattern `w*ho`
```bash
$ ./barn.exe -db toastcore.db -verb-code "#10:who"
=== #10:who ===
Names: w*ho @w*ho
Owner: #2
Perms: rxd
--- Code (20 lines) ---
```
✓ "who" correctly matches "w*ho"

### Test 3: Wildcard pattern `cr*eate`
```bash
$ ./barn.exe -db toastcore.db -verb-code "#10:create"
=== #10:create ===
Names: cr*eate @cr*eate
Owner: #2
Perms: rxd
--- Code (47 lines) ---
```
✓ "create" correctly matches "cr*eate"

### Test 4: Exact match (no wildcard)
```bash
$ ./barn.exe -db toastcore.db -verb-code "#10:parse_command"
=== #10:parse_command ===
Names: parse_command
Owner: #2
Perms: rxd
--- Code (24 lines) ---
```
✓ Exact matching still works for verbs without wildcards

### Test 5: Multiple wildcard expansion
```bash
$ ./barn.exe -db toastcore.db -verb-code "#10:connnnect"
=== #10:connnnect ===
Names: co*nnect @co*nnect
Owner: #2
Perms: rxd
--- Code (117 lines) ---
```
✓ "connnnect" correctly matches "co*nnect" with extra chars where * is

### Test 6: Invalid partial match
```bash
$ ./barn.exe -db toastcore.db -verb-code "#10:co"
Error: verb not found: co
```
✓ "co" correctly fails to match "co*nnect" (doesn't end with "nnect")

## Files Modified

- `C:\Users\Q\code\barn\db\store.go`
  - Added `strings` import
  - Added `matchVerbName()` function (lines 335-372)
  - Updated verb alias matching to use wildcard matching (line 419)

## MOO Wildcard Semantics

The implementation follows MOO verb wildcard semantics:
- `*` means "zero or more characters can appear here"
- Pattern `co*nnect` requires:
  - String starts with `co`
  - String ends with `nnect`
  - Zero or more characters between prefix and suffix
- Matching is case-insensitive
- Only one `*` per verb name is supported (standard MOO behavior)

## Outcome

The CLI verb lookup now works correctly with MOO wildcard patterns. Users can use natural verb names (like "connect", "who", "create") to look up verbs that have wildcard patterns in their definitions.

## Status

✓ Task completed successfully
✓ All tests passing
✓ No regressions in exact matching
✓ Follows MOO wildcard semantics correctly
