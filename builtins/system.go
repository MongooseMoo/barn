package builtins

import (
	"barn/types"
	"os"
)

// ============================================================================
// SYSTEM BUILTINS
// ============================================================================

// builtinGetenv implements getenv(name)
// Returns environment variable value or 0 if not found
// Requires wizard permissions
func builtinGetenv(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	name, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := os.Getenv(name.Value())
	if value == "" {
		// Check if the variable exists but is empty vs doesn't exist
		_, exists := os.LookupEnv(name.Value())
		if !exists {
			return types.Ok(types.NewInt(0))
		}
	}

	return types.Ok(types.NewStr(value))
}

// builtinTaskLocal implements task_local()
// Returns the task-local storage for the current task
// Requires wizard permissions
func builtinTaskLocal(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Return task local or empty list if not set
	if ctx.TaskLocal == nil {
		return types.Ok(types.NewEmptyList())
	}

	return types.Ok(ctx.TaskLocal)
}

// builtinSetTaskLocal implements set_task_local(value)
// Sets the task-local storage for the current task
// Requires wizard permissions
func builtinSetTaskLocal(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Set the task local storage
	ctx.TaskLocal = args[0]

	return types.Ok(types.NewInt(0))
}

// builtinTaskID implements task_id()
// Returns the current task's ID
func builtinTaskID(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	return types.Ok(types.NewInt(ctx.TaskID))
}

// builtinTicksLeft implements ticks_left()
// Returns the number of ticks remaining for the current task
func builtinTicksLeft(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	return types.Ok(types.NewInt(ctx.TicksRemaining))
}
