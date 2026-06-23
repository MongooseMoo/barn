package builtins

import (
	"sort"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

type TaskYielder interface {
	YieldReadyTasks() int
}

var globalTaskYielder TaskYielder

func SetTaskYielder(yielder TaskYielder) {
	globalTaskYielder = yielder
}

// Task management builtins - full implementation

// builtinQueuedTasks: queued_tasks() → LIST
// Returns list of currently queued tasks
// Each entry: {task_id, start_time, x, y, z, programmer, verb_loc, verb_name, line, this}
func builtinQueuedTasks(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].StartTime.After(tasks[j].StartTime)
	})

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
func builtinKillTask(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	CancelHTTPReadTask(taskID)

	return types.Ok(types.NewInt(0))
}

// builtinSuspend: suspend([seconds]) → value
// Suspends the current task for the specified duration.
// Returns the value passed to resume() when the task is resumed.
// If no seconds are specified, suspension is indefinite until resume().
// suspend(0) yields and resumes on the next scheduler cycle.
func builtinSuspend(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	// Parse seconds argument.
	// -1 is our internal sentinel for indefinite suspension.
	seconds := -1.0
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
func builtinResume(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinSetTaskPerms(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Get the new permission object
	whoVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Toast compares the target against the currently running verb's programmer
	// (RUN_ACTIV.progr), not the connected player: set_task_perms(who) is allowed
	// when the current programmer is a wizard or who is the current programmer.
	progIsWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	if errCode != types.E_NONE {
		progIsWizard = false
	}
	if !progIsWizard && whoVal.ID() != ctx.Programmer {
		return types.Err(types.E_PERM)
	}

	ctx.Programmer = whoVal.ID()

	// Update ctx.IsWizard to reflect the new programmer's actual status.
	// In Toast, the progr field determines wizard checks dynamically;
	// Barn caches IsWizard so we must update it here.
	ctx.IsWizard, errCode = hasObjectFlagForRead(ctx, whoVal.ID(), dbstore.FlagWizard)
	if errCode != types.E_NONE {
		ctx.IsWizard = false
	}

	// Also update the current CallStack frame's Programmer so that
	// caller_perms() reflects the new permissions (matches Toast's
	// behavior where set_task_perms updates RUN_ACTIV.progr).
	if t, ok := ctx.Task.(*task.Task); ok {
		if top := t.GetTopFrame(); top != nil {
			top.Programmer = whoVal.ID()
		}
	}

	return types.Ok(types.NewInt(0))
}

// builtinCallerPerms: caller_perms() → OBJ
// Returns the programmer of the calling frame (not the current frame)
// This is used for permission checks - returns who called this verb
func builtinCallerPerms(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	// If less than 2 frames, there is no MOO caller. A parser-dispatched
	// top-level command verb has no caller permissions object (Toast returns
	// #-1), whereas a top-level eval inherits the player's permissions.
	if len(stack) < 2 {
		if t.FromCommand {
			return types.Ok(types.NewObj(types.ObjNothing))
		}
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
func builtinCallers(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	// callers() returns the call stack EXCLUDING the current frame,
	// ordered most-recent-first (immediate caller first).
	// The current frame is the top of the stack (last element).
	//
	// If the current (top) frame is an eval frame, we're at the eval top level.
	// Return synthetic eval wrapper frames (matching Toast behavior).
	if len(stack) > 0 && stack[len(stack)-1].IsEvalFrame {
		return types.Ok(syntheticEvalCallers(ctx, includeLineNumbers))
	}

	result := make([]types.Value, 0, len(stack))
	for i := len(stack) - 2; i >= 0; i-- {
		frame := stack[i]

		// At the eval boundary, expose the eval activation the way Toast does:
		// the eval'd user-code frame followed by the two synthetic eval wrapper
		// frames (bf_eval builtin marker + root "eval" command frame).
		if frame.IsEvalFrame {
			result = append(result, evalUserCodeFrame(ctx, includeLineNumbers))
			result = append(result, evalWrapperFrames(ctx, includeLineNumbers)...)
			break
		}

		if includeLineNumbers {
			result = append(result, frame.ToList())
		} else {
			// Omit line number (last element)
			frameList := frame.ToList().(types.ListValue)
			truncated := make([]types.Value, frameList.Len()-1)
			for j := 0; j < frameList.Len()-1; j++ {
				truncated[j] = frameList.Get(j + 1) // 1-based indexing
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

// evalWrapperFrames returns the two activation frames Toast reports beneath the
// eval'd user code for a command-line ";" eval: a synthetic bf_eval builtin
// marker ({#-1, "eval", #-1, #-1, player}) and the root "eval" command frame,
// whose `this`/verb_loc is the player's location and whose programmer is the
// player ({location, "eval", player, location, player}).
func evalWrapperFrames(ctx *kernel.TaskContext, includeLineNumbers bool) []types.Value {
	location := types.ObjNothing
	if ctx.Store != nil {
		loc, errCode := locationForRead(ctx, ctx.Player)
		if errCode == types.E_NONE {
			location = loc
		}
	}
	makeFrame := func(this, programmer, vloc types.ObjID) types.Value {
		base := []types.Value{
			types.NewObj(this),       // this
			types.NewStr("eval"),     // verb
			types.NewObj(programmer), // programmer
			types.NewObj(vloc),       // verb_loc
			types.NewObj(ctx.Player), // player
		}
		if includeLineNumbers {
			base = append(base, types.NewInt(1))
		}
		return types.NewList(base)
	}
	return []types.Value{
		makeFrame(types.ObjNothing, types.ObjNothing, types.ObjNothing), // bf_eval builtin marker
		makeFrame(location, ctx.Player, location),                       // root eval command frame
	}
}

// evalUserCodeFrame returns the representation of the eval'd user-code activation
// itself ({#-1, "", player, #-1, player}) — Toast shows this when callers() is
// invoked from a verb that was called by the eval.
func evalUserCodeFrame(ctx *kernel.TaskContext, includeLineNumbers bool) types.Value {
	base := []types.Value{
		types.NewObj(types.ObjNothing), // this
		types.NewStr(""),               // verb (empty for eval'd code)
		types.NewObj(ctx.Player),       // programmer (the eval runs as the player)
		types.NewObj(types.ObjNothing), // verb_loc
		types.NewObj(ctx.Player),       // player
	}
	if includeLineNumbers {
		base = append(base, types.NewInt(1))
	}
	return types.NewList(base)
}

func syntheticEvalCallers(ctx *kernel.TaskContext, includeLineNumbers bool) types.Value {
	return types.NewList(evalWrapperFrames(ctx, includeLineNumbers))
}

// builtinRaise: raise(error [, message [, value]]) → none
// Raises an error, stopping execution until caught by try/except
func builtinRaise(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinTaskStack(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	taskIDVal, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Second arg (include_line_numbers) is optional, defaults to false
	includeLineNumbers := false
	if len(args) == 2 {
		includeVal, ok := args[1].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		includeLineNumbers = includeVal.Val != 0
	}

	taskID := taskIDVal.Val

	// task_stack on the currently running task is invalid (task must be suspended)
	if taskID == ctx.TaskID {
		return types.Err(types.E_INVARG)
	}

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

	// Convert to list of lists: {this, verb_name, programmer, verb_loc, player [, line]}
	// Order: innermost frame first (most recently called verb first), matching Toast.
	result := make([]types.Value, 0, len(callStack))
	for i := len(callStack) - 1; i >= 0; i-- {
		frame := callStack[i]
		if includeLineNumbers {
			result = append(result, frame.ToList())
		} else {
			// Omit line number (6th element) → 5-element list
			frameList := frame.ToList().(types.ListValue)
			truncated := make([]types.Value, frameList.Len()-1)
			for j := 0; j < frameList.Len()-1; j++ {
				truncated[j] = frameList.Get(j + 1)
			}
			result = append(result, types.NewList(truncated))
		}
	}

	return types.Ok(types.NewList(result))
}

// builtinYin: yin([threshold [, ticks [, seconds]]]) → none
// Yields execution if requested resource thresholds have been crossed.
func builtinYin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	for _, arg := range args {
		if _, ok := arg.(types.IntValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}

	if len(args) >= 2 && globalTaskYielder != nil {
		tickThreshold := args[1].(types.IntValue).Val
		if ctx.TicksRemaining <= tickThreshold {
			if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
				if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
					return types.Err(errCode)
				}
				if t, ok := ctx.Task.(*task.Task); ok {
					t.CreatedForks = nil
				}
				if errCode := FlushPendingServerOptions(ctx); errCode != types.E_NONE {
					return types.Err(errCode)
				}
				if errCode := FlushPendingConnectionSwitches(ctx); errCode != types.E_NONE {
					return types.Err(errCode)
				}
				if errCode := FlushPendingNotifications(ctx); errCode != types.E_NONE {
					return types.Err(errCode)
				}
				if errCode := FlushPendingBootPlayers(ctx); errCode != types.E_NONE {
					return types.Err(errCode)
				}
			}
			globalTaskYielder.YieldReadyTasks()
			if ctx.Store != nil {
				ctx.StoreTxn = ctx.Store.BeginReadOnly(0)
			}
		}
	}

	return types.Ok(types.NewInt(0))
}
