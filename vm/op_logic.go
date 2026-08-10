package vm

import (
	"github.com/MongooseMoo/barn/types"
)

// Logical operations

func (vm *VM) executeNot() error {
	a := vm.Pop()
	if !a.Truthy() {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeAnd(wide bool) error {
	// Short-circuit AND
	// Stack has left value, offset is in bytecode
	offset := vm.readControlFlowOperand(wide)
	val := vm.Peek(0)

	if !val.Truthy() {
		// Left is false, skip right and keep left value
		vm.CurrentFrame().IP += offset
	} else {
		// Left is true, pop it and evaluate right
		vm.Pop()
	}

	return nil
}

func (vm *VM) executeOr(wide bool) error {
	// Short-circuit OR
	// Stack has left value, offset is in bytecode
	offset := vm.readControlFlowOperand(wide)
	val := vm.Peek(0)

	if val.Truthy() {
		// Left is true, skip right and keep left value
		vm.CurrentFrame().IP += offset
	} else {
		// Left is false, pop it and evaluate right
		vm.Pop()
	}

	return nil
}
