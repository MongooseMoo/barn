# Fix ctime() Format

## Summary
Modified the ctime() function in `builtins/system.go` to return a 24-character string by removing the timezone.

## Changes
- Removed "MST" from the time formatting string
- Before: `t.Format("Mon Jan  2 15:04:05 2006 MST")`
- After: `t.Format("Mon Jan  2 15:04:05 2006")`

## Verification
- The function now returns a 24-character string
- No timezone information is included in the output
- The date format remains consistent with the original implementation

## Files Modified
- `builtins/system.go`

## Status
✅ Completed