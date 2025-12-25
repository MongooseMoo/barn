package builtins

import (
	"barn/task"
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

	// Get task-local from task if available
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			return types.Ok(t.GetTaskLocal())
		}
	}

	// Fallback to context for backward compatibility
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

	// Set task-local in task if available
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			t.SetTaskLocal(args[0])
			return types.Ok(types.NewInt(0))
		}
	}

	// Fallback to context for backward compatibility
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

	// Get from task if available (more accurate)
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			return types.Ok(types.NewInt(t.TicksLeft()))
		}
	}

	// Fallback to context
	return types.Ok(types.NewInt(ctx.TicksRemaining))
}

// builtinSecondsLeft implements seconds_left()
// Returns the number of seconds remaining for the current task
func builtinSecondsLeft(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Get from task if available
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			return types.Ok(types.NewFloat(t.SecondsLeft()))
		}
	}

	// Default fallback (assume infinite time if no task)
	return types.Ok(types.NewFloat(1000.0))
}
