package scheduler

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"barn/builtins"
	"barn/compiler"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/metrics"
	"barn/task"
	"barn/trace"
	"barn/types"
	"barn/vm"
)

// CallVerb synchronously executes a verb on an object and returns the result
// This is used for server hooks like do_login_command, user_connected, etc.
// Returns a Result with a call stack for traceback formatting
func (s *Scheduler) CallVerb(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) (result types.Result) {
	return s.CallVerbWithArgstr(objID, verbName, args, player, "")
}

func (s *Scheduler) CallVerbInContext(objID types.ObjID, verbName string, args []types.Value, parentCtx *kernel.TaskContext) types.Result {
	if parentCtx == nil {
		return types.Err(types.E_INVARG)
	}

	var (
		verb     dbstore.VerbView
		defObjID types.ObjID
		err      error
	)
	if parentCtx.StoreTxn != nil {
		verb, defObjID, err = parentCtx.StoreTxn.FindVerb(objID, verbName)
	} else {
		verb, defObjID, err = s.store.FindVerb(objID, verbName)
	}
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	prog, diagnostics := compiler.CompileMOOWithKey(verb.Code, verb.CodeKey, s.registry)
	if len(diagnostics) > 0 {
		slog.Error("verb compile error",
			slog.String("verb", verbName),
			slog.Int64("this", int64(defObjID)),
			slog.String("error", diagnostics[0].Error()))
		return types.Err(types.E_VERBNF)
	}

	player := parentCtx.Player
	if player == types.ObjNothing {
		player = parentCtx.Programmer
	}
	caller := parentCtx.ThisObj

	thisVal := types.Value(types.NewObj(objID))
	var frameThisValue types.Value
	var isAnonymous bool
	var anonErr types.ErrorCode
	if parentCtx.StoreTxn != nil {
		isAnonymous, anonErr = parentCtx.StoreTxn.ObjectIsAnonymous(objID)
	} else {
		isAnonymous, anonErr = s.store.ObjectIsAnonymous(objID)
	}
	if anonErr == types.E_NONE && isAnonymous {
		anon := types.NewAnon(objID)
		thisVal = anon
		frameThisValue = anon
	}

	savedThisObj := parentCtx.ThisObj
	savedThisValue := parentCtx.ThisValue
	savedVerb := parentCtx.Verb
	savedProgrammer := parentCtx.Programmer
	savedIsWizard := parentCtx.IsWizard

	parentCtx.ThisObj = objID
	parentCtx.ThisValue = frameThisValue
	parentCtx.Verb = verbName
	parentCtx.Programmer = verb.Owner
	parentCtx.IsWizard = s.isWizard(verb.Owner)

	parentTask, _ := parentCtx.Task.(*task.Task)
	if parentTask != nil {
		parentTask.PushFrame(task.ActivationFrame{
			This:       objID,
			ThisValue:  frameThisValue,
			Player:     player,
			Programmer: verb.Owner,
			Caller:     caller,
			Verb:       verbName,
			VerbLoc:    defObjID,
			Args:       args,
			LineNumber: 1,
		})
	}

	bcVM := vm.NewVM(s.store, s.registry)
	bcVM.Context = parentCtx
	ticks, _ := foregroundTaskLimits()
	bcVM.TickLimit = ticks
	configureVMStackLimit(bcVM)

	frame := bcVM.PrepareVerbFrame(prog, objID, player, caller, verbName, defObjID, args)
	frame.IsVerbCall = true
	frame.VerbDebug = verb.Perms.Has(dbstore.VerbDebug)
	frame.StoredVerb = strings.Join(verb.Names, " ")
	frame.SavedThisObj = savedThisObj
	frame.SavedThisValue = savedThisValue
	frame.SavedVerb = savedVerb
	frame.SavedProgrammer = savedProgrammer
	frame.SavedIsWizard = savedIsWizard
	vm.SetLocalByName(frame, prog, "this", thisVal)
	vm.SetLocalByName(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "caller", types.NewObj(caller))
	vm.SetLocalByName(frame, prog, "verb", types.NewStr(verbName))
	vm.SetLocalByName(frame, prog, "args", types.NewList(args))
	vm.SetLocalByName(frame, prog, "argstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))

	result := bcVM.ExecuteLoop()
	if parentTask != nil {
		result = s.drainForks(parentTask, bcVM, result)
	}
	if result.Flow == types.FlowException {
		trace.Exception(objID, verbName, result.Error)
	} else {
		trace.VerbReturn(objID, verbName, result.Val)
	}
	return result
}

func (s *Scheduler) CallVerbWithArgstr(objID types.ObjID, verbName string, args []types.Value, player types.ObjID, argstr string) (result types.Result) {
	// Recover from panics in compile/execute to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			metrics.PanicsRecovered.Add(1)
			slog.Error("panic in verb call",
				slog.Int64("this", int64(objID)),
				slog.String("verb", verbName),
				slog.Int64("player", int64(player)),
				slog.String("panic", fmt.Sprint(r)),
				slog.String("go_stack", string(debug.Stack())))
			result = types.Err(types.E_NONE)
		}
	}()

	// Trace verb call
	trace.VerbCall(objID, verbName, args, player, player)

	// Create a lightweight task FIRST for call stack tracking
	// This ensures we have a stack even if verb lookup fails
	t := &task.Task{
		Owner:       player,
		Programmer:  player, // Will be updated to verb owner if verb found
		CallStack:   make([]task.ActivationFrame, 0),
		TaskLocal:   types.NewEmptyMap(), // Initialize task_local to empty map
		ForkCreator: s,                   // Enable fork support in server hooks
	}

	// Look up the verb to get its owner for programmer permissions
	verb, defObjID, err := s.store.FindVerb(objID, verbName)
	if err != nil {
		// Verb not found
		result := types.Result{
			Flow:  types.FlowException,
			Error: types.E_VERBNF,
			Val:   types.None,
		}
		// Don't log E_VERBNF for optional hooks
		return result
	}

	// Compile verb to bytecode, keyed by the store's content key.
	prog, diagnostics := compiler.CompileMOOWithKey(verb.Code, verb.CodeKey, s.registry)
	if len(diagnostics) > 0 {
		slog.Error("verb failed to compile",
			slog.String("verb", verbName),
			slog.Int64("verbloc", int64(defObjID)),
			slog.String("diagnostic", diagnostics[0].Error()))
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_VERBNF,
			Val:   types.None,
		}
	}

	// Update programmer to verb owner now that we found the verb
	t.Programmer = verb.Owner

	thisVal := types.NewObj(objID)
	frameThisValue := types.None
	if isAnonymous, errCode := s.store.ObjectIsAnonymous(objID); errCode == types.E_NONE && isAnonymous {
		anon := types.NewAnon(objID)
		thisVal = anon
		frameThisValue = anon
	}

	ctx := kernel.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = verb.Owner           // Programmer is verb owner, not player
	ctx.IsWizard = s.isWizard(verb.Owner) // Set wizard flag based on verb owner
	ctx.ThisObj = objID
	ctx.ThisValue = frameThisValue
	ctx.Verb = verbName
	ctx.ServerInitiated = true // Mark as server-initiated
	ctx.Task = t               // Attach task so VM can track frames
	ctx.Store = s.store
	ctx.StoreTxn = s.store.BeginReadOnly(0)
	ctx.Registry = s.registry
	ctx.RuntimeOptions = s.options

	// Push activation frame for traceback support
	t.PushFrame(task.ActivationFrame{
		This:            objID,
		ThisValue:       frameThisValue,
		Player:          player,
		Programmer:      verb.Owner,
		Caller:          types.ObjNothing, // Toast: server-initiated hooks run with caller = #-1
		Verb:            verbName,
		VerbLoc:         defObjID,
		Args:            args,
		LineNumber:      1,
		ServerInitiated: true,
	})

	// Create bytecode VM and set up initial frame variables
	bcVM := vm.NewVM(s.store, s.registry)
	bcVM.Context = ctx
	ticks, _ := foregroundTaskLimits()
	bcVM.TickLimit = ticks
	configureVMStackLimit(bcVM)

	// Build the initial verb frame explicitly so we can preserve ANON `this`.
	// Toast sets caller = #-1 in server-initiated hook calls (do_command etc.);
	// conformance parser::do_command_sees_command_verb_and_argstr pins this.
	frame := bcVM.PrepareVerbFrame(prog, objID, player, types.ObjNothing, verbName, defObjID, args)
	frame.VerbDebug = verb.Perms.Has(dbstore.VerbDebug)
	vm.SetLocalByName(frame, prog, "this", thisVal)
	vm.SetLocalByName(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "caller", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "verb", types.NewStr(verbName))
	vm.SetLocalByName(frame, prog, "args", types.NewList(args))
	vm.SetLocalByName(frame, prog, "argstr", types.NewStr(argstr))
	result = bcVM.ExecuteLoop()

	// Handle fork yields: create child tasks and resume parent
	result = s.drainForks(t, bcVM, result)
	committed := true
	if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
		if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
			result = types.Err(errCode)
			committed = false
		}
	}
	if committed {
		t.CreatedForks = nil
		builtins.FlushPendingEffects(ctx)
	} else {
		s.discardCreatedForks(t)
		builtins.DiscardPendingEffects(ctx)
	}

	// Extract call stack BEFORE popping frames
	if result.Flow == types.FlowException {
		stack := t.GetCallStack()
		if result.CallStack != nil {
			if captured, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = captured
			}
		}
		result.CallStack = stack
		// Log traceback to server log
		s.logCallVerbTraceback(objID, verbName, result.Error, stack, player)
		// Trace exception
		trace.Exception(objID, verbName, result.Error)
	} else {
		// Trace return value
		trace.VerbReturn(objID, verbName, result.Val)
	}

	// Clean up call stack
	if len(t.CallStack) > 0 {
		t.PopFrame()
	}

	return result
}
