package vm

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// PersistenceVMSnapshot returns a detached copy of every state component
// required to resume this yielded VM.
func (vm *VM) PersistenceVMSnapshot() *task.VMSnapshot {
	if vm == nil || !vm.yielded {
		return nil
	}

	snapshot := &task.VMSnapshot{
		MaxStackDepth: vm.MaxStackDepth,
		Frames:        make([]task.VMFrameSnapshot, 0, len(vm.Frames)),
	}
	for i, frame := range vm.Frames {
		stackEnd := vm.SP
		if i+1 < len(vm.Frames) {
			stackEnd = vm.Frames[i+1].BasePointer
		}
		if stackEnd < frame.BasePointer {
			stackEnd = frame.BasePointer
		}

		saved := task.VMFrameSnapshot{
			Program:             cloneProgram(frame.Program),
			IP:                  frame.IP,
			Locals:              append([]types.Value(nil), frame.Locals...),
			CommandEnvironment:  frame.CommandEnvironment,
			Stack:               append([]types.Value(nil), vm.Stack[frame.BasePointer:stackEnd]...),
			This:                frame.This,
			ThisValue:           frame.ThisValue,
			Player:              frame.Player,
			Verb:                frame.Verb,
			StoredVerb:          storedVerbName(frame),
			Caller:              frame.Caller,
			VerbLoc:             frame.VerbLoc,
			Args:                append([]types.Value(nil), frame.Args...),
			ExceptStack:         cloneHandlers(frame.ExceptStack),
			PendingReasons:      snapshotPendingReasons(frame.PendingReasons),
			VerbDebug:           frame.VerbDebug,
			DiscardReturn:       frame.DiscardReturn,
			ReturnLocal:         frame.ReturnLocal,
			BuiltinCaller:       frame.BuiltinCaller,
			BuiltinPC:           frame.BuiltinPC,
			IsVerbCall:          frame.IsVerbCall,
			IsEvalFrame:         frame.IsEvalFrame,
			ThreadMode:          activationThreadMode(vm.Frames, i, vm.Context),
			SavedThisObj:        frame.SavedThisObj,
			SavedThisValue:      frame.SavedThisValue,
			SavedVerb:           frame.SavedVerb,
			SavedProgrammer:     frame.SavedProgrammer,
			SavedIsWizard:       frame.SavedIsWizard,
			MoveContinuation:    cloneMoveContinuation(frame.MoveContinuation),
			RecycleContinuation: snapshotRecycleContinuation(frame.RecycleContinuation),
		}
		snapshot.Frames = append(snapshot.Frames, saved)
	}
	return snapshot
}

// activationThreadMode reports frame i's own thread mode. The context holds the
// running activation's mode; each lower activation's mode is what the nearest
// context-saving frame above it saved when it was pushed.
func activationThreadMode(frames []*StackFrame, i int, ctx *task.Context) bool {
	for above := i + 1; above < len(frames); above++ {
		if savesContext(frames[above]) {
			return frames[above].SavedThreadMode
		}
	}
	return ctx == nil || ctx.ThreadMode
}

// savesContext reports whether frame restores the task context when popped.
func savesContext(frame *StackFrame) bool {
	return frame.IsVerbCall || frame.IsEvalFrame
}

func storedVerbName(frame *StackFrame) string {
	if frame.StoredVerb != "" {
		return frame.StoredVerb
	}
	return strings.Join(frame.StoredVerbNames, " ")
}

// RestoreVMSnapshot reconstructs a yielded VM from checkpoint state.
func RestoreVMSnapshot(
	snapshot *task.VMSnapshot,
	store *dbstore.Store,
	session *builtins.Session,
	ctx *task.Context,
) (*VM, error) {
	var err error
	snapshot, err = convertNativeSnapshot(snapshot, store, session)
	if err != nil {
		return nil, err
	}
	snapshot, err = rebuildSnapshot(snapshot, session)
	if err != nil {
		return nil, err
	}
	if err := validateVMSnapshot(snapshot); err != nil {
		return nil, err
	}

	machine := NewVM(store, session)
	machine.Context = ctx
	if snapshot.MaxStackDepth > 0 {
		machine.MaxStackDepth = snapshot.MaxStackDepth
	}
	machine.Frames = make([]*StackFrame, 0, len(snapshot.Frames))
	machine.Stack = nil
	recycleIDs := make([]types.ObjID, 0)

	for _, saved := range snapshot.Frames {
		if saved.Program.BuiltinLayout != [32]byte{} && (session == nil || !session.Registry().Compiler().Accepts(&saved.Program)) {
			return nil, fmt.Errorf("saved builtin registry layout differs from runtime")
		}
		program := cloneProgram(&saved.Program)
		base := len(machine.Stack)
		machine.Stack = append(machine.Stack, saved.Stack...)
		recycleContinuation := restoreRecycleContinuation(saved.RecycleContinuation)
		frame := &StackFrame{
			Program:             &program,
			IP:                  saved.IP,
			BasePointer:         base,
			Locals:              append([]types.Value(nil), saved.Locals...),
			CommandEnvironment:  saved.CommandEnvironment,
			This:                saved.This,
			ThisValue:           saved.ThisValue,
			Player:              saved.Player,
			Verb:                saved.Verb,
			StoredVerb:          saved.StoredVerb,
			Caller:              saved.Caller,
			VerbLoc:             saved.VerbLoc,
			Args:                append([]types.Value(nil), saved.Args...),
			ExceptStack:         cloneHandlers(saved.ExceptStack),
			PendingReasons:      restorePendingReasons(saved.PendingReasons),
			VerbDebug:           saved.VerbDebug,
			DiscardReturn:       saved.DiscardReturn,
			ReturnLocal:         saved.ReturnLocal,
			BuiltinCaller:       saved.BuiltinCaller,
			BuiltinPC:           saved.BuiltinPC,
			IsVerbCall:          saved.IsVerbCall,
			IsEvalFrame:         saved.IsEvalFrame,
			SavedThisObj:        saved.SavedThisObj,
			SavedThisValue:      saved.SavedThisValue,
			SavedVerb:           saved.SavedVerb,
			SavedProgrammer:     saved.SavedProgrammer,
			SavedIsWizard:       saved.SavedIsWizard,
			SavedThreadMode:     true,
			MoveContinuation:    cloneMoveContinuation(saved.MoveContinuation),
			RecycleContinuation: recycleContinuation,
		}
		if n := len(machine.Frames); n > 0 && savesContext(frame) {
			frame.SavedThreadMode = snapshot.Frames[n-1].ThreadMode
		}
		machine.Frames = append(machine.Frames, frame)
		if recycleContinuation != nil && recycleContinuation.request.Object.IsAnonymous() {
			recycleIDs = append(recycleIDs, recycleContinuation.request.Object.ID())
		}
	}

	if len(recycleIDs) > 0 {
		if session == nil {
			return nil, fmt.Errorf("restored recycle lifecycle requires a session")
		}
		if !session.RestoreRecycleGuards(recycleIDs) {
			return nil, fmt.Errorf("restored recycle lifecycle conflicts with active recycle")
		}
	}

	if ctx != nil {
		ctx.ThreadMode = snapshot.Frames[len(snapshot.Frames)-1].ThreadMode
	}
	machine.SP = len(machine.Stack)
	machine.FP = len(machine.Frames) - 1
	machine.frame = machine.Frames[len(machine.Frames)-1]
	machine.yielded = true
	machine.yieldResult = types.Result{Flow: types.FlowSuspend}
	return machine, nil
}

func convertNativeSnapshot(snapshot *task.VMSnapshot, store *dbstore.Store, session *builtins.Session) (*task.VMSnapshot, error) {
	if snapshot == nil || len(snapshot.Frames) == 0 || snapshot.Frames[0].Native == nil {
		return snapshot, nil
	}
	if session == nil || store == nil {
		return nil, fmt.Errorf("native continuation import requires a store and builtin session")
	}
	if snapshot.NativeFunctionID != 0 {
		return nil, fmt.Errorf("unsupported native VM function continuation %d", snapshot.NativeFunctionID)
	}
	converted := &task.VMSnapshot{MaxStackDepth: snapshot.MaxStackDepth, Frames: make([]task.VMFrameSnapshot, len(snapshot.Frames))}
	previousCall := compiler.NativeResumeVerb
	stackBase := 0
	for index, saved := range snapshot.Frames {
		if saved.Native == nil {
			return nil, fmt.Errorf("mixed native and Barn activation encodings")
		}
		vector := -1
		if index == 0 {
			vector = snapshot.NativeRootVector
		}
		var move *task.MoveContinuationSnapshot
		evalFrame := previousCall == compiler.NativeResumeEval
		switch previousCall {
		case compiler.NativeResumeEval:
			saved = withoutEvalReturnContinuation(saved)
		case compiler.NativeResumeBuiltin:
			saved, move = withoutMoveContinuation(saved)
			if move == nil {
				// call_function("eval", ...) is a builtin call site whose callee
				// is an eval activation. ExportNativeSnapshot marks it as it
				// marks eval()'s own.
				marked := saved.Native.Builtin != nil
				saved = withoutEvalReturnContinuation(saved)
				evalFrame = marked && saved.Native.Builtin == nil
			}
		}
		frame, call, err := session.Registry().Compiler().ConvertNativeFrame(saved, vector, index == len(snapshot.Frames)-1)
		if err != nil {
			return nil, fmt.Errorf("native frame %d: %w", index, err)
		}
		frame.IsVerbCall = previousCall == compiler.NativeResumeVerb || previousCall == compiler.NativeResumePass
		frame.IsEvalFrame = evalFrame
		if evalFrame {
			// eval, or call_function when Toast reached eval through it: the
			// name callers() gives the eval activation's builtin entry.
			frame.BuiltinCaller, frame.BuiltinPC = "eval", 2
			if builtin := snapshot.Frames[index].Native.Builtin; builtin != nil {
				frame.BuiltinCaller = builtin.Name
			}
		}
		if move != nil {
			// The frame as pushMoveLifecycleVerb leaves a hook verb's.
			frame.IsVerbCall = true
			frame.DiscardReturn = true
			frame.MoveContinuation = move
			// move, or call_function when Toast reached move through it.
			frame.BuiltinCaller, frame.BuiltinPC = snapshot.Frames[index].Native.Builtin.Name, move.Stage+1
		}
		if index != 0 {
			parent := snapshot.Frames[index-1]
			frame.SavedThisObj = parent.This
			frame.SavedThisValue = parent.ThisValue
			frame.SavedVerb = parent.Verb
			frame.SavedProgrammer = parent.Native.Programmer
			frame.SavedIsWizard, _ = store.DirectTxn().HasObjectFlag(parent.Native.Programmer, dbstore.FlagWizard)
		}
		for handlerIndex := range frame.ExceptStack {
			frame.ExceptStack[handlerIndex].StackDepth += stackBase
		}
		stackBase += len(frame.Stack)
		converted.Frames[index] = frame
		previousCall = call
	}
	return converted, nil
}

// Toast's bf_eval pushes the eval activation and then returns a call package,
// so the OP_BI_FUNC_CALL handler records builtin eval with pc 2 on that new
// activation rather than on its caller. When the frame returns, Toast calls
// bf_eval(next=2), which wraps the value in {1, value}. Barn models the same
// continuation as IsEvalFrame, set from the caller's eval() call site, so
// the callee's own mark is dropped here. Any other builtin state, or an eval
// mark at another pc or with saved data, is left for ConvertNativeFrame to
// refuse.
//
// Under call_function("eval", ...) Toast records builtin call_function with
// the same pc, its line naming eval, and no eval data after it (execute.cc
// bf_call_function, bf_call_function_write; bf_eval has no writer). When the
// eval activation returns, Toast calls the builtin its mark names with the
// value (execute.cc unwind_stack, the bi_func_pc branch of a return), and
// bf_call_function only calls bf_eval(next=2), so that mark is dropped as a
// direct eval()'s. ExportNativeSnapshot writes the plain eval mark there,
// which Toast resumes to the same {1, value}.
func withoutEvalReturnContinuation(saved task.VMFrameSnapshot) task.VMFrameSnapshot {
	builtin := saved.Native.Builtin
	if builtin != nil && builtin.Name == "call_function" && builtin.State == nativeCallFunctionEvalState && builtin.Inner != nil {
		builtin = builtin.Inner
	}
	if saved.BuiltinPC != 2 || builtin == nil || builtin.Name != "eval" || builtin.State != "" || builtin.Inner != nil {
		return saved
	}
	native := *saved.Native
	native.Builtin = nil
	saved.Native = &native
	saved.BuiltinPC = 0
	return saved
}

// Toast's state line for call_function("eval", ...) (execute.cc
// bf_call_function_write).
const nativeCallFunctionEvalState = "bf_call_function data: fname = eval"

// Toast's state line for a move() continuation (objects.cc bf_move_write).
const nativeMoveState = "bf_move data: what = %d, where = %d, position = %d"

// Toast's state line for call_function("move", ...) (execute.cc
// bf_call_function_write).
const nativeCallFunctionMoveState = "bf_call_function data: fname = move"

// Toast's bf_move calls where:accept, oldloc:exitfunc and where:enterfunc as
// verb activations and records builtin move on each one, with the pc do_move
// continues at when that verb returns (2, 3, 4) and its what, where and
// position (objects.cc do_move). Barn models the same continuation as the
// hook frame's MoveContinuation, whose Stage is that pc less one. Toast keeps
// no old location: do_move's oldloc is a local it does not read after
// exitfunc, as continueMoveLifecycleAfterExit does not. Decentralized is what
// builtinMove gives a task that has not yet mutated the live store, which a
// task restored from a checkpoint has not. A move mark with another pc or
// other state is left for ConvertNativeFrame to refuse.
//
// Under call_function("move", ...) Toast records builtin call_function with
// the same pc, its line naming move, and move's data line inside it
// (execute.cc bf_call_function, bf_call_function_write). When the hook
// returns, bf_call_function only calls bf_move with that pc and data, so the
// inner mark is converted as a direct move()'s.
func withoutMoveContinuation(saved task.VMFrameSnapshot) (task.VMFrameSnapshot, *task.MoveContinuationSnapshot) {
	builtin := saved.Native.Builtin
	if builtin != nil && builtin.Name == "call_function" && builtin.State == nativeCallFunctionMoveState && builtin.Inner != nil {
		builtin = builtin.Inner
	}
	if builtin == nil || builtin.Name != "move" || builtin.Inner != nil || saved.BuiltinPC <= moveAwaitAccept || saved.BuiltinPC > moveAwaitEnter+1 {
		return saved, nil
	}
	var what, where, position int64
	if _, err := fmt.Sscanf(builtin.State, nativeMoveState, &what, &where, &position); err != nil || builtin.State != fmt.Sprintf(nativeMoveState, what, where, position) {
		return saved, nil
	}
	move := &task.MoveContinuationSnapshot{
		Stage:         saved.BuiltinPC - 1,
		What:          types.NewObj(types.ObjID(what)),
		Where:         types.NewObj(types.ObjID(where)),
		OldLocation:   types.None,
		Position:      position,
		Decentralized: true,
	}
	native := *saved.Native
	native.Builtin = nil
	saved.Native = &native
	saved.BuiltinPC = 0
	return saved, move
}

// ErrToastLosesObjectContinuation is ExportNativeSnapshot's refusal of a task
// with a verb that create(), recreate() or recycle() called and continues
// after (initialize, the object's recycle, a location's exitfunc). The task
// stays in Barn's frame form, because stock Toast does not resume its own form
// of that continuation correctly (ToastStunt 03af4fa2, src/objects.cc):
// bf_create and bf_recreate keep the object as a Var (427-428, 517-518) and
// return var_ref(*data) when initialize returns (454, 538), but
// bf_create_write prints only the Var's first 8 bytes as an Objid (546-548)
// and bf_create_read allocates and reads one Objid (552-554), so the Var's
// type is read from past that allocation; bf_recycle is the same (833-834,
// 850; bf_recycle_write 962-964, bf_recycle_read 970-972). Recorded runs of
// stock Toast resuming such a checkpoint returned an integer from create()
// and recreate() and reached panic_moo (957) in recycle(). Remove this
// refusal, and write Toast's bf_create/bf_recycle data, once the Toast
// oracle's read functions restore the object value.
var ErrToastLosesObjectContinuation = errors.New("stock Toast does not resume a create, recreate or recycle continuation from its checkpoint")

// ExportNativeSnapshot is convertNativeSnapshot's inverse: it returns snapshot
// with every frame as Toast holds it at the same call, so Toast can load and
// resume it, or an error when any frame has no Toast form. Programmers are
// the activations' (callStack), which Toast stores beside each frame.
func ExportNativeSnapshot(snapshot *task.VMSnapshot, callStack []types.ActivationFrame, compile *compiler.Compiler) (*task.VMSnapshot, error) {
	if snapshot == nil || len(snapshot.Frames) == 0 {
		return nil, fmt.Errorf("empty VM snapshot")
	}
	// Checked before any frame is exported: create()'s caller frame holds the
	// object on its stack, which no Toast call site does.
	for index, saved := range snapshot.Frames {
		// A callback of call_function("create", ...) and the others is named
		// call_function (see nameBuiltinCaller); move's has its continuation.
		name := saved.BuiltinCaller
		if name == "create" || name == "recreate" || name == "recycle" || (name == "call_function" && saved.MoveContinuation == nil && !saved.IsEvalFrame) {
			return nil, fmt.Errorf("frame %d (#%d:%s): %s: %w", index, saved.VerbLoc, saved.Verb, saved.BuiltinCaller, ErrToastLosesObjectContinuation)
		}
	}
	exported := &task.VMSnapshot{MaxStackDepth: snapshot.MaxStackDepth, Frames: make([]task.VMFrameSnapshot, len(snapshot.Frames))}
	for index, saved := range snapshot.Frames {
		move, moveBuiltin := saved.MoveContinuation, saved.BuiltinCaller
		if move != nil {
			// Toast's move data holds object numbers (see withoutMoveContinuation).
			if (moveBuiltin != "move" && moveBuiltin != "call_function") || saved.BuiltinPC != move.Stage+1 || move.What.Type() != types.TYPE_OBJ || move.Where.Type() != types.TYPE_OBJ {
				return nil, fmt.Errorf("frame %d (#%d:%s): move continuation has no Toast form", index, saved.VerbLoc, saved.Verb)
			}
			saved.MoveContinuation, saved.BuiltinCaller, saved.BuiltinPC = nil, "", 0
		}
		evalBuiltin := ""
		if saved.IsEvalFrame {
			// Written below as Toast's eval mark.
			evalBuiltin, saved.BuiltinCaller, saved.BuiltinPC = saved.BuiltinCaller, "", 0
		}
		frame, vector, err := compile.ExportNativeFrame(saved, index == len(snapshot.Frames)-1)
		if err != nil {
			return nil, fmt.Errorf("frame %d (#%d:%s): %w", index, saved.VerbLoc, saved.Verb, err)
		}
		if index == 0 {
			exported.NativeRootVector = vector
		} else if vector != -1 {
			return nil, fmt.Errorf("frame %d (#%d:%s) is not in its program's main vector", index, saved.VerbLoc, saved.Verb)
		}
		if index < len(callStack) {
			frame.Native.Programmer = callStack[index].Programmer
		}
		if saved.IsEvalFrame {
			// Toast holds eval()'s return continuation on the eval activation
			// (see withoutEvalReturnContinuation); without it Toast would hand
			// the caller the bare value instead of {1, value}.
			native := *frame.Native
			native.Builtin = &task.NativeBuiltinSnapshot{Name: "eval"}
			if evalBuiltin == "call_function" {
				// bf_call_function_write: its line naming eval; bf_eval has no
				// data. Toast resumed from it names the entry call_function.
				native.Builtin = &task.NativeBuiltinSnapshot{Name: "call_function", State: "bf_call_function data: fname = eval", Inner: native.Builtin}
			}
			frame.Native = &native
			frame.BuiltinPC = 2
		}
		if move != nil {
			native := *frame.Native
			native.Builtin = &task.NativeBuiltinSnapshot{Name: "move", State: fmt.Sprintf(nativeMoveState, move.What.Obj(), move.Where.Obj(), move.Position)}
			if moveBuiltin == "call_function" {
				// bf_call_function_write: its line naming move, then move's.
				native.Builtin = &task.NativeBuiltinSnapshot{Name: "call_function", State: nativeCallFunctionMoveState, Inner: native.Builtin}
			}
			frame.Native = &native
			frame.BuiltinPC = move.Stage + 1
		}
		exported.Frames[index] = frame
	}
	return exported, nil
}

// ProgramMismatchError refuses a saved task one of whose frames ran a program
// that compiling the frame's saved source does not produce today.
type ProgramMismatchError struct {
	Frame   int
	Verb    string
	VerbLoc types.ObjID
	Reason  string
}

func (e *ProgramMismatchError) Error() string {
	return fmt.Sprintf("frame %d (#%d:%s): %s", e.Frame, e.VerbLoc, e.Verb, e.Reason)
}

// rebuildSnapshot replaces the program of every frame read from a database
// (Rebuild set) with the compiler's output for its saved source, so a saved
// task can only run code a compile of that source produces.
func rebuildSnapshot(snapshot *task.VMSnapshot, session *builtins.Session) (*task.VMSnapshot, error) {
	if snapshot == nil || !slices.ContainsFunc(snapshot.Frames, func(frame task.VMFrameSnapshot) bool { return frame.Rebuild != nil }) {
		return snapshot, nil
	}
	if session == nil {
		return nil, fmt.Errorf("rebuilding saved programs requires a builtin session")
	}
	compile := session.Registry().Compiler()
	rebuilt := *snapshot
	rebuilt.Frames = append([]task.VMFrameSnapshot(nil), snapshot.Frames...)
	for index := range rebuilt.Frames {
		frame := &rebuilt.Frames[index]
		if frame.Rebuild == nil {
			continue
		}
		mismatch := func(reason string) error {
			return &ProgramMismatchError{Frame: index, Verb: frame.Verb, VerbLoc: frame.VerbLoc, Reason: reason}
		}
		if frame.Rebuild.Legacy {
			if !rebuildLegacyFrame(compile, frame) {
				return nil, mismatch("saved bytecode is not the compiler's output for its source")
			}
		} else {
			program, err := compile.Rebuild(frame.Program.Source, frame.Program.Origin)
			if err != nil {
				return nil, mismatch(err.Error())
			}
			if program.Fingerprint() != frame.Rebuild.Fingerprint {
				return nil, mismatch("compiling its source no longer gives the program the task ran")
			}
			locals, err := placeLocals(program, frame)
			if err != nil {
				return nil, fmt.Errorf("frame %d: %w", index, err)
			}
			frame.Program, frame.Locals = cloneProgram(program), locals
		}
		frame.Rebuild = nil
	}
	return &rebuilt, nil
}

// rebuildLegacyFrame replaces a v1 frame's stored program with the compiled
// program, or one of its fork bodies, whose Code and Constants equal it.
func rebuildLegacyFrame(compile *compiler.Compiler, frame *task.VMFrameSnapshot) bool {
	root, diagnostics := compile.CompileMOO(frame.Program.Source)
	if len(diagnostics) != 0 {
		return false
	}
	for candidate, fork := root, 0; candidate != nil; fork++ {
		if slices.Equal(candidate.Code, frame.Program.Code) && slices.EqualFunc(candidate.Constants, frame.Program.Constants, func(a, b types.Value) bool {
			return bytecode.ConstantKey(a) == bytecode.ConstantKey(b)
		}) {
			frame.Program = cloneProgram(candidate)
			return true
		}
		candidate = root.ExtractFork(fork + 1)
	}
	return false
}

// placeLocals moves a v2 frame's named locals, read in saved order, and its
// compiler temporaries into the rebuilt program's slots.
func placeLocals(program *bytecode.Program, frame *task.VMFrameSnapshot) ([]types.Value, error) {
	locals := make([]types.Value, program.NumLocals)
	for slot := range locals {
		locals[slot] = types.Unbound
	}
	for index, name := range frame.Program.VarNames {
		slot := slices.Index(program.VarNames, name)
		if slot < 0 || index >= len(frame.Locals) {
			return nil, fmt.Errorf("saved local %q is not a variable of the program", name)
		}
		locals[slot] = frame.Locals[index]
	}
	for _, internal := range frame.Rebuild.Internal {
		if internal.Slot < len(program.VarNames) || internal.Slot >= program.NumLocals {
			return nil, fmt.Errorf("saved internal local slot %d outside [%d, %d)", internal.Slot, len(program.VarNames), program.NumLocals)
		}
		locals[internal.Slot] = internal.Value
	}
	return locals, nil
}

func validateVMSnapshot(snapshot *task.VMSnapshot) error {
	if snapshot == nil || len(snapshot.Frames) == 0 {
		return fmt.Errorf("empty VM snapshot")
	}
	if snapshot.MaxStackDepth <= 0 || len(snapshot.Frames) > snapshot.MaxStackDepth {
		return fmt.Errorf("invalid maximum stack depth %d for %d frames", snapshot.MaxStackDepth, len(snapshot.Frames))
	}

	stackBase := 0
	for frameIndex := range snapshot.Frames {
		saved := &snapshot.Frames[frameIndex]
		heights, err := bytecode.StackHeights(&saved.Program)
		if err != nil {
			return fmt.Errorf("frame %d: %w", frameIndex, err)
		}
		if !saved.Program.IsInstructionBoundary(saved.IP) {
			return fmt.Errorf("frame %d: saved IP %d is not an instruction boundary", frameIndex, saved.IP)
		}
		if heights[saved.IP] < 0 {
			return fmt.Errorf("frame %d: saved IP %d is unreachable", frameIndex, saved.IP)
		}
		// A caller's saved IP follows its call, whose result the callee's
		// return pushes, if it pushes one, onto the saved stack.
		resumeHeight := len(saved.Stack)
		if frameIndex+1 < len(snapshot.Frames) && returnPushesResult(&snapshot.Frames[frameIndex+1]) {
			resumeHeight++
		}
		if heights[saved.IP] != resumeHeight {
			return fmt.Errorf("frame %d: stack height %d at saved IP %d, where the program has %d",
				frameIndex, resumeHeight, saved.IP, heights[saved.IP])
		}
		if len(saved.Locals) < saved.Program.NumLocals {
			return fmt.Errorf("frame %d: %d local slots for program requiring %d", frameIndex, len(saved.Locals), saved.Program.NumLocals)
		}
		if saved.ReturnLocal < 0 || (saved.ReturnLocal != 0 && (frameIndex == 0 || saved.ReturnLocal > len(snapshot.Frames[frameIndex-1].Locals))) {
			return fmt.Errorf("frame %d: invalid caller result local", frameIndex)
		}
		stackEnd := stackBase + len(saved.Stack)
		for index, pending := range saved.PendingReasons {
			if !saved.Program.IsInstructionBoundary(pending.FinallyIP) || !saved.Program.FinallyContains(pending.FinallyIP, saved.IP) {
				return fmt.Errorf("frame %d: pending cleanup owner does not contain saved IP", frameIndex)
			}
			if index != 0 && (saved.PendingReasons[index-1].FinallyIP == pending.FinallyIP || !saved.Program.FinallyContains(saved.PendingReasons[index-1].FinallyIP, pending.FinallyIP)) {
				return fmt.Errorf("frame %d: pending cleanup reasons are not nested", frameIndex)
			}
			if !pending.Error.ValidExceptionReason() {
				return fmt.Errorf("frame %d: invalid pending error reason", frameIndex)
			}
			switch pending.Kind {
			case task.PendingError:
				if !pending.Error.Present || !pending.Error.ValidExceptionReason() {
					return fmt.Errorf("frame %d: invalid pending error reason", frameIndex)
				}
			case task.PendingReturn:
				if pending.Error.Present {
					return fmt.Errorf("frame %d: return has pending error", frameIndex)
				}
			case task.PendingJump:
				if pending.Error.Present || pending.HandlerDepth < 0 || pending.HandlerDepth > len(saved.ExceptStack) || !saved.Program.IsInstructionBoundary(pending.TargetIP) || heights[pending.TargetIP] != len(pending.Stack) {
					return fmt.Errorf("frame %d: invalid pending loop transfer", frameIndex)
				}
			case task.PendingAbort:
				if pending.Error.Present {
					return fmt.Errorf("frame %d: abort has pending error", frameIndex)
				}
			default:
				return fmt.Errorf("frame %d: invalid pending cleanup reason kind", frameIndex)
			}
		}
		for handlerIndex, handler := range saved.ExceptStack {
			if handler.Type != bytecode.HandlerExcept && handler.Type != bytecode.HandlerFinally {
				return fmt.Errorf("frame %d handler %d: invalid handler type %d", frameIndex, handlerIndex, handler.Type)
			}
			if !saved.Program.IsInstructionBoundary(handler.HandlerIP) {
				return fmt.Errorf("frame %d handler %d: handler target %d is not an instruction boundary", frameIndex, handlerIndex, handler.HandlerIP)
			}
			if handler.EndIP != 0 && !saved.Program.IsInstructionBoundary(handler.EndIP) {
				return fmt.Errorf("frame %d handler %d: end target %d is not an instruction boundary", frameIndex, handlerIndex, handler.EndIP)
			}
			if handler.VarIndex < -1 || handler.VarIndex >= saved.Program.NumLocals {
				return fmt.Errorf("frame %d handler %d: local index %d outside program locals", frameIndex, handlerIndex, handler.VarIndex)
			}
			if handler.ExceptBase < 0 || handler.ExceptBase > handlerIndex+1 {
				return fmt.Errorf("frame %d handler %d: invalid handler group base", frameIndex, handlerIndex)
			}
			if handler.StackDepth < stackBase || handler.StackDepth > stackEnd {
				return fmt.Errorf("frame %d handler %d: stack depth %d outside [%d,%d]", frameIndex, handlerIndex, handler.StackDepth, stackBase, stackEnd)
			}
			// Unwinding sets SP to StackDepth and jumps to HandlerIP.
			if depth := handler.StackDepth - stackBase; depth != heights[handler.HandlerIP] {
				return fmt.Errorf("frame %d handler %d: stack depth %d, where handler %d has %d", frameIndex, handlerIndex, depth, handler.HandlerIP, heights[handler.HandlerIP])
			}
		}
		stackBase = stackEnd
	}
	return nil
}

// returnPushesResult reports whether callee's return pushes a value onto its
// caller's stack (vm/stack.go Return): an eval frame always pushes its
// wrapped result, a move or recycle continuation pushes the builtin's result
// or starts the next hook (whose return does), and any other frame pushes
// unless its caller discards the result.
func returnPushesResult(callee *task.VMFrameSnapshot) bool {
	return callee.IsEvalFrame || callee.MoveContinuation != nil || callee.RecycleContinuation != nil || !callee.DiscardReturn
}

func cloneMoveContinuation(state *task.MoveContinuationSnapshot) *task.MoveContinuationSnapshot {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

func clonePendingReasons(pending []pendingReason) []pendingReason {
	if pending == nil {
		return nil
	}
	copy := append([]pendingReason(nil), pending...)
	for index := range copy {
		copy[index].Stack = append([]types.Value(nil), pending[index].Stack...)
		if original, ok := pending[index].Error.(*VMException); ok {
			cloned := *original
			copy[index].Error = &cloned
		}
	}
	return copy
}

func snapshotPendingReasons(pending []pendingReason) []task.PendingReasonSnapshot {
	result := make([]task.PendingReasonSnapshot, len(pending))
	for index, reason := range pending {
		result[index] = task.PendingReasonSnapshot{Kind: reason.Kind, FinallyIP: reason.FinallyIP, Value: reason.Value, TargetIP: reason.TargetIP, HandlerDepth: reason.HandlerDepth, Stack: append([]types.Value(nil), reason.Stack...)}
		if reason.Error != nil {
			encoded := &result[index].Error
			encoded.Present, encoded.Reason, encoded.Code = true, exceptionReason(reason.Error), errorCode(reason.Error)
			switch value := reason.Error.(type) {
			case VMException:
				encoded.Value = value.Value
			case *VMException:
				encoded.Value = value.Value
			}
		}
	}
	return result
}

func restorePendingReasons(pending []task.PendingReasonSnapshot) []pendingReason {
	result := make([]pendingReason, len(pending))
	for index, reason := range pending {
		result[index] = pendingReason{Kind: reason.Kind, FinallyIP: reason.FinallyIP, Value: reason.Value, TargetIP: reason.TargetIP, HandlerDepth: reason.HandlerDepth, Stack: append([]types.Value(nil), reason.Stack...)}
		if reason.Error.Present {
			result[index].Error = VMException{Code: reason.Error.Code, Value: reason.Error.Value, Reason: reason.Error.Reason}
		}
	}
	return result
}

func snapshotRecycleContinuation(state *recycleContinuation) *task.RecycleContinuationSnapshot {
	if state == nil {
		return nil
	}
	request := state.request
	return &task.RecycleContinuationSnapshot{
		ExitCallback: state.exitCallback,
		Object:       request.Object,
		OldParents:   append([]types.ObjID(nil), request.OldParents...),
		OldChildren:  append([]types.ObjID(nil), request.OldChildren...),
		OldContents:  append([]types.ObjID(nil), request.OldContents...),
		OldLocation:  request.OldLocation,
	}
}

func restoreRecycleContinuation(state *task.RecycleContinuationSnapshot) *recycleContinuation {
	if state == nil {
		return nil
	}
	return &recycleContinuation{exitCallback: state.ExitCallback, request: builtins.RecycleLifecycleRequest{
		Object:      state.Object,
		OldParents:  append([]types.ObjID(nil), state.OldParents...),
		OldChildren: append([]types.ObjID(nil), state.OldChildren...),
		OldContents: append([]types.ObjID(nil), state.OldContents...),
		OldLocation: state.OldLocation,
	}}
}

func cloneProgram(program *bytecode.Program) bytecode.Program {
	if program == nil {
		return bytecode.Program{}
	}
	slots := program.BuiltinSlots
	slots.TypeConstants = append([]bytecode.TypeConstantSlot(nil), slots.TypeConstants...)
	return bytecode.Program{
		Code:          append([]byte(nil), program.Code...),
		Constants:     append([]types.Value(nil), program.Constants...),
		VarNames:      append([]string(nil), program.VarNames...),
		LineInfo:      append([]bytecode.LineEntry(nil), program.LineInfo...),
		NumLocals:     program.NumLocals,
		Source:        append([]string(nil), program.Source...),
		BuiltinSlots:  slots,
		BuiltinLayout: program.BuiltinLayout,
		Origin:        program.Origin,
		// Immutable once the compiler fills it, like Constants' values.
		BuiltinPropertyConstants: program.BuiltinPropertyConstants,
		ForkSources:              program.ForkSources,
	}
}

func cloneHandlers(handlers []bytecode.Handler) []bytecode.Handler {
	if len(handlers) == 0 {
		return nil
	}
	cloned := make([]bytecode.Handler, len(handlers))
	for i, handler := range handlers {
		cloned[i] = handler
		cloned[i].Codes = append([]types.ErrorCode(nil), handler.Codes...)
	}
	return cloned
}
