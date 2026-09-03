package vm

import (
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
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
			PendingReturn:       frame.PendingReturn,
			HasPendingReturn:    frame.HasPendingReturn,
			VerbDebug:           frame.VerbDebug,
			DiscardReturn:       frame.DiscardReturn,
			IsVerbCall:          frame.IsVerbCall,
			IsEvalFrame:         frame.IsEvalFrame,
			SavedThisObj:        frame.SavedThisObj,
			SavedThisValue:      frame.SavedThisValue,
			SavedVerb:           frame.SavedVerb,
			SavedProgrammer:     frame.SavedProgrammer,
			SavedIsWizard:       frame.SavedIsWizard,
			MoveContinuation:    cloneMoveContinuation(frame.MoveContinuation),
			RecycleContinuation: snapshotRecycleContinuation(frame.RecycleContinuation),
		}
		if frame.PendingError != nil {
			saved.PendingError.Present = true
			switch pending := frame.PendingError.(type) {
			case VMException:
				saved.PendingError.Code = pending.Code
				saved.PendingError.Value = pending.Value
			case MooError:
				saved.PendingError.Code = pending.Code
			default:
				saved.PendingError.Code = extractErrorCode(frame.PendingError)
			}
		}
		snapshot.Frames = append(snapshot.Frames, saved)
	}
	return snapshot
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
	ctx *kernel.TaskContext,
) (*VM, error) {
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
		program := cloneProgram(&saved.Program)
		base := len(machine.Stack)
		machine.Stack = append(machine.Stack, saved.Stack...)
		recycleContinuation := restoreRecycleContinuation(saved.RecycleContinuation)
		frame := &StackFrame{
			Program:             &program,
			IP:                  saved.IP,
			BasePointer:         base,
			Locals:              append([]types.Value(nil), saved.Locals...),
			This:                saved.This,
			ThisValue:           saved.ThisValue,
			Player:              saved.Player,
			Verb:                saved.Verb,
			StoredVerb:          saved.StoredVerb,
			Caller:              saved.Caller,
			VerbLoc:             saved.VerbLoc,
			Args:                append([]types.Value(nil), saved.Args...),
			ExceptStack:         cloneHandlers(saved.ExceptStack),
			PendingReturn:       saved.PendingReturn,
			HasPendingReturn:    saved.HasPendingReturn,
			VerbDebug:           saved.VerbDebug,
			DiscardReturn:       saved.DiscardReturn,
			IsVerbCall:          saved.IsVerbCall,
			IsEvalFrame:         saved.IsEvalFrame,
			SavedThisObj:        saved.SavedThisObj,
			SavedThisValue:      saved.SavedThisValue,
			SavedVerb:           saved.SavedVerb,
			SavedProgrammer:     saved.SavedProgrammer,
			SavedIsWizard:       saved.SavedIsWizard,
			MoveContinuation:    cloneMoveContinuation(saved.MoveContinuation),
			RecycleContinuation: recycleContinuation,
		}
		if saved.PendingError.Present {
			frame.PendingError = VMException{
				Code:  saved.PendingError.Code,
				Value: saved.PendingError.Value,
			}
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

	machine.SP = len(machine.Stack)
	machine.FP = len(machine.Frames) - 1
	machine.frame = machine.Frames[len(machine.Frames)-1]
	machine.yielded = true
	machine.yieldResult = types.Result{Flow: types.FlowSuspend}
	return machine, nil
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
		if err := bytecode.VerifyProgram(&saved.Program); err != nil {
			return fmt.Errorf("frame %d: %w", frameIndex, err)
		}
		if !saved.Program.IsInstructionBoundary(saved.IP) {
			return fmt.Errorf("frame %d: saved IP %d is not an instruction boundary", frameIndex, saved.IP)
		}
		if len(saved.Locals) < saved.Program.NumLocals {
			return fmt.Errorf("frame %d: %d local slots for program requiring %d", frameIndex, len(saved.Locals), saved.Program.NumLocals)
		}
		stackEnd := stackBase + len(saved.Stack)
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
			if handler.StackDepth < stackBase || handler.StackDepth > stackEnd {
				return fmt.Errorf("frame %d handler %d: stack depth %d outside [%d,%d]", frameIndex, handlerIndex, handler.StackDepth, stackBase, stackEnd)
			}
		}
		stackBase = stackEnd
	}
	return nil
}

func cloneMoveContinuation(state *task.MoveContinuationSnapshot) *task.MoveContinuationSnapshot {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

func snapshotRecycleContinuation(state *recycleContinuation) *task.RecycleContinuationSnapshot {
	if state == nil {
		return nil
	}
	request := state.request
	return &task.RecycleContinuationSnapshot{
		Object:      request.Object,
		OldParents:  append([]types.ObjID(nil), request.OldParents...),
		OldChildren: append([]types.ObjID(nil), request.OldChildren...),
		OldContents: append([]types.ObjID(nil), request.OldContents...),
		OldLocation: request.OldLocation,
	}
}

func restoreRecycleContinuation(state *task.RecycleContinuationSnapshot) *recycleContinuation {
	if state == nil {
		return nil
	}
	return &recycleContinuation{request: builtins.RecycleLifecycleRequest{
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
	return bytecode.Program{
		Code:         append([]byte(nil), program.Code...),
		Constants:    append([]types.Value(nil), program.Constants...),
		VarNames:     append([]string(nil), program.VarNames...),
		LineInfo:     append([]bytecode.LineEntry(nil), program.LineInfo...),
		NumLocals:    program.NumLocals,
		Source:       append([]string(nil), program.Source...),
		BuiltinSlots: program.BuiltinSlots,
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
