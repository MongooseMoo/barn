package vm

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
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

// clearDeadStackSlots releases references above the stack pointer. Pop stays
// minimal because it is on the interpreter's hot path; clearing once when the
// VM yields prevents a suspended task from retaining its stack high-water mark.
func (vm *VM) clearDeadStackSlots() {
	clear(vm.Stack[vm.SP:])
}

// FetchByte reads a byte from the current instruction stream.
func (vm *VM) FetchByte() byte {
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

// ReadWide reads a 4-byte unsigned control-flow operand from the current
// instruction stream.
func (vm *VM) ReadWide() uint32 {
	frame := vm.CurrentFrame()
	value := uint32(frame.Program.Code[frame.IP])<<24 |
		uint32(frame.Program.Code[frame.IP+1])<<16 |
		uint32(frame.Program.Code[frame.IP+2])<<8 |
		uint32(frame.Program.Code[frame.IP+3])
	frame.IP += 4
	return value
}

func (vm *VM) readControlFlowOperand(wide bool) int {
	if wide {
		return int(vm.ReadWide())
	}
	return int(vm.ReadShort())
}

// Return returns from the current frame
func (vm *VM) Return(value types.Value) error {
	if len(vm.Frames) == 0 {
		return nil
	}

	frame := vm.Frames[len(vm.Frames)-1]
	for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
		handler := frame.ExceptStack[i]
		if handler.Type != bytecode.HandlerFinally {
			continue
		}
		frame.ExceptStack = frame.ExceptStack[:i]
		frame.PendingReturn = value
		frame.HasPendingReturn = true
		vm.SP = handler.StackDepth
		frame.IP = handler.HandlerIP
		return nil
	}
	frame.HasPendingReturn = false
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
		if vm.Task != nil {
			vm.Task.PopFrame()
		}
		vm.SP = frame.BasePointer
		vm.popFrame()
		vm.Push(wrapped)
		return nil
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
		if vm.Task != nil {
			vm.Task.PopFrame()
		}
	}

	continuation := frame.MoveContinuation
	recycleContinuation := frame.RecycleContinuation
	vm.SP = frame.BasePointer
	vm.popFrame()
	if continuation != nil {
		result := vm.resumeMoveLifecycle(continuation, types.Ok(value))
		switch result.Flow {
		case types.FlowException:
			return VMException{Code: result.Error, Value: result.Val}
		case types.FlowBuiltinPush:
			return nil
		case types.FlowNormal, types.FlowReturn:
			vm.Push(result.Val)
			return nil
		default:
			return fmt.Errorf("unexpected move continuation flow %d", result.Flow)
		}
	}
	if recycleContinuation != nil {
		result := vm.resumeRecycleLifecycle(recycleContinuation, types.Ok(value))
		switch result.Flow {
		case types.FlowException:
			return VMException{Code: result.Error, Value: result.Val}
		case types.FlowBuiltinPush:
			return nil
		case types.FlowNormal, types.FlowReturn:
			vm.Push(result.Val)
			return nil
		default:
			return fmt.Errorf("unexpected recycle continuation flow %d", result.Flow)
		}
	}
	if !frame.DiscardReturn {
		vm.Push(value)
	}
	return nil
}
