package builtins

import (
	"barn/types"
	"runtime"
)

// ============================================================================
// GARBAGE COLLECTION BUILTINS
// ============================================================================

// builtinRunGC implements run_gc()
// Triggers garbage collection (wizard only)
// Returns 0 on success
func builtinRunGC(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Trigger Go's garbage collector
	// This is primarily symbolic since Go manages its own GC,
	// but for anonymous objects with cyclic references, this provides
	// a way to force collection
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

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Since Go manages its own GC and we don't have a tri-color marking
	// algorithm like ToastStunt's cyclic reference collector,
	// we return a map with all zeros
	// In the future, if we implement anonymous object cycle detection,
	// these could report actual statistics
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
