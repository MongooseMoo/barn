package vm

import (
	"barn/bytecode"
	"barn/task"
	"barn/types"
)

// buildTraceback returns a MOO list of stack frames suitable for the 4th
// element of a caught exception value.  Frames are ordered innermost-first
// (the verb where the error occurred comes first).  Eval infrastructure below
// the eval'd-code activation (the bf_eval marker and the verb that called
// eval()) is always excluded.  The eval'd-code activation itself is included
// only when includeEvalFrame is set — i.e. when the error is caught at, or
// unwinds to, the eval boundary rather than being caught by a verb above it
// (matching ToastStunt).
func (vm *VM) buildTraceback(includeEvalFrame bool) types.Value {
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
			if includeEvalFrame {
				frames = append(frames, f.ToList())
			}
			// Stop at the eval boundary either way — frames below are eval
			// infrastructure, not part of the in-eval traceback.
			break
		}
		if f.ServerInitiated {
			continue
		}
		frames = append(frames, f.ToList())
	}
	return types.NewList(frames)
}

// matchingExceptAboveEvalFrame reports whether some frame strictly above the
// nearest eval frame has an except handler that matches errCode. When true, that
// verb catches the error before it reaches the eval boundary, so the eval'd-code
// activation is not part of the traceback. Finally handlers are ignored: they run
// and re-raise, so they do not stop propagation toward the eval frame.
func (vm *VM) matchingExceptAboveEvalFrame(errCode types.ErrorCode) bool {
	for i := len(vm.Frames) - 1; i >= 0; i-- {
		frame := vm.Frames[i]
		if frame.IsEvalFrame {
			return false
		}
		for _, h := range frame.ExceptStack {
			if h.Type == bytecode.HandlerExcept && h.Matches(errCode) {
				return true
			}
		}
	}
	return false
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
			This:        frame.This,
			ThisValue:   types.None,
			Player:      frame.Player,
			Programmer:  types.ObjNothing,
			Caller:      frame.Caller,
			Verb:        frame.Verb,
			StoredVerb:  frame.StoredVerb,
			VerbLoc:     frame.VerbLoc,
			Args:        frame.Args,
			LineNumber:  line,
			SourceLine:  vm.sourceLineForFrame(frame, line),
			IsEvalFrame: frame.IsEvalFrame,
		})
	}

	return stack
}
