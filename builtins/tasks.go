package builtins

import (
	"sort"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

type TaskYielder interface {
	YieldReadyTasks() int
}

// Task management builtins - full implementation

// builtinQueuedTasks: queued_tasks() → LIST
// Returns list of currently queued tasks
// Each entry: {task_id, start_time, x, y, z, programmer, verb_loc, verb_name, line, this}
func builtinQueuedTasks(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	includeVariables := false
	if len(args) >= 1 {
		if args[0].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		includeVariables = args[0].Truthy()
	}

	countMode := false
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		countMode = args[1].Truthy()
	}

	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	tasks := mgr.GetQueuedTasks()
	type queuedTaskSnapshot struct {
		startTime time.Time
		queueSeq  int64
		id        int64
		info      types.Value
	}
	snapshots := make([]queuedTaskSnapshot, 0, len(tasks))
	for _, queuedTask := range tasks {
		startTime, queueSeq, id, info := queuedTask.QueuedTaskSnapshot(includeVariables)
		snapshots = append(snapshots, queuedTaskSnapshot{startTime, queueSeq, id, info})
	}
	// Toast returns waiting tasks in ascending start-time order (earliest
	// first). It never sorts in bf_queued_tasks (tasks.cc:2571-2581); the
	// ordering comes from waiting_tasks, which enqueue_waiting keeps sorted
	// ascending by start_tv (tasks.cc:1193-1204). Match that here.
	sort.SliceStable(snapshots, func(i, j int) bool {
		if !snapshots[i].startTime.Equal(snapshots[j].startTime) {
			return snapshots[i].startTime.Before(snapshots[j].startTime)
		}
		if snapshots[i].queueSeq != snapshots[j].queueSeq {
			return snapshots[i].queueSeq < snapshots[j].queueSeq
		}
		return snapshots[i].id < snapshots[j].id
	})

	result := make([]types.Value, 0, len(snapshots))
	for _, snapshot := range snapshots {
		info := snapshot.info
		if !ctx.IsWizard && info.Get(5).Obj() != ctx.Programmer {
			continue
		}
		thisValue := info.Get(9)
		if thisValue.Type() == types.TYPE_ANON && !ctx.IsWizard {
			owner, errCode := objectOwnerForRead(ctx, thisValue.ID())
			if errCode != types.E_NONE || owner != ctx.Programmer {
				continue
			}
		}
		result = append(result, info)
	}

	if countMode {
		return types.Ok(types.NewInt(int64(len(result))))
	}

	return types.Ok(types.NewList(result))
}

// builtinKillTask: kill_task(task_id) → none
// Kills the specified task
// Requires permission: must be task owner or wizard
func builtinKillTask(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	taskID := args[0].Int()

	// Special case: killing yourself returns E_INTRPT
	if ctx.TaskID == taskID {
		return types.Err(types.E_INTRPT)
	}

	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}

	errCode := mgr.KillTask(taskID, ctx.Programmer, ctx.IsWizard)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	ctx.Session.CancelHTTPReadTask(taskID)

	return types.Ok(types.NewInt(0))
}

// builtinSuspend: suspend([seconds]) → value
// Suspends the current task for the specified duration.
// Returns the value passed to resume() when the task is resumed.
// If no seconds are specified, suspension is indefinite until resume().
// suspend(0) yields and resumes on the next scheduler cycle.
func builtinSuspend(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	// Get the task from context
	if ctx.Task == nil {
		// No task to suspend - this shouldn't happen in normal execution
		return types.Err(types.E_INVARG)
	}

	t := ctx.Task
	if t == nil {
		return types.Err(types.E_INVARG)
	}

	// Parse seconds argument.
	// -1 is our internal sentinel for indefinite suspension.
	seconds := -1.0
	if len(args) == 1 {
		switch args[0].Type() {
		case types.TYPE_INT:
			seconds = float64(args[0].Int())
		case types.TYPE_FLOAT:
			seconds = args[0].Float()
		default:
			return types.Err(types.E_TYPE)
		}
		if seconds < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	// Suspend the task
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, seconds)

	// Return FlowSuspend so the engine knows to pause execution.
	// The task will be resumed later via resume() builtin
	return types.Suspend(seconds)
}

// builtinResume: resume(task_id [, value]) → none
// Resumes a suspended task with the given value
// The value (or 0 if not specified) is returned from suspend()
// Requires permission: must be task owner or wizard
func builtinResume(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	taskID := args[0].Int()

	// Get the value to pass to the resumed task
	var value types.Value = types.NewInt(0)
	if len(args) == 2 {
		value = args[1]
	}

	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	errCode := mgr.ResumeTask(taskID, value, ctx.Programmer, ctx.IsWizard)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetTaskPerms: set_task_perms(who) → none
// Changes the permission context for the current task
// Wizard only - allows running code with different permissions
func builtinSetTaskPerms(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Get the new permission object
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}

	// Toast compares the target against the currently running verb's programmer
	// (RUN_ACTIV.progr), not the connected player: set_task_perms(who) is allowed
	// when the current programmer is a wizard or who is the current programmer.
	progIsWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	if errCode != types.E_NONE {
		progIsWizard = false
	}
	if !progIsWizard && args[0].ID() != ctx.Programmer {
		return types.Err(types.E_PERM)
	}

	ctx.Programmer = args[0].ID()

	// Update ctx.IsWizard to reflect the new programmer's actual status.
	// In Toast, the progr field determines wizard checks dynamically;
	// Barn caches IsWizard so we must update it here.
	ctx.IsWizard, errCode = hasObjectFlagForRead(ctx, args[0].ID(), dbstore.FlagWizard)
	if errCode != types.E_NONE {
		ctx.IsWizard = false
	}

	// Also update the current CallStack frame's Programmer so that
	// caller_perms() reflects the new permissions (matches Toast's
	// behavior where set_task_perms updates RUN_ACTIV.progr).
	if t := ctx.Task; t != nil {
		t.SetTopFrameProgrammer(args[0].ID())
	}

	return types.Ok(types.NewInt(0))
}

// builtinCallerPerms: caller_perms() → OBJ
// Returns the programmer of the calling frame (not the current frame)
// This is used for permission checks - returns who called this verb
func builtinCallerPerms(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Get the task from context
	if ctx.Task == nil {
		// No task - return programmer from context (top-level eval)
		return types.Ok(types.NewObj(ctx.Programmer))
	}

	t := ctx.Task
	if t == nil {
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
func builtinCallers(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	// Default: do NOT include line numbers (5-element frames)
	// Pass true/1 to include line numbers (6-element frames)
	includeLineNumbers := false
	if len(args) == 1 {
		includeLineNumbers = args[0].Truthy()
	}

	// Get the task from context
	if ctx.Task == nil {
		if ctx.Verb == "" {
			return types.Ok(syntheticEvalCallers(ctx, includeLineNumbers))
		}
		return types.Ok(types.NewList([]types.Value{}))
	}

	t := ctx.Task
	if t == nil {
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
		if frame.ThisValue.Type() == types.TYPE_ANON && !ctx.IsWizard {
			owner, errCode := objectOwnerForRead(ctx, frame.ThisValue.ID())
			if errCode != types.E_NONE || owner != ctx.Programmer {
				frame.ThisValue = types.NewAnon(types.ObjNothing)
			}
		}

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
			frameList := frame.ToList()
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
func evalWrapperFrames(ctx *Execution, includeLineNumbers bool) []types.Value {
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
func evalUserCodeFrame(ctx *Execution, includeLineNumbers bool) types.Value {
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

func syntheticEvalCallers(ctx *Execution, includeLineNumbers bool) types.Value {
	return types.NewList(evalWrapperFrames(ctx, includeLineNumbers))
}

// builtinRaise: raise(error [, message [, value]]) → none
// Raises an error, stopping execution until caught by try/except
func builtinRaise(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	code := types.E_NONE
	message := args[0].String()
	if args[0].Type() == types.TYPE_ERR {
		code = args[0].Code()
		message = code.Message()
	}
	if len(args) >= 2 {
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		message = args[1].Str()
	}

	exceptionValue := types.Value(types.NewInt(0))
	if len(args) == 3 {
		exceptionValue = args[2]
	}

	exceptionList := types.NewList([]types.Value{
		args[0],
		types.NewStr(message),
		exceptionValue,
	})

	return types.Result{
		Flow:  types.FlowException,
		Error: code,
		Val:   exceptionList,
	}
}

// builtinTaskStack: task_stack(task_id [, include_line_numbers [, include_vars]]) → LIST
// Returns the call stack for a suspended task
// Each frame is a map with keys: this, verb, programmer, verb_loc, player, line_number
func builtinTaskStack(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	taskID := args[0].Int()

	// task_stack on the currently running task is invalid (task must be suspended)
	if taskID == ctx.TaskID {
		return types.Err(types.E_INVARG)
	}

	// Get the task from manager
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	t := mgr.GetTask(taskID)
	if t == nil {
		return types.Err(types.E_INVARG)
	}

	// Permission check: must be task owner or wizard
	if t.Owner != ctx.Programmer && !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Optional args are TYPE_ANY in Toast. The second flag includes line
	// numbers; the third includes each frame's bound runtime variables.
	includeLineNumbers := false
	if len(args) >= 2 {
		includeLineNumbers = args[1].Truthy()
	}
	includeVariables := false
	if len(args) >= 3 {
		includeVariables = args[2].Truthy()
	}

	// Get call stack
	callStack := t.GetCallStack()

	// Convert to list of lists: {this, verb_name, programmer, verb_loc, player [, line]}
	// Order: innermost frame first (most recently called verb first), matching Toast.
	result := make([]types.Value, 0, len(callStack))
	for i := len(callStack) - 1; i >= 0; i-- {
		frame := callStack[i]
		frameList := frame.ToList()
		values := make([]types.Value, 0, 7)
		if includeLineNumbers {
			values = append(values, frameList.Elements()...)
		} else {
			// Omit line number (6th element) → 5-element list
			for j := 0; j < frameList.Len()-1; j++ {
				values = append(values, frameList.Get(j+1))
			}
		}
		if includeVariables {
			runtimeVariables := frame.RuntimeVariables
			if runtimeVariables.Type() != types.TYPE_MAP {
				runtimeVariables = types.NewEmptyMap()
			}
			values = append(values, runtimeVariables)
		}
		result = append(result, types.NewList(values))
	}

	return types.Ok(types.NewList(result))
}

// builtinYin: yin([seconds [, min_ticks [, min_seconds]]]) → 0
// "Yield if needed": suspends for `seconds` (default 0) when the task is
// running low on ticks (ticks_left() < min_ticks, default 2000) or on time
// (seconds_left() < min_seconds, default 2); otherwise returns 0 immediately.
// Mirrors ToastStunt bf_yield_if_needed (execute.cc), including validating
// min_ticks/min_seconds against the fg_ticks/fg_seconds limits only when at
// least one argument is supplied.
func builtinYin(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	seconds := 0.0
	if len(args) >= 1 {
		switch args[0].Type() {
		case types.TYPE_INT:
			seconds = float64(args[0].Int())
		case types.TYPE_FLOAT:
			seconds = args[0].Float()
		default:
			return types.Err(types.E_TYPE)
		}
	}

	minTicks := int64(2000)
	if len(args) >= 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		minTicks = args[1].Int()
	}

	minSeconds := int64(2)
	if len(args) >= 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		minSeconds = args[2].Int()
	}

	if len(args) >= 1 {
		fgTicks, fgSeconds := ctx.Session.GetTaskLimits(false)
		if seconds < 0 || minTicks <= 0 || minSeconds <= 0 ||
			minTicks >= fgTicks || float64(minSeconds) >= fgSeconds {
			return types.Err(types.E_INVARG)
		}
	}

	t := ctx.Task
	if t == nil {
		return types.Ok(types.NewInt(0))
	}

	if ctx.TicksRemaining >= minTicks && int64(t.SecondsLeft()) >= minSeconds {
		return types.Ok(types.NewInt(0))
	}

	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, seconds)
	return types.Suspend(seconds)
}

func builtinTaskPerms(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewObj(ctx.Programmer))
}

func builtinQueueInfo(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		// queue_info() with no argument is allowed for non-wizards (Toast).
		// The connected player is payload here, not an authority identity.
		players := []types.ObjID{}
		seen := map[types.ObjID]struct{}{}
		if ctx.Player > 0 {
			seen[ctx.Player] = struct{}{}
			players = append(players, ctx.Player)
		}
		if cm := hostOf(ctx).ConnManager; cm != nil {
			for _, p := range cm.ConnectedPlayers(false) {
				if _, ok := seen[p]; ok {
					continue
				}
				seen[p] = struct{}{}
				players = append(players, p)
			}
		}
		out := make([]types.Value, 0, len(players))
		for _, p := range players {
			out = append(out, types.NewObj(p))
		}
		return types.Ok(types.NewList(out))
	}

	target, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}

	if !ctx.IsWizard {
		if target != ctx.Programmer {
			return types.Err(types.E_PERM)
		}
		return types.Ok(types.NewInt(countBackgroundTasksFor(mgr, target)))
	}

	connected := 0
	if resolveConnection(ctx, target) != nil {
		connected = 1
	} else if target != ctx.Player {
		// Toast behavior for wizard querying non-connected/nonexistent player.
		// This is connection-state handling, not a permission decision.
		return types.Ok(types.NewInt(0))
	}

	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("player"), types.NewObj(target)},
		{types.NewStr("connected"), types.NewInt(int64(connected))},
		{types.NewStr("num_bg_tasks"), types.NewInt(countBackgroundTasksFor(mgr, target))},
	}))
}

func countBackgroundTasksFor(tasks TaskLister, player types.ObjID) int64 {
	count := int64(0)
	for _, t := range tasks.GetQueuedTasks() {
		if t.Owner == player {
			count++
		}
	}
	return count
}

func builtinFinishedTasks(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	all := mgr.GetAllTasks()
	result := make([]types.Value, 0)
	for _, t := range all {
		st := t.GetState()
		if st == task.TaskCompleted || st == task.TaskKilled {
			result = append(result, types.NewInt(t.ID))
		}
	}
	return types.Ok(types.NewList(result))
}

func builtinThreads(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	all := mgr.GetAllTasks()
	result := make([]types.Value, 0, len(all))
	for _, t := range all {
		if t.GetState() == task.TaskRunning || t.GetState() == task.TaskSuspended || t.GetState() == task.TaskQueued {
			result = append(result, types.NewInt(t.ID))
		}
	}
	return types.Ok(types.NewList(result))
}

func builtinThreadPool(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if args[0].Str() != "INIT" || args[1].Str() != "MAIN" {
		return types.Err(types.E_INVARG)
	}
	if len(args) == 3 && args[2].Int() < 0 {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(1))
}

func builtinSetThreadMode(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 1 {
		if args[0].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		ctx.ThreadMode = args[0].Truthy()
		return types.Ok(types.NewInt(0))
	}
	if ctx.ThreadMode {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}
