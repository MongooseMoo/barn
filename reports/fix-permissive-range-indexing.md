# Fix Permissive Range Indexing - Investigation Report

## Summary

**Barn's range indexing is ALREADY CORRECT** and matches Toast's permissive behavior for backwards ranges. The `news` command failure is caused by a different issue, not by range indexing semantics.

## Investigation

### Toast Behavior Verification

Tested Toast's behavior with the `toast_oracle` tool:

```bash
./toast_oracle.exe '{1, 2, 3}[3..1]'   # Returns: {}
./toast_oracle.exe '{1}[$..$-4]'       # Returns: {} (evaluates to [1..-3])
./toast_oracle.exe '{1}[1..4]'         # Returns: Range error
./toast_oracle.exe '{1, 2, 3}[1..3]'   # Returns: {1, 2, 3}
```

**Toast's rule**: Return empty list/string when `start > end` (backwards range), but throw Range error when indices are out of bounds for forward ranges.

### Barn's Implementation

Examined `vm/indexing.go`:

```go
func listRange(list types.ListValue, start, end int64) types.Result {
    length := int64(list.Len())

    // If start > end, return empty list (before bounds checking per MOO semantics)
    if start > end {
        return types.Ok(types.NewList([]types.Value{}))
    }

    // Check bounds
    if start < 1 || start > length {
        return types.Err(types.E_RANGE)
    }
    if end < 1 || end > length {
        return types.Err(types.E_RANGE)
    }
    // ...
}
```

This implementation:
1. **First** checks if `start > end` (backwards) → returns empty list
2. **Then** checks bounds → returns Range error if out of bounds

This is exactly Toast's behavior.

### Barn Behavior Verification

Tested Barn with Test.db:

```bash
; return {1, 2, 3}[3..1];    # Returns: {}
; return {1, 2, 3}[1..10];   # Returns: E_RANGE
; x = {1}; return x[1..-3];  # Returns: {}
```

**Result**: Barn matches Toast's behavior perfectly.

### News Command Failure Analysis

Added debug logging to `listRange` and ran the `news` command:

```
2025/12/29 01:01:59 listRange: start=1, end=0, length=0
2025/12/29 01:01:59 listRange: returning empty list (backwards range)
2025/12/29 01:01:59 listRange: start=1, end=2, length=2
2025/12/29 01:01:59 listRange: start=1, end=1, length=0
2025/12/29 01:01:59 listRange: E_RANGE (start out of bounds)
```

The error traceback:
```
#2 <- #46:messages_in_seq (this == #46), line 6:  Range error
```

The verb code at line 6:
```moo
return caller.messages[msgs[1]..msgs[2] - 1];
```

**Key findings**:
1. First call: `[1..0]` on empty list → **correctly** returns `{}` (backwards range)
2. Third call: `[1..1]` on empty list → **correctly** throws E_RANGE (not backwards, start out of bounds)

The third call has `start=1, end=1, length=0`. This is NOT a backwards range (`1 > 1` is false), so it correctly throws E_RANGE because we're trying to access index 1 of an empty list.

### Root Cause

The `news` command failure is NOT due to incorrect range semantics. The error comes from a legitimate out-of-bounds access `[1..1]` on an empty list, which Toast would also reject.

The failure happens somewhere in the call chain:
- `#46:messages_in_seq` line 6
- Called from `#45:messages_in_seq` line 24
- Called from `#61:news_display_seq_full` line 7
- Called from `#6:news` line 45

One of these callers is passing invalid indices to an empty messages list. This is likely a bug in the MOO code itself or in how Barn handles the messages property initialization.

## Conclusion

**No changes needed** to Barn's range indexing implementation. It already correctly implements Toast's permissive backwards-range semantics.

The `news` command failure is a separate issue, likely related to:
1. The `caller.messages` property being empty when it shouldn't be
2. The MOO code generating invalid indices for an empty list
3. A bug in how the news system determines message ranges

To fix the `news` command, investigate why `caller.messages` is empty and why the code is trying to access `[1..1]` on an empty list.

## Test Results

### Backwards Ranges (Permissive)
- ✅ `{1, 2, 3}[3..1]` → `{}`
- ✅ `{1}[1..-3]` → `{}`
- ✅ `"abc"[3..1]` → `""`

### Out-of-Bounds Ranges (Strict)
- ✅ `{1, 2, 3}[1..10]` → `E_RANGE`
- ✅ `{1}[1..4]` → `E_RANGE`
- ✅ `{}[1..1]` → `E_RANGE`

All tests pass and match Toast behavior.
