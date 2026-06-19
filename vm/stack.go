package vm

import (
	"barn/task"
	"barn/trace"
	"barn/types"
)

// Push pushes a value onto the stack
func (vm *VM) Push(v types.Value) {
	if vm.SP >= len(vm.Stack) {
		vm.Stack = append(vm.Stack, v)
	} else {
		vm.Stack[vm.SP] = v
	}
	vm.SP++
}

// Pop pops a value from the stack
func (vm *VM) Pop() types.Value {
	if vm.SP == 0 {
		panic("stack underflow")
	}
	vm.SP--
	return vm.Stack[vm.SP]
}

// Peek peeks at a value on the stack (0 = top)
func (vm *VM) Peek(offset int) types.Value {
	if vm.SP-1-offset < 0 {
		panic("stack underflow")
	}
	return vm.Stack[vm.SP-1-offset]
}

// PopN pops N values from the stack
func (vm *VM) PopN(n int) []types.Value {
	if vm.SP < n {
		panic("stack underflow")
	}
	values := make([]types.Value, n)
	for i := n - 1; i >= 0; i-- {
		values[i] = vm.Pop()
	}
	return values
}

// ReadByte reads a byte from the current instruction stream
func (vm *VM) ReadByte() byte {
	frame := vm.CurrentFrame()
	b := frame.Program.Code[frame.IP]
	frame.IP++
	return b
}

// ReadShort reads a 2-byte short from the current instruction stream
func (vm *VM) ReadShort() uint16 {
	frame := vm.CurrentFrame()
	hi := frame.Program.Code[frame.IP]
	lo := frame.Program.Code[frame.IP+1]
	frame.IP += 2
	return uint16(hi)<<8 | uint16(lo)
}

// Return returns from the current frame
func (vm *VM) Return(value types.Value) {
	if len(vm.Frames) == 0 {
		return
	}

	frame := vm.Frames[len(vm.Frames)-1]
	vm.collectPendingWaifsFromFrame(frame)

	// Eval frame returning normally: wrap result in {1, value}
	if frame.IsEvalFrame {
		wrapped := types.NewList([]types.Value{
			types.NewInt(1),
			value,
		})
		// Restore context
		if vm.Context != nil {
			vm.Context.ThisObj = frame.SavedThisObj
			vm.Context.ThisValue = frame.SavedThisValue
			vm.Context.Verb = frame.SavedVerb
			vm.Context.Programmer = frame.SavedProgrammer
			vm.Context.IsWizard = frame.SavedIsWizard
		}
		// Pop activation frame from task call stack
		if vm.Context != nil && vm.Context.Task != nil {
			if t, ok := vm.Context.Task.(*task.Task); ok {
				t.PopFrame()
			}
		}
		vm.SP = frame.BasePointer
		vm.popFrame()
		vm.Push(wrapped)
		return
	}

	// If this was a verb-call frame, restore context and pop activation frame
	if frame.IsVerbCall && vm.Context != nil {
		trace.VerbReturn(frame.This, frame.Verb, value)
		vm.Context.ThisObj = frame.SavedThisObj
		vm.Context.ThisValue = frame.SavedThisValue
		vm.Context.Verb = frame.SavedVerb
		vm.Context.Programmer = frame.SavedProgrammer
		vm.Context.IsWizard = frame.SavedIsWizard

		// Pop activation frame from task call stack
		if vm.Context.Task != nil {
			if t, ok := vm.Context.Task.(*task.Task); ok {
				t.PopFrame()
			}
		}
	}

	vm.SP = frame.BasePointer
	vm.popFrame()
	vm.Push(value)
}
