package vm

import (
	"barn/types"
)

// Logical operations

func (vm *VM) executeNot() error {
	a := vm.Pop()
	if !a.Truthy() {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeAnd() error {
	// Short-circuit AND
	// Stack has left value, offset is in bytecode
	offset := vm.ReadShort()
	val := vm.Peek(0)

	if !val.Truthy() {
		// Left is false, skip right and keep left value
		vm.CurrentFrame().IP += int(offset)
	} else {
		// Left is true, pop it and evaluate right
		vm.Pop()
	}

	return nil
}

func (vm *VM) executeOr() error {
	// Short-circuit OR
	// Stack has left value, offset is in bytecode
	offset := vm.ReadShort()
	val := vm.Peek(0)

	if val.Truthy() {
		// Left is true, skip right and keep left value
		vm.CurrentFrame().IP += int(offset)
	} else {
		// Left is false, pop it and evaluate right
		vm.Pop()
	}

	return nil
}
