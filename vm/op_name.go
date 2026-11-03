package vm

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

func (vm *VM) staticNameFromConstant(index int, kind string) (string, error) {
	return staticNameAt(vm.CurrentFrame().Program, index, kind)
}

// staticPropertyName is staticNameFromConstant for a property name, with the
// compiler's classification of it as a built-in property name or not.
func (vm *VM) staticPropertyName(index int) (name string, builtin bool, err error) {
	program := vm.CurrentFrame().Program
	name, err = staticNameAt(program, index, "property")
	if err != nil {
		return "", false, err
	}
	return name, program.IsBuiltinPropertyConstant(index), nil
}

func staticNameAt(program *bytecode.Program, index int, kind string) (string, error) {
	constants := program.Constants
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
		return "", newMooErrorf(types.E_TYPE, "dynamic %s name must be a string", kind)
	}
	return name.Str(), nil
}
