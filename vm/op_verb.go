package vm

import (
	"strings"
	"sync/atomic"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
)

// executeCallVerb handles the original OP_CALL_VERB encoding. A 0xFF name
// operand remains the legacy dynamic form so persisted suspended tasks resume
// correctly; newly compiled programs use OP_CALL_VERB_DYNAMIC.
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
	verbNameIdx := vm.FetchByte()
	if verbNameIdx == 0xFF {
		return vm.executeCallVerbDynamic()
	}
	return vm.executeCallVerbStatic(int(verbNameIdx))
}

func (vm *VM) executeCallVerbWide() error {
	return vm.executeCallVerbStatic(int(vm.ReadShort()))
}

func (vm *VM) executeCallVerbStatic(nameIdx int) error {
	if _, err := vm.staticNameFromConstant(nameIdx, "verb"); err != nil {
		return err
	}
	program := vm.CurrentFrame().Program
	return vm.executeCallVerbNamed(program.Constants[nameIdx], int(vm.FetchByte()), callSiteAt(program, nameIdx))
}

func (vm *VM) executeCallVerbDynamic() error {
	argc := int(vm.FetchByte())
	name := vm.Pop()
	if name.Type() != types.TYPE_STR {
		return newMooError(types.E_TYPE, "dynamic verb name must be a string")
	}
	return vm.executeCallVerbNamed(name, argc, nil)
}

// executeCallVerbNamed takes the verb name as the string value the program
// already holds, which becomes the callee's `verb` without a copy. site is the
// call site of a static name, or nil.
func (vm *VM) executeCallVerbNamed(name types.Value, argc int, site *verbCallSite) error {
	// Pop arguments
	var args []types.Value
	argsList := types.None
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		if listVal.Type() != types.TYPE_LIST {
			return newMooError(types.E_TYPE, "expected list for spliced verb args")
		}
		args, argsList = splicedArgs(listVal), listVal
	} else {
		args = vm.PopN(argc)
	}

	// Pop the object
	objVal := vm.Pop()
	return vm.startVerbCallResolved(objVal, name.Str(), name, args, argsList, nil, site)
}

func (vm *VM) startVerbCall(objVal types.Value, verbName string, args []types.Value) error {
	return vm.startVerbCallResolved(objVal, verbName, types.None, args, types.None, nil, nil)
}

// resolvedVerb is a call target the caller has already looked up, through the
// task's transaction, for the object and name it passes to
// startVerbCallResolved.
type resolvedVerb struct {
	verb    dbstore.VerbView
	definer types.ObjID
}

// startVerbCallResolved is startVerbCall with the verb lookup optionally done
// by the caller: a nil resolved looks the verb up here. nameValue is verbName
// as a string value when the caller has one, and None otherwise. argsList is
// args as a list value when the caller has one (a spliced call), which becomes
// the callee's `args` without a copy, and None otherwise. site, when not nil,
// is the static call site making the call; only a call on a plain object
// number uses it.
func (vm *VM) startVerbCallResolved(objVal types.Value, verbName string, nameValue types.Value, args []types.Value, argsList types.Value, resolved *resolvedVerb, site *verbCallSite) error {
	if objVal.Type() != types.TYPE_OBJ || objVal.IsAnonymous() {
		site = nil
	}
	// Resolve the object ID from the target value.
	// Handles ObjValue (including anonymous), WaifValue, and primitive prototypes.
	var objID types.ObjID
	thisValue := types.None // Non-None for waif, primitive, and anonymous targets
	isWaif := false

	switch objVal.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		objID = objVal.ID()
		if objVal.IsAnonymous() {
			thisValue = objVal // "this" = the anonymous ObjValue itself
		}
	case types.TYPE_WAIF:
		objID = objVal.Class() // Verb lookup goes to the waif's class
		thisValue = objVal     // "this" = the waif itself
		isWaif = true
	default:
		// Check for primitive prototype dispatch (str, int, float, list, map, err, bool)
		if vm.Store != nil {
			protoID := getPrimitivePrototypeFromStore(vm.Context.StoreTxn, objVal)
			if protoID != types.ObjNothing {
				objID = protoID
				thisValue = objVal // "this" = the primitive value itself
			} else {
				return newMooError(types.E_TYPE, "verb call requires an object")
			}
		} else {
			return newMooError(types.E_TYPE, "verb call requires an object")
		}
	}

	if vm.Store == nil {
		return newMooError(types.E_INVIND, "no object store available")
	}

	txn := vm.Context.StoreTxn

	// Check object validity
	if !validForRead(txn, objID) {
		vm.Store.NoteVerbCacheMiss()
		return newMooErrorf(types.E_INVIND, "invalid object #%d", objID)
	}

	// Look up verb via store (with inheritance)
	lookupVerbName := verbName
	if isWaif && !strings.HasPrefix(lookupVerbName, ":") {
		lookupVerbName = ":" + lookupVerbName
		nameValue = types.None
	}
	// A verb without the execute flag does not shadow a same-named, executable
	// verb defined further up the ancestry chain — ToastStunt's call dispatch
	// (obj:verb() syntax) skips past it and keeps searching. Only when no
	// ancestor defines an executable match does dispatch fail, as E_VERBNF.
	var verb dbstore.VerbView
	var defObjID types.ObjID
	if resolved != nil {
		verb, defObjID = resolved.verb, resolved.definer
	} else {
		var err error
		if site != nil {
			verb, defObjID, err = txn.FindCallableVerbAt(site.lookup, objID, lookupVerbName)
		} else {
			verb, defObjID, err = findCallableVerbForRead(txn, objID, lookupVerbName)
		}
		if err != nil {
			vm.Store.NoteVerbCacheMiss()
			return newMooErrorf(types.E_VERBNF, "verb not found: %s", verbName)
		}
	}

	// Try to compile verb to bytecode. The store carries the verb's content key,
	// so the cache lookup on this hot path does not rehash the source.
	var prog *bytecode.Program
	var diagnostics []compiler.Diagnostic
	if site != nil {
		prog, diagnostics = site.compile(vm.Builtins.Registry().Compiler(), verb.Code, verb.CodeKey)
	} else {
		prog, diagnostics = vm.Builtins.Registry().Compiler().CompileMOOWithKey(verb.Code, verb.CodeKey)
	}
	if len(diagnostics) > 0 {
		return newMooErrorf(types.E_VERBNF, "compile error in %s: %s", verbName, diagnostics[0].Error())
	}

	// Enforce the frame limit before touching the task context, locals, or
	// activation stack: like Toast's call_verb2, a call refused with E_MAXREC
	// leaves the caller running with its own this, permissions, and thread mode.
	if err := vm.checkFrameLimit(); err != nil {
		return err
	}

	// --- Native frame push ---

	// Get current frame's context for caller/player
	currentFrame := vm.CurrentFrame()
	callerObj := currentFrame.This
	callerValue := types.NewObj(callerObj)
	if !currentFrame.ThisValue.IsNone() {
		callerValue = currentFrame.ThisValue
	}
	player := currentFrame.Player
	if slot := currentFrame.Program.BuiltinSlots.Player; slot != 0 && vm.Context != nil && vm.Context.IsWizard {
		if local := currentFrame.Locals[slot-1]; local.Type() == types.TYPE_OBJ {
			player = local.Obj()
		}
	}

	// Trace nested verb calls when tracing is enabled.
	trace.VerbCall(objID, lookupVerbName, len(args), player, callerObj)

	// Save current context fields for restore on return/unwind
	var savedThisObj types.ObjID
	savedThisValue := types.None
	var savedVerb string
	var savedProgrammer types.ObjID
	var savedIsWizard bool
	savedThreadMode := true
	if vm.Context != nil {
		savedThisObj = vm.Context.ThisObj
		savedThisValue = vm.Context.ThisValue
		savedVerb = vm.Context.Verb
		savedProgrammer = vm.Context.Programmer
		savedIsWizard = vm.Context.IsWizard
		savedThreadMode = vm.Context.ThreadMode
	}

	// Push new stack frame. copyCommandLocals below sets every command
	// variable, so the frame skips their defaults.
	frame := vm.frameWithoutCommandDefaults(StackFrame{
		Program:         prog,
		IP:              0,
		BasePointer:     vm.SP,
		Locals:          vm.allocLocals(prog.NumLocals),
		localsOnStack:   true,
		This:            objID,
		ThisValue:       thisValue,
		Player:          player,
		Verb:            lookupVerbName,
		StoredVerbNames: verb.Names,
		Caller:          callerObj,
		VerbLoc:         defObjID,
		Args:            args,
		IsVerbCall:      true,
		VerbDebug:       verb.Perms.Has(dbstore.VerbDebug),
		SavedThisObj:    savedThisObj,
		SavedThisValue:  savedThisValue,
		SavedVerb:       savedVerb,
		SavedProgrammer: savedProgrammer,
		SavedIsWizard:   savedIsWizard,
		SavedThreadMode: savedThreadMode,
	})

	// Pre-populate built-in variables using their compiler-resolved slots.
	// For waif/primitive/anonymous targets, "this" is the actual value, not NewObj(objID).
	if !thisValue.IsNone() {
		SetLocalBySlot(frame, prog.BuiltinSlots.This, thisValue)
	} else {
		SetLocalBySlot(frame, prog.BuiltinSlots.This, types.NewObj(objID))
	}
	if nameValue.IsNone() {
		nameValue = types.NewStr(strings.TrimPrefix(lookupVerbName, ":"))
	}
	SetLocalBySlot(frame, prog.BuiltinSlots.Verb, nameValue)
	SetLocalBySlot(frame, prog.BuiltinSlots.Caller, callerValue)
	setArgsLocal(frame, args, argsList)
	SetLocalBySlot(frame, prog.BuiltinSlots.Player, types.NewObj(player))

	copyCommandLocals(frame, currentFrame)

	// Update shared context for builtins
	if vm.Context != nil {
		isWizard := false
		if vm.Store != nil {
			hasWizard, errCode := hasObjectFlagForRead(txn, verb.Owner, dbstore.FlagWizard)
			isWizard = errCode == types.E_NONE && hasWizard
		}
		vm.Context.ThisObj = objID
		vm.Context.ThisValue = thisValue // waif/primitive/anonymous value, or nil for normal
		vm.Context.Verb = lookupVerbName
		vm.Context.Programmer = verb.Owner
		vm.Context.IsWizard = isWizard
		// Every verb activation, pass() included, starts with Toast's
		// default thread mode (call_verb2 with DEFAULT_THREAD_MODE).
		vm.Context.ThreadMode = true
	}

	// Push activation frame onto task call stack (if we have a task)
	if vm.Task != nil {
		actFrame := types.ActivationFrame{
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
		vm.Task.PushFrame(actFrame)
	}

	vm.pushFrame(frame)

	// Return nil — Run() loop continues executing the new frame's bytecode
	return nil
}

func (vm *VM) markBuiltinCaller(name string, pc int) {
	frame := vm.CurrentFrame()
	frame.BuiltinCaller, frame.BuiltinPC = name, pc
	if vm.Task != nil {
		vm.Task.SetTopFrameBuiltinCaller(name, pc)
	}
}

func (vm *VM) pushInitialize(created types.Value, args []types.Value, builtin string) types.Result {
	// The builtin returns the created object after the callback, regardless of
	// the callback's normal result. The ordinary stack preserves it on suspend.
	vm.Push(created)
	if err := vm.startVerbCall(created, "initialize", args); err != nil {
		vm.Pop()
		if errorCode(err) == types.E_VERBNF {
			return types.Ok(created)
		}
		return moveLifecycleErrorResult(err)
	}
	vm.CurrentFrame().DiscardReturn = true
	vm.markBuiltinCaller(builtin, 2)
	return types.Result{Flow: types.FlowBuiltinPush}
}

// pushProtectedVerb shares ordinary verb activation, return, unwind, and
// suspension with the calling VM. It resolves #0:name from the call site in
// slot, the protected builtin's own, so repeated calls reuse the site's lookup
// record and compiled program; found is false, with nothing pushed, when #0
// has no callable verb by that name. args belongs to the builtin dispatcher and
// is copied for the wrapper's frame.
func (vm *VM) pushProtectedVerb(name types.Value, slot *atomic.Value, args []types.Value) (types.Result, bool) {
	site := callSiteIn(slot)
	verb, definer, err := vm.Context.StoreTxn.FindCallableVerbAt(site.lookup, types.ObjID(0), name.Str())
	if err != nil {
		return types.Result{}, false
	}
	threadMode := vm.Context.ThreadMode
	resolved := resolvedVerb{verb: verb, definer: definer}
	args = append([]types.Value(nil), args...)
	if err := vm.startVerbCallResolved(types.NewObj(0), name.Str(), name, args, types.None, &resolved, site); err != nil {
		result := types.Err(errorCode(err))
		result.Reason = exceptionReason(err)
		return result, true
	}
	// Toast runs a #0:bf_<name> wrapper in the calling activation's thread
	// mode rather than the default one (functions.cc call_bi_func).
	vm.Context.ThreadMode = threadMode
	return types.Result{Flow: types.FlowBuiltinPush}, true
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
	argc := int(vm.FetchByte())

	frame := vm.CurrentFrame()
	if frame == nil {
		return newMooError(types.E_INVIND, "no active frame for pass()")
	}
	// Get pass-through args
	var passArgs []types.Value
	passList := types.None
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		if listVal.Type() != types.TYPE_LIST {
			return newMooError(types.E_TYPE, "expected list for spliced pass() args")
		}
		passArgs, passList = splicedArgs(listVal), listVal
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

	// Non-debug frames resume after errors, so consume operands and args first.
	// Legacy programs have no layout fingerprint, but still require pass enabled.
	registry := vm.Builtins.Registry()
	if !registry.Has("pass") || !registry.Compiler().Accepts(frame.Program) {
		return VMException{Code: types.E_INVARG}
	}

	verbName := frame.Verb
	if verbName == "" {
		return newMooError(types.E_INVIND, "pass() called outside of a verb")
	}

	verbLoc := frame.VerbLoc
	if verbLoc == types.ObjNothing {
		return newMooError(types.E_INVIND, "pass() has no defining object")
	}

	if vm.Store == nil {
		return newMooError(types.E_INVIND, "no object store available")
	}

	txn := vm.Context.StoreTxn
	parents, parentsErr := txn.Parents(verbLoc)
	if parentsErr != types.E_NONE || len(parents) == 0 {
		return newMooError(types.E_INVIND, "pass() has no parent object")
	}

	// FindParentVerb walks ancestors the same way obj:verb() dispatch does
	// (FindCallableVerb): a non-executable same-named verb on an intermediate
	// ancestor is skipped, not treated as a match, so it never shadows an
	// executable verb defined further up the chain.
	verb, defObjID, err := findParentVerbForRead(txn, verbLoc, verbName)
	if err != nil {
		// Distinguish two cases the way ToastStunt does: if the defining object
		// has no parent at all, pass() indirects through #-1 (an invalid object)
		// and raises E_INVIND; if a real parent simply doesn't define the verb,
		// it's E_VERBNF.
		if parent, _ := txn.Parent(verbLoc); parent == types.ObjNothing {
			return newMooError(types.E_INVIND, "pass() has no parent object")
		}
		return newMooError(types.E_VERBNF, "no parent verb for pass()")
	}

	// Compile the parent verb to bytecode, keyed by the store's content key.
	prog, diagnostics := vm.Builtins.Registry().Compiler().CompileMOOWithKey(verb.Code, verb.CodeKey)
	if len(diagnostics) > 0 {
		return newMooErrorf(types.E_VERBNF, "compile error in pass() for %s: %s", verbName, diagnostics[0].Error())
	}

	// As in startVerbCall, a pass() refused with E_MAXREC must leave the
	// caller's context untouched, so check before any of it changes.
	if err := vm.checkFrameLimit(); err != nil {
		return err
	}

	// --- Native frame push ---

	// Save current context fields for restore on return/unwind
	var savedThisObj types.ObjID
	savedThisValue := types.None
	var savedVerb string
	var savedProgrammer types.ObjID
	var savedIsWizard bool
	savedThreadMode := true
	if vm.Context != nil {
		savedThisObj = vm.Context.ThisObj
		savedThisValue = vm.Context.ThisValue
		savedVerb = vm.Context.Verb
		savedProgrammer = vm.Context.Programmer
		savedIsWizard = vm.Context.IsWizard
		savedThreadMode = vm.Context.ThreadMode
	}

	// Preserve the effective `this` value for primitive/waif/anonymous pass() calls.
	passThis := types.NewObj(frame.This)
	passThisValue := types.None
	if !frame.ThisValue.IsNone() {
		passThis = frame.ThisValue
		passThisValue = frame.ThisValue
	}

	// Push new stack frame with parent verb's bytecode
	// this = current frame's this (preserve original target)
	// VerbLoc = defObjID (where the parent verb was found, for chained pass())
	// copyCommandLocals below sets every command variable, so the frame skips
	// their defaults.
	newFrame := vm.frameWithoutCommandDefaults(StackFrame{
		Program:         prog,
		IP:              0,
		BasePointer:     vm.SP,
		Locals:          vm.allocLocals(prog.NumLocals),
		localsOnStack:   true,
		This:            frame.This,
		ThisValue:       passThisValue,
		Player:          frame.Player,
		Verb:            verbName,
		StoredVerbNames: verb.Names,
		Caller:          frame.Caller,
		VerbLoc:         defObjID,
		Args:            passArgs,
		IsVerbCall:      true,
		VerbDebug:       verb.Perms.Has(dbstore.VerbDebug),
		SavedThisObj:    savedThisObj,
		SavedThisValue:  savedThisValue,
		SavedVerb:       savedVerb,
		SavedProgrammer: savedProgrammer,
		SavedIsWizard:   savedIsWizard,
		SavedThreadMode: savedThreadMode,
	})

	// Pre-populate built-in variables
	SetLocalBySlot(newFrame, prog.BuiltinSlots.This, passThis)
	SetLocalBySlot(newFrame, prog.BuiltinSlots.Verb, types.NewStr(verbName))
	SetLocalBySlot(newFrame, prog.BuiltinSlots.Caller, types.NewObj(frame.Caller))
	setArgsLocal(newFrame, passArgs, passList)
	SetLocalBySlot(newFrame, prog.BuiltinSlots.Player, types.NewObj(frame.Player))

	copyCommandLocals(newFrame, frame)

	// Update shared context for builtins
	if vm.Context != nil {
		isWizard := false
		if vm.Store != nil {
			hasWizard, errCode := hasObjectFlagForRead(txn, verb.Owner, dbstore.FlagWizard)
			isWizard = errCode == types.E_NONE && hasWizard
		}
		vm.Context.ThisObj = frame.This
		vm.Context.ThisValue = passThisValue
		vm.Context.Verb = verbName
		vm.Context.Programmer = verb.Owner
		vm.Context.IsWizard = isWizard
		// Every verb activation, pass() included, starts with Toast's
		// default thread mode (call_verb2 with DEFAULT_THREAD_MODE).
		vm.Context.ThreadMode = true
	}

	// Trace pass() target call.
	trace.VerbCall(frame.This, verbName, len(passArgs), frame.Player, frame.Caller)

	// Push activation frame onto task call stack (if we have a task)
	if vm.Task != nil {
		actFrame := types.ActivationFrame{
			This:       frame.This,
			ThisValue:  passThisValue,
			Player:     frame.Player,
			Programmer: verb.Owner,
			Caller:     frame.Caller,
			Verb:       verbName,
			VerbLoc:    defObjID,
			Args:       passArgs,
			LineNumber: 0,
		}
		vm.Task.PushFrame(actFrame)
	}

	vm.pushFrame(newFrame)

	// Return nil — Run() loop continues executing the new frame's bytecode
	return nil
}

func validForRead(txn *dbstore.StoreTxn, objID types.ObjID) bool {
	return txn.Valid(objID)
}

func hasObjectFlagForRead(txn *dbstore.StoreTxn, objID types.ObjID, flag dbstore.ObjectFlags) (bool, types.ErrorCode) {
	return txn.HasObjectFlag(objID, flag)
}

// findCallableVerbForRead resolves a verb for call dispatch (obj:verb()): a
// same-named verb without execute permission does not shadow an executable one
// defined further up the ancestry chain. It reads through the task's snapshot
// transaction when present, mirroring findVerbForRead's MVCC behavior.
func findCallableVerbForRead(txn *dbstore.StoreTxn, objID types.ObjID, verbName string) (dbstore.VerbView, types.ObjID, error) {
	return txn.FindCallableVerb(objID, verbName)
}

func findParentVerbForRead(txn *dbstore.StoreTxn, verbLoc types.ObjID, verbName string) (dbstore.VerbView, types.ObjID, error) {
	return txn.FindParentVerb(verbLoc, verbName)
}

// setArgsLocal fills a new verb frame's `args` variable with args, as list
// when the caller already holds them as a list value (None otherwise). A
// program that never names `args` has no slot for it, and nothing reads the
// list then: the arguments stay on the frame (frame.Args) for pass(),
// suspension and callers, so no list is built.
func setArgsLocal(frame *StackFrame, args []types.Value, list types.Value) {
	slot := frame.Program.BuiltinSlots.Args
	if slot == 0 {
		return
	}
	if list.IsNone() {
		list = types.NewList(args)
	}
	SetLocalBySlot(frame, slot, list)
}

// commandSlots returns the program's one-based local slots for the command
// variables, in task.CommandVariableNames order; zero means not referenced.
func commandSlots(program *bytecode.Program) [6]int {
	s := &program.BuiltinSlots
	return [...]int{s.Argstr, s.Dobjstr, s.Iobjstr, s.Prepstr, s.Dobj, s.Iobj}
}

func copyCommandLocals(dst, src *StackFrame) {
	slots := commandSlots(dst.Program)
	for index, value := range src.commandEnvironment() {
		if slot := slots[index]; slot != 0 {
			SetLocalBySlot(dst, slot, value)
		} else {
			dst.CommandEnvironment[index] = value
		}
	}
}

func (frame *StackFrame) commandEnvironment() [6]types.Value {
	values := frame.CommandEnvironment
	for index, slot := range commandSlots(frame.Program) {
		if slot != 0 {
			values[index] = frame.Locals[slot-1]
		}
	}
	return values
}
