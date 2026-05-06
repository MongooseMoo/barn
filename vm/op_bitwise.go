package vm

import (
	"barn/types"
	"fmt"
)

// Bitwise operations

func (vm *VM) executeBitOr() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.(types.IntValue)
	bInt, bOk := b.(types.IntValue)

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.IntValue{Val: aInt.Val | bInt.Val})
	return nil
}

func (vm *VM) executeBitAnd() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.(types.IntValue)
	bInt, bOk := b.(types.IntValue)

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.IntValue{Val: aInt.Val & bInt.Val})
	return nil
}

func (vm *VM) executeBitXor() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.(types.IntValue)
	bInt, bOk := b.(types.IntValue)

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.IntValue{Val: aInt.Val ^ bInt.Val})
	return nil
}

func (vm *VM) executeBitNot() error {
	a := vm.Pop()

	aInt, ok := a.(types.IntValue)
	if !ok {
		return fmt.Errorf("E_TYPE: bitwise operations require integers")
	}

	vm.Push(types.IntValue{Val: ^aInt.Val})
	return nil
}

func (vm *VM) executeShl() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.(types.IntValue)
	bInt, bOk := b.(types.IntValue)

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: shift operations require integers")
	}

	if bInt.Val < 0 {
		return fmt.Errorf("E_INVARG: negative shift count")
	}
	if bInt.Val == 64 {
		vm.Push(types.IntValue{Val: 0})
		return nil
	}
	if bInt.Val > 64 {
		return fmt.Errorf("E_INVARG: invalid shift count")
	}

	vm.Push(types.IntValue{Val: aInt.Val << uint(bInt.Val)})
	return nil
}

func (vm *VM) executeShr() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aOk := a.(types.IntValue)
	bInt, bOk := b.(types.IntValue)

	if !aOk || !bOk {
		return fmt.Errorf("E_TYPE: shift operations require integers")
	}

	if bInt.Val < 0 {
		return fmt.Errorf("E_INVARG: negative shift count")
	}
	if bInt.Val == 64 {
		vm.Push(types.IntValue{Val: 0})
		return nil
	}
	if bInt.Val > 64 {
		return fmt.Errorf("E_INVARG: invalid shift count")
	}

	// Use unsigned cast for logical right shift (zero-fill, not sign-extending)
	result := int64(uint64(aInt.Val) >> uint(bInt.Val))
	vm.Push(types.IntValue{Val: result})
	return nil
}
