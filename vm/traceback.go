package vm

import (
	"barn/task"
	"barn/types"
)

// buildTraceback returns a MOO list of stack frames suitable for the 4th
// element of a caught exception value.  Frames are ordered innermost-first
// (the verb where the error occurred comes first).  Only real verb frames
// are included — eval infrastructure is excluded.
func (vm *VM) buildTraceback() types.Value {
	if vm.Context == nil || vm.Context.Task == nil {
		return types.NewList([]types.Value{})
	}
	t, ok := vm.Context.Task.(*task.Task)
	if !ok {
		return types.NewList([]types.Value{})
	}

	stack := t.GetCallStack()
	frames := make([]types.Value, 0, len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		f := stack[i]
		if f.IsEvalFrame {
			// Stop at eval boundary — don't include eval infrastructure
			// or the verb that called eval() (frames below this point)
			break
		}
		if f.ServerInitiated {
			continue
		}
		frames = append(frames, f.ToList())
	}
	return types.NewList(frames)
}

// snapshotActivationFrames captures the current VM call chain as activation
// frames for traceback formatting.
func (vm *VM) snapshotActivationFrames(topLine int) []task.ActivationFrame {
	if len(vm.Frames) == 0 {
		return nil
	}

	stack := make([]task.ActivationFrame, 0, len(vm.Frames))
	for i, frame := range vm.Frames {
		line := 1
		if i == len(vm.Frames)-1 {
			line = topLine
		} else if frame.Program != nil {
			// For caller frames, IP points at the next instruction to execute.
			// Use IP-1 so traceback lines point at the call site that led here.
			ip := frame.IP - 1
			if ip < 0 {
				ip = 0
			}
			line = frame.Program.LineForIP(ip)
		}

		stack = append(stack, task.ActivationFrame{
			This:       frame.This,
			ThisValue:  nil,
			Player:     frame.Player,
			Programmer: types.ObjNothing,
			Caller:     frame.Caller,
			Verb:       frame.Verb,
			VerbLoc:    frame.VerbLoc,
			Args:       frame.Args,
			LineNumber: line,
			SourceLine: vm.sourceLineForFrame(frame, line),
		})
	}

	return stack
}
