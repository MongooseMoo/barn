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
			Program:         cloneProgram(frame.Program),
			IP:              frame.IP,
			Locals:          append([]types.Value(nil), frame.Locals...),
			Stack:           append([]types.Value(nil), vm.Stack[frame.BasePointer:stackEnd]...),
			This:            frame.This,
			ThisValue:       frame.ThisValue,
			Player:          frame.Player,
			Verb:            frame.Verb,
			StoredVerb:      storedVerbName(frame),
			Caller:          frame.Caller,
			VerbLoc:         frame.VerbLoc,
			Args:            append([]types.Value(nil), frame.Args...),
			ExceptStack:     cloneHandlers(frame.ExceptStack),
			VerbDebug:       frame.VerbDebug,
			DiscardReturn:   frame.DiscardReturn,
			IsVerbCall:      frame.IsVerbCall,
			IsEvalFrame:     frame.IsEvalFrame,
			SavedThisObj:    frame.SavedThisObj,
			SavedThisValue:  frame.SavedThisValue,
			SavedVerb:       frame.SavedVerb,
			SavedProgrammer: frame.SavedProgrammer,
			SavedIsWizard:   frame.SavedIsWizard,
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
	registry *builtins.Registry,
	ctx *kernel.TaskContext,
) (*VM, error) {
	if snapshot == nil || len(snapshot.Frames) == 0 {
		return nil, fmt.Errorf("empty VM snapshot")
	}

	machine := NewVM(store, registry)
	machine.Context = ctx
	if snapshot.MaxStackDepth > 0 {
		machine.MaxStackDepth = snapshot.MaxStackDepth
	}
	machine.Frames = make([]*StackFrame, 0, len(snapshot.Frames))
	machine.Stack = nil

	for _, saved := range snapshot.Frames {
		program := cloneProgram(&saved.Program)
		if saved.IP < 0 || saved.IP > len(program.Code) {
			return nil, fmt.Errorf("saved IP %d outside program of %d bytes", saved.IP, len(program.Code))
		}
		base := len(machine.Stack)
		machine.Stack = append(machine.Stack, saved.Stack...)
		frame := &StackFrame{
			Program:         &program,
			IP:              saved.IP,
			BasePointer:     base,
			Locals:          append([]types.Value(nil), saved.Locals...),
			This:            saved.This,
			ThisValue:       saved.ThisValue,
			Player:          saved.Player,
			Verb:            saved.Verb,
			StoredVerb:      saved.StoredVerb,
			Caller:          saved.Caller,
			VerbLoc:         saved.VerbLoc,
			Args:            append([]types.Value(nil), saved.Args...),
			ExceptStack:     cloneHandlers(saved.ExceptStack),
			VerbDebug:       saved.VerbDebug,
			DiscardReturn:   saved.DiscardReturn,
			IsVerbCall:      saved.IsVerbCall,
			IsEvalFrame:     saved.IsEvalFrame,
			SavedThisObj:    saved.SavedThisObj,
			SavedThisValue:  saved.SavedThisValue,
			SavedVerb:       saved.SavedVerb,
			SavedProgrammer: saved.SavedProgrammer,
			SavedIsWizard:   saved.SavedIsWizard,
		}
		if saved.PendingError.Present {
			frame.PendingError = VMException{
				Code:  saved.PendingError.Code,
				Value: saved.PendingError.Value,
			}
		}
		machine.Frames = append(machine.Frames, frame)
	}

	machine.SP = len(machine.Stack)
	machine.FP = len(machine.Frames) - 1
	machine.frame = machine.Frames[len(machine.Frames)-1]
	machine.yielded = true
	machine.yieldResult = types.Result{Flow: types.FlowSuspend}
	return machine, nil
}

func cloneProgram(program *bytecode.Program) bytecode.Program {
	if program == nil {
		return bytecode.Program{}
	}
	return bytecode.Program{
		Code:      append([]byte(nil), program.Code...),
		Constants: append([]types.Value(nil), program.Constants...),
		VarNames:  append([]string(nil), program.VarNames...),
		LineInfo:  append([]bytecode.LineEntry(nil), program.LineInfo...),
		NumLocals: program.NumLocals,
		Source:    append([]string(nil), program.Source...),
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
