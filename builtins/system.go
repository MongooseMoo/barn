package builtins

import (
	"barn/db"
	"barn/task"
	"barn/types"
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
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
		return types.Ok(types.NewEmptyMap())
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

// builtinExec implements exec(command [, input]) → LIST
// Executes external command and returns {exit_code, stdout, stderr}
// Requires wizard permissions
func builtinExec(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Parse command
	var program string
	var cmdArgs []string

	switch cmd := args[0].(type) {
	case types.ListValue:
		// List form: {"program", "arg1", "arg2"}
		if cmd.Len() == 0 {
			return types.Err(types.E_INVARG)
		}
		progVal, ok := cmd.Get(1).(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		program = progVal.Value()
		cmdArgs = make([]string, cmd.Len()-1)
		for i := 2; i <= cmd.Len(); i++ {
			argVal, ok := cmd.Get(i).(types.StrValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			cmdArgs[i-2] = argVal.Value()
		}
	case types.StrValue:
		// String form: "command with args" - use shell
		program = "sh"
		cmdArgs = []string{"-c", cmd.Value()}
	default:
		return types.Err(types.E_TYPE)
	}

	// Get input if provided
	var input string
	if len(args) == 2 {
		inputVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		input = inputVal.Value()
	}

	// Execute command
	result := execCommand(program, cmdArgs, input)
	return result
}

// execCommand runs a command and returns {exit_code, stdout, stderr}
func execCommand(program string, args []string, input string) types.Result {
	cmd := exec.Command(program, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewBufferString(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Recreate command with context
	cmd = exec.CommandContext(ctx, program, args...)
	cmd.Stdin = bytes.NewBufferString(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			// Timeout - return E_EXEC
			return types.Err(types.E_EXEC)
		} else {
			// Command not found or other error - return E_INVARG per spec
			return types.Err(types.E_INVARG)
		}
	}

	// Return {exit_code, stdout, stderr}
	result := []types.Value{
		types.NewInt(int64(exitCode)),
		types.NewStr(stdout.String()),
		types.NewStr(stderr.String()),
	}
	return types.Ok(types.NewList(result))
}

// builtinTime implements time()
// Returns the current time as a Unix timestamp (seconds since epoch)
func builtinTime(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(time.Now().Unix()))
}

// builtinCtime implements ctime([time])
// Converts a Unix timestamp to a human-readable string
func builtinCtime(ctx *types.TaskContext, args []types.Value) types.Result {
	var timestamp int64
	if len(args) == 0 {
		timestamp = time.Now().Unix()
	} else if len(args) == 1 {
		switch v := args[0].(type) {
		case types.IntValue:
			timestamp = v.Val
		case types.FloatValue:
			timestamp = int64(v.Val)
		default:
			return types.Err(types.E_TYPE)
		}
	} else {
		return types.Err(types.E_ARGS)
	}
	t := time.Unix(timestamp, 0)
	// MOO format: "Sun Dec 26 22:30:00 2025" (24 chars, no timezone)
	// Go's _2 gives space-padded day: " 1" for day 1, "28" for day 28
	return types.Ok(types.NewStr(t.Format("Mon Jan _2 15:04:05 2006")))
}

// builtinServerVersion implements server_version([key])
// Returns server version information
// With no args: returns version string like "1.0.0"
// With arg: returns specific version info (not fully implemented yet)
func builtinServerVersion(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 {
		return types.Ok(types.NewStr("1.0.0-barn"))
	}
	// For now, just return version string for any argument
	return types.Ok(types.NewStr("1.0.0-barn"))
}

// builtinServerLog implements server_log(message)
// Logs a message to the server log. Requires wizard permissions.
func builtinServerLog(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Convert all args to strings and concatenate
	var msg string
	for _, arg := range args {
		switch v := arg.(type) {
		case types.StrValue:
			msg += v.Value()
		default:
			msg += arg.String()
		}
	}

	// Log to server output
	// TODO: Use a proper logging system
	println("[SERVER_LOG]", msg)

	return types.Ok(types.NewInt(0))
}

// builtinLoadServerOptions implements load_server_options()
// Reloads server configuration from $server_options object.
// Reads properties like max_string_concat and caches them globally.
// Requires wizard permissions.
func builtinLoadServerOptions(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Load server options from $server_options object into global cache
	loaded := LoadServerOptionsFromStore(store)

	return types.Ok(types.NewInt(int64(loaded)))
}
