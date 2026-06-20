package vm

import (
	"barn/types"
	"fmt"
)

// Bitwise operations

func (vm *VM) executeBitOr() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.AsInt()
	bInt, bOk := b.AsInt()

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(aInt | bInt))
	return nil
}

func (vm *VM) executeBitAnd() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.AsInt()
	bInt, bOk := b.AsInt()

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(aInt & bInt))
	return nil
}

func (vm *VM) executeBitXor() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.AsInt()
	bInt, bOk := b.AsInt()

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(aInt ^ bInt))
	return nil
}

func (vm *VM) executeBitNot() error {
	a := vm.Pop()

	aInt, ok := a.AsInt()
	if !ok {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.NewInt(^aInt))
	return nil
}

func (vm *VM) executeShl() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.AsInt()
	bInt, bOk := b.AsInt()

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: shift operations require integers")
	}

	if bInt < 0 {
		return fmt.Errorf("E_INVARG: negative shift count")
	}
	if bInt == 64 {
		vm.Push(types.NewInt(0))
		return nil
	}
	if bInt > 64 {
		return fmt.Errorf("E_INVARG: invalid shift count")
	}

	vm.Push(types.NewInt(aInt << uint(bInt)))
	return nil
}

func (vm *VM) executeShr() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.AsInt()
	bInt, bOk := b.AsInt()

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: shift operations require integers")
	}

	if bInt < 0 {
		return fmt.Errorf("E_INVARG: negative shift count")
	}
	if bInt == 64 {
		vm.Push(types.NewInt(0))
		return nil
	}
	if bInt > 64 {
		return fmt.Errorf("E_INVARG: invalid shift count")
	}

	// Use unsigned cast for logical right shift (zero-fill, not sign-extending)
	result := int64(uint64(aInt) >> uint(bInt))
	vm.Push(types.NewInt(result))
	return nil
}
