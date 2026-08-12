package engine

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// CallVerb synchronously executes a verb on an object and returns the result
// This is used for server hooks like do_login_command, user_connected, etc.
// Returns a Result with a call stack for traceback formatting
func (s *Runtime) CallVerb(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) (result types.Result) {
	return s.CallVerbWithArgstr(objID, verbName, args, player, "")
}

func (s *Runtime) CallVerbInContext(objID types.ObjID, verbName string, args []types.Value, parent *builtins.Execution) types.Result {
	if parent == nil || parent.TaskContext == nil {
		return types.Err(types.E_INVARG)
	}
	parentCtx := parent.TaskContext
	// This is a separate synchronous VM even though it shares the parent's
	// transaction/context. Count it under the same opaque lease so run_gc cannot
	// exclude the sole owner while both outer and inner VMs hold roots. A
	// sweep-owned hook instead inherits the already-held start barrier.
	if !s.isSweepOwnedContext(parentCtx) {
		if ownerTaskID, claimed, attributable := s.executionContextClaim(parentCtx); claimed {
			if !attributable {
				ownerTaskID = ambiguousExecutionOwnerID
			}
			s.acquireInheritedTaskExecution(ownerTaskID)
			s.acquireExecutionContext(parentCtx, ownerTaskID)
			defer func() {
				s.releaseExecutionContext(parentCtx, ownerTaskID)
				s.releaseTaskExecution(ownerTaskID)
			}()
		}
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

	prog, diagnostics := s.registry.Compiler().CompileMOOWithKey(verb.Code, verb.CodeKey)
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

	parentTask := parent.Task
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

	// Pooled: nothing outlives this call. The VM is never handed to the task
	// (SetBytecodeVM is not called here), a FlowSuspend from a nested hook is
	// returned to the caller and the VM dropped, and ReleaseVM itself declines to
	// pool a still-yielded VM. Release happens after drainForks, which resumes on
	// this same VM.
	bcVM := vm.AcquireVM(s.store, s.registry)
	bcVM.Context = parentCtx
	bcVM.Task = parentTask
	ticks, _ := foregroundTaskLimits(s.registry)
	bcVM.TickLimit = ticks
	configureVMStackLimit(bcVM, s.registry)

	frame := bcVM.PrepareVerbFrame(prog, objID, player, caller, verbName, defObjID, args)
	frame.IsVerbCall = true
	frame.VerbDebug = verb.Perms.Has(dbstore.VerbDebug)
	frame.StoredVerbNames = verb.Names
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
	vm.ReleaseVM(bcVM)
	if result.Flow == types.FlowException {
		trace.Exception(objID, verbName, result.Error)
	} else {
		trace.VerbReturn(objID, verbName, result.Val)
	}
	return result
}

type vmOwnership uint8

const (
	vmOwnershipNone vmOwnership = iota
	vmOwnershipExecution
	vmOwnershipSweep
)

func (s *Runtime) CallVerbWithArgstr(objID types.ObjID, verbName string, args []types.Value, player types.ObjID, argstr string) (result types.Result) {
	return s.callVerbWithArgstr(objID, verbName, args, player, argstr, vmOwnershipNone, 0)
}

func (s *Runtime) callVerbWithArgstr(objID types.ObjID, verbName string, args []types.Value, player types.ObjID, argstr string, ownership vmOwnership, ownerTaskID int64) (result types.Result) {
	s.beginFinalizationProducer()
	defer s.finishFinalizationProducer()
	var leasedTask *task.Task
	var inheritedLease bool
	var ownedCtx *kernel.TaskContext
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
		if ownedCtx != nil {
			switch ownership {
			case vmOwnershipExecution:
				s.releaseExecutionContext(ownedCtx, ownerTaskID)
			case vmOwnershipSweep:
				s.releaseSweepContext(ownedCtx)
			}
		}
		if inheritedLease {
			s.releaseTaskExecution(ownerTaskID)
		}
		if leasedTask != nil {
			s.releaseTaskExecution(leasedTask.ID)
			s.flushDeferredGC()
		}
	}()

	// Create the lightweight task without touching the target or arguments, then
	// publish ownership at the earliest safe entry. Anonymous/waif arguments are
	// roots even during trace, lookup, and compile, before a VM frame exists to be
	// scanned by GC.
	t := &task.Task{
		Owner:       player,
		Programmer:  player, // Will be updated to verb owner if verb found
		CallStack:   make([]task.ActivationFrame, 0),
		TaskLocal:   types.NewEmptyMap(), // Initialize task_local to empty map
		ForkCreator: s,                   // Enable fork support in server hooks
	}
	if ownership == vmOwnershipNone {
		s.acquireTaskExecution(t)
		leasedTask = t
		ownership = vmOwnershipExecution
		ownerTaskID = t.ID
	} else if ownership == vmOwnershipExecution {
		s.acquireInheritedTaskExecution(ownerTaskID)
		inheritedLease = true
	}

	// Trace only after the direct path owns either a physical lease or the
	// caller's sweep barrier.
	trace.VerbCall(objID, verbName, args, player, player)

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
	prog, diagnostics := s.registry.Compiler().CompileMOOWithKey(verb.Code, verb.CodeKey)
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
	ctx.Store = s.store
	ctx.StoreTxn = s.store.BeginReadOnly(0)
	ctx.RuntimeOptions = s.options

	// Propagate the already-published ownership to nested registry hooks.
	switch ownership {
	case vmOwnershipExecution:
		s.acquireExecutionContext(ctx, ownerTaskID)
	case vmOwnershipSweep:
		s.acquireSweepContext(ctx)
	}
	ownedCtx = ctx

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

	// Create bytecode VM and set up initial frame variables. Pooled for the same
	// reason as CallVerbInContext: this VM is never stored on the task, and the
	// only post-execution uses (commit, effect flush, traceback) read the task and
	// the context, never the VM.
	bcVM := vm.AcquireVM(s.store, s.registry)
	bcVM.Context = ctx
	bcVM.Task = t
	ticks, _ := foregroundTaskLimits(s.registry)
	bcVM.TickLimit = ticks
	configureVMStackLimit(bcVM, s.registry)

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
	vm.ReleaseVM(bcVM)
	committed := true
	if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
		if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
			result = types.Err(errCode)
			committed = false
		}
	}
	if committed {
		t.CreatedForks = nil
		builtins.FlushPendingEffects(s.registry.NewExecution(ctx, t))
	} else {
		s.discardCreatedForks(t)
		builtins.DiscardPendingEffects(s.registry.NewExecution(ctx, t))
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
