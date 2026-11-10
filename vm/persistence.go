package vm

import (
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
		if recycleContinuation != nil {
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
		frame, call, err := session.Registry().Compiler().ConvertNativeFrame(saved, vector, index == len(snapshot.Frames)-1)
		if err != nil {
			return nil, fmt.Errorf("native frame %d: %w", index, err)
		}
		frame.IsVerbCall = previousCall == compiler.NativeResumeVerb || previousCall == compiler.NativeResumePass
		frame.IsEvalFrame = previousCall == compiler.NativeResumeEval
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
