package builtins

import (
	"runtime"

	"github.com/MongooseMoo/barn/types"
)

// ============================================================================
// GARBAGE COLLECTION BUILTINS
// ============================================================================

// builtinRunGC implements run_gc()
// Triggers garbage collection (wizard only)
// Returns 0 on success
func builtinRunGC(ctx *Execution, args []types.Value) types.Result {
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

	if runGC := hostOf(ctx).RunGC; runGC != nil {
		aborted := false
		err := runGC(ctx, func() bool {
			if !beginIrreversible(ctx) {
				aborted = true
				return false
			}
			ctx.IrreversibleSideEffect = true
			return true
		})
		if aborted {
			return abortedAttempt()
		}
		if err != nil {
			return types.Err(types.E_INVARG)
		}
	}

	return types.Ok(types.NewInt(0))
}

// builtinGCStats implements gc_stats()
// Returns GC statistics map (wizard only)
// Returns map with color keys: green, yellow, black, gray, white, purple, pink
func builtinGCStats(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	measure := hostOf(ctx).AnonymousGCStats
	if measure == nil {
		return types.Err(types.E_QUOTA)
	}
	stats := measure()
	result := types.NewEmptyMap()
	result = result.MapSet(types.NewStr("green"), types.NewInt(stats.Green))
	result = result.MapSet(types.NewStr("yellow"), types.NewInt(stats.Yellow))
	result = result.MapSet(types.NewStr("black"), types.NewInt(stats.Black))
	result = result.MapSet(types.NewStr("gray"), types.NewInt(stats.Gray))
	result = result.MapSet(types.NewStr("white"), types.NewInt(stats.White))
	result = result.MapSet(types.NewStr("purple"), types.NewInt(stats.Purple))
	result = result.MapSet(types.NewStr("pink"), types.NewInt(stats.Pink))

	return types.Ok(result)
}
