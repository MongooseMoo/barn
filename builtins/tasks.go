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
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	filterPlayer := types.ObjID(0)
	if len(args) >= 1 {
		target, ok := parseConnectionTarget(args[0])
		if !ok {
			return types.Err(types.E_TYPE)
		}
		filterPlayer = target
	}

	countMode := false
	if len(args) == 2 {
		mode, ok := args[1].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		countMode = mode.Val != 0
	}

	mgr := task.GetManager()
	tasks := mgr.GetQueuedTasks()

	result := make([]types.Value, 0, len(tasks))
	for _, t := range tasks {
		if filterPlayer > 0 && t.Owner != filterPlayer {
			continue
		}
		result = append(result, t.ToQueuedTaskInfo())
	}

	if countMode {
		return types.Ok(types.NewInt(int64(len(result))))
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

	// Special case: killing yourself returns E_INTRPT
	if ctx.TaskID == taskID {
		return types.Err(types.E_INTRPT)
	}

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
		if seconds < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	// Suspend the task
	mgr := task.GetManager()
	mgr.SuspendTask(t, seconds)

	// Return FlowSuspend so scheduler knows to pause execution
	// The task will be resumed later via resume() builtin
	return types.Suspend(seconds)
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

	if !ctx.IsWizard && whoVal.ID() != ctx.Player {
		return types.Err(types.E_PERM)
	}

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
		// No task - return programmer from context (top-level eval)
		return types.Ok(types.NewObj(ctx.Programmer))
	}

	t, ok := ctx.Task.(*task.Task)
	if !ok {
		return types.Ok(types.NewObj(ctx.Programmer))
	}

	// Get the call stack
	stack := t.GetCallStack()

	// If less than 2 frames, return the task's programmer (top-level eval)
	if len(stack) < 2 {
		return types.Ok(types.NewObj(t.Programmer))
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

	// Default: do NOT include line numbers (5-element frames)
	// Pass true/1 to include line numbers (6-element frames)
	includeLineNumbers := false
	if len(args) == 1 {
		val, ok := args[0].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		includeLineNumbers = val.Val != 0
	}

	// Get the task from context
	if ctx.Task == nil {
		if ctx.Verb == "" {
			return types.Ok(syntheticEvalCallers(ctx, includeLineNumbers))
		}
		return types.Ok(types.NewList([]types.Value{}))
	}

	t, ok := ctx.Task.(*task.Task)
	if !ok {
		if ctx.Verb == "" {
			return types.Ok(syntheticEvalCallers(ctx, includeLineNumbers))
		}
		return types.Ok(types.NewList([]types.Value{}))
	}

	// Get the call stack
	stack := t.GetCallStack()

	// callers() returns the call stack EXCLUDING the current frame
	// The current frame is the top of the stack (last element)
	// So we skip the last frame and return all others (except server-initiated)
	result := make([]types.Value, 0, len(stack))
	for i, frame := range stack {
		// Skip the last frame (current frame - the one calling callers())
		if i == len(stack)-1 {
			continue
		}

		// Skip server-initiated frames (do_login_command, user_connected, etc.)
		if frame.ServerInitiated {
			continue
		}

		if includeLineNumbers {
			result = append(result, frame.ToList())
		} else {
			// Omit line number (last element)
			// ListValue.Get() is 1-based, so we get elements 1 through Len()-1
			frameList := frame.ToList().(types.ListValue)
			truncated := make([]types.Value, frameList.Len()-1)
			for i := 0; i < frameList.Len()-1; i++ {
				truncated[i] = frameList.Get(i + 1) // 1-based indexing
			}
			result = append(result, types.NewList(truncated))
		}
	}

	// Top-level eval compatibility: return two eval wrapper frames.
	if len(result) == 0 && ctx.Verb == "" {
		return types.Ok(syntheticEvalCallers(ctx, includeLineNumbers))
	}

	return types.Ok(types.NewList(result))
}

func syntheticEvalCallers(ctx *types.TaskContext, includeLineNumbers bool) types.Value {
	makeFrame := func() types.Value {
		base := []types.Value{
			types.NewObj(types.ObjNothing), // this
			types.NewStr("eval"),           // verb
			types.NewObj(ctx.Programmer),   // programmer
			types.NewObj(types.ObjNothing), // verb_loc
			types.NewObj(ctx.Player),       // player
		}
		if includeLineNumbers {
			base = append(base, types.NewInt(1))
		}
		return types.NewList(base)
	}
	return types.NewList([]types.Value{makeFrame(), makeFrame()})
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

	message := errVal.Code().Message()
	if len(args) >= 2 {
		msgVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		message = msgVal.Value()
	}

	exceptionValue := types.Value(types.NewInt(0))
	if len(args) == 3 {
		exceptionValue = args[2]
	}

	exceptionList := types.NewList([]types.Value{
		types.NewErr(errVal.Code()),
		types.NewStr(message),
		exceptionValue,
	})

	return types.Result{
		Flow:  types.FlowException,
		Error: errVal.Code(),
		Val:   exceptionList,
	}
}

// builtinTaskStack: task_stack(task_id [, include_line_numbers]) → LIST
// Returns the call stack for a suspended task
// Each frame is a map with keys: this, verb, programmer, verb_loc, player, line_number
func builtinTaskStack(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	taskIDVal, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Second arg (include_line_numbers) is optional, defaults to true
	includeLineNumbers := true
	if len(args) == 2 {
		includeVal, ok := args[1].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		includeLineNumbers = includeVal.Val != 0
	}

	taskID := taskIDVal.Val

	// Get the task from manager
	mgr := task.GetManager()
	t := mgr.GetTask(taskID)
	if t == nil {
		return types.Err(types.E_INVARG)
	}

	// Permission check: must be task owner or wizard
	if t.Owner != ctx.Programmer && !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Get call stack
	callStack := t.GetCallStack()

	// Convert to list of maps
	result := make([]types.Value, 0, len(callStack))
	for _, frame := range callStack {
		if includeLineNumbers {
			result = append(result, frame.ToMap())
		} else {
			// Omit line_number from map
			// Note: 'this' is always an object ID (#-1 for primitives, matching Toast)
			frameMap := types.NewMap([][2]types.Value{
				{types.NewStr("this"), types.NewObj(frame.This)},
				{types.NewStr("verb"), types.NewStr(frame.Verb)},
				{types.NewStr("programmer"), types.NewObj(frame.Programmer)},
				{types.NewStr("verb_loc"), types.NewObj(frame.VerbLoc)},
				{types.NewStr("player"), types.NewObj(frame.Player)},
			})
			result = append(result, frameMap)
		}
	}

	return types.Ok(types.NewList(result))
}

// builtinYin: yin([threshold [, ticks [, seconds]]]) → none
// Yields execution if resources are low.
// Currently implemented as a no-op since we don't have tick-based execution.
func builtinYin(ctx *types.TaskContext, args []types.Value) types.Result {
	// Args: optional threshold, ticks, seconds - all INT
	// For now, just accept 0-3 arguments and return 0 (no-op)
	if len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	// Validate args are INTs if provided
	for _, arg := range args {
		if _, ok := arg.(types.IntValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}

	// No-op - just return 0
	return types.Ok(types.NewInt(0))
}
