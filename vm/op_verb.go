package vm

import (
	"fmt"
	"strings"

	"barn/bytecode"
	dbstore "barn/db/store"
	"barn/task"
	"barn/trace"
	"barn/types"
)

// executeCallVerb handles OP_CALL_VERB: call a verb on an object.
//
// Bytecode format: OP_CALL_VERB <verb_name_idx:byte> <argc:byte>
// verb_name_idx = 0xFF means dynamic (verb name string is on top of stack, above args).
//
// Stack layout (top to bottom):
//
//	[verb_name_str] (only if dynamic, i.e. verb_name_idx == 0xFF)
//	arg_N
//	...
//	arg_1
//	obj
//
// Native frame push: compiles the verb to bytecode and pushes a new StackFrame.
// Returns a compile error if bytecode compilation fails.
func (vm *VM) executeCallVerb() error {
	verbNameIdx := vm.ReadByte()
	argc := int(vm.ReadByte())

	// Resolve verb name
	var verbName string
	if verbNameIdx == 0xFF {
		// Dynamic verb name: pop from stack (above args)
		nameVal := vm.Pop()
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: dynamic verb name must be a string")
		}
		verbName = strVal.Value()
	} else {
		// Static verb name: from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[verbNameIdx]
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("internal error: verb name constant is not a string")
		}
		verbName = strVal.Value()
	}

	// Pop arguments
	var args []types.Value
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		list, ok := listVal.(types.ListValue)
		if !ok {
			return fmt.Errorf("E_TYPE: expected list for spliced verb args")
		}
		args = make([]types.Value, list.Len())
		for i := 1; i <= list.Len(); i++ {
			args[i-1] = list.Get(i)
		}
	} else {
		args = vm.PopN(argc)
	}

	// Pop the object
	objVal := vm.Pop()

	// Resolve the object ID from the target value.
	// Handles ObjValue (including anonymous), WaifValue, and primitive prototypes.
	var objID types.ObjID
	var thisValue types.Value // Non-nil for waif, primitive, and anonymous targets
	isWaif := false

	switch target := objVal.(type) {
	case types.ObjValue:
		objID = target.ID()
		if target.IsAnonymous() {
			thisValue = target // "this" = the anonymous ObjValue itself
		}
	case types.WaifValue:
		objID = target.Class() // Verb lookup goes to the waif's class
		thisValue = target     // "this" = the waif itself
		isWaif = true
	default:
		// Check for primitive prototype dispatch (str, int, float, list, map, err, bool)
		if vm.Store != nil {
			protoID := getPrimitivePrototypeFromStore(vm.Store, objVal)
			if protoID != types.ObjNothing {
				objID = protoID
				thisValue = objVal // "this" = the primitive value itself
			} else {
				return fmt.Errorf("E_TYPE: verb call requires an object")
			}
		} else {
			return fmt.Errorf("E_TYPE: verb call requires an object")
		}
	}

	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	// Check object validity
	if !vm.Store.Valid(objID) {
		vm.Store.NoteVerbCacheMiss()
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Look up verb via store (with inheritance)
	lookupVerbName := verbName
	if isWaif && !strings.HasPrefix(lookupVerbName, ":") {
		lookupVerbName = ":" + lookupVerbName
	}
	verb, defObjID, err := vm.Store.FindVerb(objID, lookupVerbName)
	if err != nil {
		vm.Store.NoteVerbCacheMiss()
		return fmt.Errorf("E_VERBNF: verb not found: %s", verbName)
	}

	// A verb without the execute flag is not callable: ToastStunt treats it as
	// nonexistent for call dispatch (E_VERBNF), not a permission error.
	if !verb.Perms.Has(dbstore.VerbExecute) {
		vm.Store.NoteVerbCacheMiss()
		return fmt.Errorf("E_VERBNF: verb not found: %s", verbName)
	}

	// Try to compile verb to bytecode
	prog, compileErr := bytecode.CompileVerbBytecode(verb.Code, vm.Builtins)
	if compileErr != nil {
		return fmt.Errorf("E_VERBNF: compile error in %s: %v", verbName, compileErr)
	}

	// --- Native frame push ---

	// Get current frame's context for caller/player
	currentFrame := vm.CurrentFrame()
	callerObj := currentFrame.This
	player := currentFrame.Player
	if vm.Context != nil && vm.Context.Player != types.ObjNothing {
		player = vm.Context.Player
	}

	// Trace nested verb calls when tracing is enabled.
	trace.VerbCall(objID, lookupVerbName, args, player, callerObj)

	// Save current context fields for restore on return/unwind
	var savedThisObj types.ObjID
	var savedThisValue types.Value
	var savedVerb string
	var savedProgrammer types.ObjID
	var savedIsWizard bool
	if vm.Context != nil {
		savedThisObj = vm.Context.ThisObj
		savedThisValue = vm.Context.ThisValue
		savedVerb = vm.Context.Verb
		savedProgrammer = vm.Context.Programmer
		savedIsWizard = vm.Context.IsWizard
	}

	// Push new stack frame
	frame := &StackFrame{
		Program:         prog,
		IP:              0,
		BasePointer:     vm.SP,
		Locals:          make([]types.Value, prog.NumLocals),
		This:            objID,
		Player:          player,
		Verb:            lookupVerbName,
		StoredVerb:      strings.Join(verb.Names, " "),
		Caller:          callerObj,
		VerbLoc:         defObjID,
		Args:            args,
		LoopStack:       make([]bytecode.LoopState, 0, 4),
		ExceptStack:     make([]bytecode.Handler, 0, 4),
		IsVerbCall:      true,
		VerbDebug:       verb.Perms.Has(dbstore.VerbDebug),
		SavedThisObj:    savedThisObj,
		SavedThisValue:  savedThisValue,
		SavedVerb:       savedVerb,
		SavedProgrammer: savedProgrammer,
		SavedIsWizard:   savedIsWizard,
	}

	// Initialize locals to unbound (reading before assignment raises E_VARNF)
	for i := range frame.Locals {
		frame.Locals[i] = types.UnboundValue{}
	}

	// Pre-populate built-in variables using VarNames.
	// For waif/primitive/anonymous targets, "this" is the actual value, not NewObj(objID).
	if thisValue != nil {
		SetLocalByName(frame, prog, "this", thisValue)
	} else {
		SetLocalByName(frame, prog, "this", types.NewObj(objID))
	}
	SetLocalByName(frame, prog, "verb", types.NewStr(lookupVerbName))
	SetLocalByName(frame, prog, "caller", types.NewObj(callerObj))
	SetLocalByName(frame, prog, "args", types.NewList(args))
	SetLocalByName(frame, prog, "player", types.NewObj(player))

	// Propagate command environment variables from the task's parsed command context.
	// Only propagate real command context when NOT inside an eval() boundary.
	// Eval'd code has no command context, so nested verb calls see empty strings.
	insideEval := false
	for _, f := range vm.Frames {
		if f.IsEvalFrame {
			insideEval = true
			break
		}
	}
	if !insideEval && vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			SetLocalByName(frame, prog, "argstr", types.NewStr(t.Argstr))
			SetLocalByName(frame, prog, "dobjstr", types.NewStr(t.Dobjstr))
			SetLocalByName(frame, prog, "iobjstr", types.NewStr(t.Iobjstr))
			SetLocalByName(frame, prog, "prepstr", types.NewStr(t.Prepstr))
			SetLocalByName(frame, prog, "dobj", types.NewObj(t.Dobj))
			SetLocalByName(frame, prog, "iobj", types.NewObj(t.Iobj))
		}
	} else {
		SetLocalByName(frame, prog, "argstr", types.NewStr(""))
		SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
		SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
		SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
		SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
		SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))
	}

	// Update shared context for builtins
	if vm.Context != nil {
		isWizard := false
		if vm.Store != nil {
			hasWizard, errCode := vm.Store.HasObjectFlag(verb.Owner, dbstore.FlagWizard)
			isWizard = errCode == types.E_NONE && hasWizard
		}
		vm.Context.ThisObj = objID
		vm.Context.ThisValue = thisValue // waif/primitive/anonymous value, or nil for normal
		vm.Context.Verb = lookupVerbName
		vm.Context.Programmer = verb.Owner
		vm.Context.IsWizard = isWizard
	}

	// Push activation frame onto task call stack (if we have a task)
	if vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			actFrame := task.ActivationFrame{
				This:       objID,
				ThisValue:  thisValue, // Store waif/primitive/anonymous value for callers()/queued_tasks()
				Player:     player,
				Programmer: verb.Owner,
				Caller:     callerObj,
				Verb:       lookupVerbName,
				VerbLoc:    defObjID,
				Args:       args,
				LineNumber: 0,
			}
			t.PushFrame(actFrame)
		}
	}

	if err := vm.checkFrameLimit(); err != nil {
		return err
	}
	vm.pushFrame(frame)

	// Return nil — Run() loop continues executing the new frame's bytecode
	return nil
}

// executePass handles OP_PASS: call the same verb on the parent object.
//
// Bytecode format: OP_PASS <argc:byte>
// argc = 0: inherit current frame's args
// argc = 0xFF: splice mode, pop one args list and expand it
// argc > 0: pop argc args from stack
//
// Looks up the parent of VerbLoc (where the current verb is defined),
// finds the same verb name on an ancestor, compiles it to bytecode,
// and pushes a new frame. Preserves `this` (original target).
func (vm *VM) executePass() error {
	argc := int(vm.ReadByte())

	frame := vm.CurrentFrame()
	if frame == nil {
		return fmt.Errorf("E_INVIND: no active frame for pass()")
	}

	verbName := frame.Verb
	if verbName == "" {
		return fmt.Errorf("E_INVIND: pass() called outside of a verb")
	}

	verbLoc := frame.VerbLoc
	if verbLoc == types.ObjNothing {
		return fmt.Errorf("E_INVIND: pass() has no defining object")
	}

	// Get pass-through args
	var passArgs []types.Value
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		list, ok := listVal.(types.ListValue)
		if !ok {
			return fmt.Errorf("E_TYPE: expected list for spliced pass() args")
		}
		passArgs = make([]types.Value, list.Len())
		for i := 1; i <= list.Len(); i++ {
			passArgs[i-1] = list.Get(i)
		}
	} else if argc > 0 {
		passArgs = vm.PopN(argc)
	} else {
		// Inherit args from current frame's stored Args
		if frame.Args != nil {
			passArgs = frame.Args
		} else {
			passArgs = []types.Value{}
		}
	}

	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	verb, defObjID, err := vm.Store.FindParentVerb(verbLoc, verbName)
	if err != nil {
		// Distinguish two cases the way ToastStunt does: if the defining object
		// has no parent at all, pass() indirects through #-1 (an invalid object)
		// and raises E_INVIND; if a real parent simply doesn't define the verb,
		// it's E_VERBNF.
		if parent, _ := vm.Store.Parent(verbLoc); parent == types.ObjNothing {
			return fmt.Errorf("E_INVIND: pass() has no parent object")
		}
		return fmt.Errorf("E_VERBNF: no parent verb for pass()")
	}

	// Check execute permission
	if !verb.Perms.Has(dbstore.VerbExecute) {
		return fmt.Errorf("E_PERM: parent verb %s is not executable", verbName)
	}

	// Compile the parent verb to bytecode
	prog, compileErr := bytecode.CompileVerbBytecode(verb.Code, vm.Builtins)
	if compileErr != nil {
		return fmt.Errorf("E_VERBNF: compile error in pass() for %s: %v", verbName, compileErr)
	}

	// --- Native frame push ---

	// Save current context fields for restore on return/unwind
	var savedThisObj types.ObjID
	var savedThisValue types.Value
	var savedVerb string
	var savedProgrammer types.ObjID
	var savedIsWizard bool
	if vm.Context != nil {
		savedThisObj = vm.Context.ThisObj
		savedThisValue = vm.Context.ThisValue
		savedVerb = vm.Context.Verb
		savedProgrammer = vm.Context.Programmer
		savedIsWizard = vm.Context.IsWizard
	}

	// Preserve the effective `this` value for primitive/waif/anonymous pass() calls.
	passThis := types.Value(types.NewObj(frame.This))
	var passThisValue types.Value
	if vm.Context != nil && vm.Context.ThisValue != nil {
		passThis = vm.Context.ThisValue
		passThisValue = vm.Context.ThisValue
	}

	// Push new stack frame with parent verb's bytecode
	// this = current frame's this (preserve original target)
	// VerbLoc = defObjID (where the parent verb was found, for chained pass())
	newFrame := &StackFrame{
		Program:         prog,
		IP:              0,
		BasePointer:     vm.SP,
		Locals:          make([]types.Value, prog.NumLocals),
		This:            frame.This,
		Player:          frame.Player,
		Verb:            verbName,
		StoredVerb:      strings.Join(verb.Names, " "),
		Caller:          verbLoc,
		VerbLoc:         defObjID,
		Args:            passArgs,
		LoopStack:       make([]bytecode.LoopState, 0, 4),
		ExceptStack:     make([]bytecode.Handler, 0, 4),
		IsVerbCall:      true,
		VerbDebug:       verb.Perms.Has(dbstore.VerbDebug),
		SavedThisObj:    savedThisObj,
		SavedThisValue:  savedThisValue,
		SavedVerb:       savedVerb,
		SavedProgrammer: savedProgrammer,
		SavedIsWizard:   savedIsWizard,
	}

	// Initialize locals to unbound (reading before assignment raises E_VARNF)
	for i := range newFrame.Locals {
		newFrame.Locals[i] = types.UnboundValue{}
	}

	// Pre-populate built-in variables
	SetLocalByName(newFrame, prog, "this", passThis)
	SetLocalByName(newFrame, prog, "verb", types.NewStr(verbName))
	SetLocalByName(newFrame, prog, "caller", types.NewObj(verbLoc))
	SetLocalByName(newFrame, prog, "args", types.NewList(passArgs))
	SetLocalByName(newFrame, prog, "player", types.NewObj(frame.Player))

	// pass() continues the same command, so propagate the command parsing
	// variables to the inherited verb (matching the regular verb-dispatch path
	// above and Toast). Eval'd code has no command context.
	insideEval := false
	for _, f := range vm.Frames {
		if f.IsEvalFrame {
			insideEval = true
			break
		}
	}
	if !insideEval && vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			SetLocalByName(newFrame, prog, "argstr", types.NewStr(t.Argstr))
			SetLocalByName(newFrame, prog, "dobjstr", types.NewStr(t.Dobjstr))
			SetLocalByName(newFrame, prog, "iobjstr", types.NewStr(t.Iobjstr))
			SetLocalByName(newFrame, prog, "prepstr", types.NewStr(t.Prepstr))
			SetLocalByName(newFrame, prog, "dobj", types.NewObj(t.Dobj))
			SetLocalByName(newFrame, prog, "iobj", types.NewObj(t.Iobj))
		}
	} else {
		SetLocalByName(newFrame, prog, "argstr", types.NewStr(""))
		SetLocalByName(newFrame, prog, "dobjstr", types.NewStr(""))
		SetLocalByName(newFrame, prog, "iobjstr", types.NewStr(""))
		SetLocalByName(newFrame, prog, "prepstr", types.NewStr(""))
		SetLocalByName(newFrame, prog, "dobj", types.NewObj(types.ObjNothing))
		SetLocalByName(newFrame, prog, "iobj", types.NewObj(types.ObjNothing))
	}

	// Update shared context for builtins
	if vm.Context != nil {
		isWizard := false
		if vm.Store != nil {
			hasWizard, errCode := vm.Store.HasObjectFlag(verb.Owner, dbstore.FlagWizard)
			isWizard = errCode == types.E_NONE && hasWizard
		}
		vm.Context.ThisObj = frame.This
		vm.Context.ThisValue = passThisValue
		vm.Context.Verb = verbName
		vm.Context.Programmer = verb.Owner
		vm.Context.IsWizard = isWizard
	}

	// Trace pass() target call.
	trace.VerbCall(frame.This, verbName, passArgs, frame.Player, verbLoc)

	// Push activation frame onto task call stack (if we have a task)
	if vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			actFrame := task.ActivationFrame{
				This:       frame.This,
				ThisValue:  passThisValue,
				Player:     frame.Player,
				Programmer: verb.Owner,
				Caller:     verbLoc,
				Verb:       verbName,
				VerbLoc:    defObjID,
				Args:       passArgs,
				LineNumber: 0,
			}
			t.PushFrame(actFrame)
		}
	}

	if err := vm.checkFrameLimit(); err != nil {
		return err
	}
	vm.pushFrame(newFrame)

	// Return nil — Run() loop continues executing the new frame's bytecode
	return nil
}
