package vm

import (
	"fmt"
	"github.com/MongooseMoo/barn/types"
)

// Bitwise operations

func (vm *VM) executeBitOr() error {
	b := vm.Pop()
	a := vm.Pop()

	if a.Type() != types.TYPE_INT || b.Type() != types.TYPE_INT {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(a.Int() | b.Int()))
	return nil
}

func (vm *VM) executeBitAnd() error {
	b := vm.Pop()
	a := vm.Pop()

	if a.Type() != types.TYPE_INT || b.Type() != types.TYPE_INT {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(a.Int() & b.Int()))
	return nil
}

func (vm *VM) executeBitXor() error {
	b := vm.Pop()
	a := vm.Pop()

	if a.Type() != types.TYPE_INT || b.Type() != types.TYPE_INT {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(a.Int() ^ b.Int()))
	return nil
}

func (vm *VM) executeBitNot() error {
	a := vm.Pop()

	if a.Type() != types.TYPE_INT {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(^a.Int()))
	return nil
}

func (vm *VM) executeShl() error {
	b := vm.Pop()
	a := vm.Pop()

	if a.Type() != types.TYPE_INT || b.Type() != types.TYPE_INT {
		return fmt.Errorf("E_TYPE: shift operations require integers")
	}
	bVal := b.Int()

	if bVal < 0 {
		return fmt.Errorf("E_INVARG: negative shift count")
	}
	if bVal == 64 {
		vm.Push(types.NewInt(0))
		return nil
	}
	if bVal > 64 {
		return fmt.Errorf("E_INVARG: invalid shift count")
	}

	vm.Push(types.NewInt(a.Int() << uint(bVal)))
	return nil
}

func (vm *VM) executeShr() error {
	b := vm.Pop()
	a := vm.Pop()

	if a.Type() != types.TYPE_INT || b.Type() != types.TYPE_INT {
		return fmt.Errorf("E_TYPE: shift operations require integers")
	}
	bVal := b.Int()

	if bVal < 0 {
		return fmt.Errorf("E_INVARG: negative shift count")
	}
	if bVal == 64 {
		vm.Push(types.NewInt(0))
		return nil
	}
	if bVal > 64 {
		return fmt.Errorf("E_INVARG: invalid shift count")
	}

	// Use unsigned cast for logical right shift (zero-fill, not sign-extending)
	result := int64(uint64(a.Int()) >> uint(bVal))
	vm.Push(types.NewInt(result))
	return nil
}
