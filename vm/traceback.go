package vm

import (
	"strings"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
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
	if vm.Context == nil || vm.Task == nil {
		return types.NewList([]types.Value{})
	}
	includeVariables := vm.Builtins != nil && vm.Builtins.IncludeRTVars(vm.Context)
	if includeVariables {
		vm.snapshotTaskRuntimeVariables()
	}
	stack := types.ExpandBuiltinFrames(vm.Task.GetCallStack())
	frames := make([]types.Value, 0, len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		f := stack[i]
		if f.IsEvalFrame && !includeEvalFrame {
			break
		}
		if f.ServerInitiated && !f.IsEvalFrame {
			continue
		}
		if f.ThisValue.Type() == types.TYPE_ANON {
			f.ThisValue = types.NewAnon(types.ObjNothing)
		}
		frame := f.ToList()
		if includeVariables {
			values := append(frame.Elements(), f.RuntimeVariableMap())
			frame = types.NewList(values)
		}
		frames = append(frames, frame)
		if f.IsEvalFrame {
			break
		}
	}
	return types.NewList(frames)
}

// activationFramesForException restores the original stack held across a
// finally block. The live activations have already unwound when it re-raises.
func (vm *VM) activationFramesForException(value types.Value) []types.ActivationFrame {
	if value.Type() != types.TYPE_LIST || value.Len() != 4 || value.Get(4).Type() != types.TYPE_LIST {
		return nil
	}
	traceback := value.Get(4)
	stack := make([]types.ActivationFrame, 0, traceback.Len())
	for i := traceback.Len(); i > 0; i-- {
		entry := traceback.Get(i)
		if entry.Type() != types.TYPE_LIST || entry.Len() < 6 {
			return nil
		}
		frame := types.ActivationFrame{
			This: types.ObjNothing, ThisValue: entry.Get(1),
			Verb: entry.Get(2).Str(), Programmer: entry.Get(3).Obj(),
			VerbLoc: entry.Get(4).Obj(), Player: entry.Get(5).Obj(),
			LineNumber: int(entry.Get(6).Int()),
		}
		if frame.ThisValue.Type() == types.TYPE_OBJ {
			frame.This = frame.ThisValue.Obj()
			frame.ThisValue = types.None
		}
		if entry.Len() >= 7 {
			frame.RuntimeVariables = entry.Get(7)
		}
		if vm.Context != nil && vm.Context.StoreTxn != nil {
			if verb, _, err := vm.Context.StoreTxn.FindVerb(frame.VerbLoc, frame.Verb); err == nil {
				frame.StoredVerbNames = verb.Names
				if frame.LineNumber > 0 && frame.LineNumber <= len(verb.Code) {
					frame.SourceLine = strings.TrimSpace(verb.Code[frame.LineNumber-1])
				}
			}
		}
		stack = append(stack, frame)
	}
	return stack
}

// limitExceptionTraceback excludes activations outside the nearest matching
// except handler. Finally handlers run without ending error propagation.
func (vm *VM) limitExceptionTraceback(traceback types.Value, code types.ErrorCode) types.Value {
	depth := 0
	for i := len(vm.Frames) - 1; i >= 0; i-- {
		frame := vm.Frames[i]
		depth++
		for j := len(frame.ExceptStack) - 1; j >= 0; j-- {
			handler := frame.ExceptStack[j]
			if handler.Type == bytecode.HandlerExcept && handler.Matches(code) {
				if depth < traceback.Len() {
					return types.NewList(traceback.Elements()[:depth])
				}
				return traceback
			}
		}
		if frame.IsEvalFrame {
			break
		}
	}
	return traceback
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
func (vm *VM) snapshotActivationFrames(topLine int) []types.ActivationFrame {
	if len(vm.Frames) == 0 {
		return nil
	}

	includeVariables := vm.Builtins != nil && vm.Builtins.IncludeRTVars(vm.Context)
	var taskStack []types.ActivationFrame
	if vm.Task != nil {
		taskStack = vm.Task.GetCallStack()
	}

	stack := make([]types.ActivationFrame, 0, len(vm.Frames))
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

		programmer := types.ObjNothing
		serverInitiated := false
		if i < len(taskStack) {
			programmer = taskStack[i].Programmer
			serverInitiated = taskStack[i].ServerInitiated
		}

		var runtimeVariables *types.RuntimeVariableSnapshot
		if includeVariables && frame.Program != nil {
			runtimeVariables = &types.RuntimeVariableSnapshot{
				Names:  frame.Program.VarNames,
				Values: append([]types.Value(nil), frame.Locals...),
			}
		}
		stack = append(stack, types.ActivationFrame{
			This:                    frame.This,
			ThisValue:               frame.ThisValue,
			Player:                  frame.Player,
			Programmer:              programmer,
			Caller:                  frame.Caller,
			Verb:                    frame.Verb,
			StoredVerb:              frame.StoredVerb,
			StoredVerbNames:         frame.StoredVerbNames,
			VerbLoc:                 frame.VerbLoc,
			Args:                    frame.Args,
			LineNumber:              line,
			SourceLine:              vm.sourceLineForFrame(frame, line),
			ServerInitiated:         serverInitiated,
			IsEvalFrame:             frame.IsEvalFrame,
			RuntimeVariableSnapshot: runtimeVariables,
			BuiltinCaller:           frame.BuiltinCaller,
			BuiltinPC:               frame.BuiltinPC,
		})
	}

	return types.ExpandBuiltinFrames(stack)
}
