package vm

import (
	"fmt"

	"barn/types"
)

func (vm *VM) staticNameFromConstant(index int, kind string) (string, error) {
	constants := vm.CurrentFrame().Program.Constants
	if index < 0 || index >= len(constants) {
		return "", fmt.Errorf("internal error: %s name constant index %d out of range", kind, index)
	}
	name := constants[index]
	if name.Type() != types.TYPE_STR {
		return "", fmt.Errorf("internal error: %s name constant is not a string", kind)
	}
	return name.Str(), nil
}

func (vm *VM) popDynamicName(kind string) (string, error) {
	name := vm.Pop()
	if name.Type() != types.TYPE_STR {
		return "", fmt.Errorf("E_TYPE: dynamic %s name must be a string", kind)
	}
	return name.Str(), nil
}
