# Report: Implement GC Builtins

## Status: COMPLETE ✓

## Summary

Successfully implemented `run_gc()` and `gc_stats()` builtins for the Barn MOO server. Both builtins require wizard permissions and provide garbage collection functionality compatible with ToastStunt's implementation.

## Implementation Details

### Files Created
- `builtins/gc.go` - Contains `run_gc()` and `gc_stats()` implementations

### Files Modified
- `builtins/registry.go` - Registered both GC builtins

### run_gc()
- **Signature**: `run_gc()` - no arguments
- **Permissions**: Wizard only (returns E_PERM otherwise)
- **Behavior**: Triggers Go's garbage collector via `runtime.GC()`
- **Returns**: Integer 0 on success

### gc_stats()
- **Signature**: `gc_stats()` - no arguments
- **Permissions**: Wizard only (returns E_PERM otherwise)
- **Behavior**: Returns GC statistics map with 7 color keys
- **Returns**: Map with keys: `green`, `yellow`, `black`, `gray`, `white`, `purple`, `pink`
- **Values**: All currently return 0 (Go manages its own GC, no tri-color marking stats)

## Test Results

Ran 22 GC conformance tests:
- **20 PASSED** ✓
- **2 FAILED** (due to pre-existing bug in `maphaskey`, not GC implementation)

### Failing Tests
Both failures are due to `maphaskey()` returning string `"true"` instead of integer `1`:
1. `gc::gc_stats_has_purple_key` - maphaskey returns "true" instead of 1
2. `gc::gc_stats_has_black_key` - maphaskey returns "true" instead of 1

The GC builtins themselves are working correctly - verified via manual testing:
```
; return run_gc();
=> {1, 0}

; return gc_stats();
=> {1, ["black" -> 0, "gray" -> 0, "green" -> 0, "pink" -> 0, "purple" -> 0, "white" -> 0, "yellow" -> 0]}
```

### Passing Test Categories
- Permission tests (4/4)
- Structure tests (3/5 - 2 failed due to maphaskey)
- Nested structure tests (2/2)
- Anonymous object tests (1/1)
- Cyclic reference tests (8/8)
- Empty list/map tests (2/2)

## Reference Implementation

Reviewed ToastStunt implementation in `/c/Users/Q/src/toaststunt/src/garbage.cc`:
- `bf_run_gc` (lines 408-419) - Sets gc_run_called flag, returns no_var_pack()
- `bf_gc_stats` (lines 421-450) - Returns map with 7 color keys from gc_stats() function
- Both check `is_wizard(progr)` before proceeding

## Commit

```
commit e9f3f0b
Author: Q
Date:   2025-12-30

    Implement run_gc() and gc_stats() builtins

    - run_gc() triggers garbage collection (wizard only)
    - gc_stats() returns GC statistics map (wizard only)
    - Both require wizard permissions, return E_PERM otherwise
    - gc_stats() returns map with 7 color keys: green, yellow, black, gray, white, purple, pink
    - All values currently return 0 (Go manages its own GC)
    - 20/22 GC conformance tests pass (2 failures due to pre-existing maphaskey bug)
```

## Next Steps

The maphaskey bug should be fixed separately to get the remaining 2 GC tests passing. The issue is:
- `maphaskey([key -> val], key)` returns string `"true"` instead of integer `1`
- This affects any code that checks `maphaskey() == 1` instead of truthy evaluation

## Files

### builtins/gc.go
```go
package builtins

import (
	"barn/types"
	"runtime"
)

// builtinRunGC implements run_gc()
// Triggers garbage collection (wizard only)
// Returns 0 on success
func builtinRunGC(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	runtime.GC()
	return types.Ok(types.NewInt(0))
}

// builtinGCStats implements gc_stats()
// Returns GC statistics map (wizard only)
// Returns map with color keys: green, yellow, black, gray, white, purple, pink
func builtinGCStats(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Return map with all zeros (Go manages its own GC)
	result := types.NewEmptyMap()
	result = result.Set(types.NewStr("green"), types.NewInt(0))
	result = result.Set(types.NewStr("yellow"), types.NewInt(0))
	result = result.Set(types.NewStr("black"), types.NewInt(0))
	result = result.Set(types.NewStr("gray"), types.NewInt(0))
	result = result.Set(types.NewStr("white"), types.NewInt(0))
	result = result.Set(types.NewStr("purple"), types.NewInt(0))
	result = result.Set(types.NewStr("pink"), types.NewInt(0))

	return types.Ok(result)
}
```

### Registry Update
```go
// GC builtins
r.Register("run_gc", builtinRunGC)
r.Register("gc_stats", builtinGCStats)
```
