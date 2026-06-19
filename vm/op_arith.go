package vm

import (
	"barn/builtins"
	"barn/types"
	"fmt"
	"math"
)

// Arithmetic operations

func (vm *VM) executeAdd() error {
	b := vm.Pop()
	a := vm.Pop()

	// Fast path: numeric addition is by far the most common case in hot loops,
	// so test it before string/list to avoid failed type assertions per op.
	if aInt, ok := a.(types.IntValue); ok {
		if bInt, ok := b.(types.IntValue); ok {
			vm.Push(types.IntValue{Val: aInt.Val + bInt.Val})
			return nil
		}
	}
	if aFloat, ok := a.(types.FloatValue); ok {
		if bFloat, ok := b.(types.FloatValue); ok {
			vm.Push(types.FloatValue{Val: aFloat.Val + bFloat.Val})
			return nil
		}
	}

	// Handle string concatenation
	if _, ok := a.(types.StrValue); ok {
		if _, ok := b.(types.StrValue); ok {
			resultStr := a.(types.StrValue).Value() + b.(types.StrValue).Value()
			if errCode := builtins.CheckStringLimit(resultStr); errCode != types.E_NONE {
				return fmt.Errorf("E_QUOTA: string too long")
			}
			vm.Push(types.NewStr(resultStr))
			return nil
		}
	}

	// Handle list concatenation (list + list) and append (list + any)
	if aList, aIsList := a.(types.ListValue); aIsList {
		if bList, bIsList := b.(types.ListValue); bIsList {
			// list + list → concatenation (new list)
			aElems := aList.Elements()
			bElems := bList.Elements()
			newElems := make([]types.Value, len(aElems)+len(bElems))
			copy(newElems, aElems)
			copy(newElems[len(aElems):], bElems)
			vm.Push(types.NewList(newElems))
			return nil
		}
		// list + any → append (new list)
		vm.Push(aList.Append(b))
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for +")
}

func (vm *VM) executeSub() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if aIsInt && bIsInt {
		vm.Push(types.IntValue{Val: aInt.Val - bInt.Val})
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat.Val
		bf := bFloat.Val
		vm.Push(types.FloatValue{Val: af - bf})
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for -")
}

func (vm *VM) executeMul() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if aIsInt && bIsInt {
		vm.Push(types.IntValue{Val: aInt.Val * bInt.Val})
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat.Val
		bf := bFloat.Val
		vm.Push(types.FloatValue{Val: af * bf})
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for *")
}

func (vm *VM) executeDiv() error {
	b := vm.Pop()
	a := vm.Pop()

	bInt, bIsInt := b.(types.IntValue)
	if bIsInt && bInt.Val == 0 {
		return fmt.Errorf("E_DIV: division by zero")
	}

	aInt, aIsInt := a.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if aIsInt && bIsInt {
		// Toast special case: MININT / -1 returns MININT to prevent overflow
		if aInt.Val == MININT && bInt.Val == -1 {
			vm.Push(types.IntValue{Val: MININT})
		} else {
			vm.Push(types.IntValue{Val: aInt.Val / bInt.Val})
		}
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat.Val
		bf := bFloat.Val
		if bf == 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		vm.Push(types.FloatValue{Val: af / bf})
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for /")
}

func (vm *VM) executeMod() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if !(aIsInt || aIsFloat) || !(bIsInt || bIsFloat) {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}
	if aIsInt != bIsInt {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}

	// Check for division by zero
	if bIsInt && bInt.Val == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}
	if bIsFloat && bFloat.Val == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}

	// Both are floats.
	if aIsFloat {
		af := aFloat.Val
		bf := bFloat.Val
		result := math.Mod(af, bf)
		// Floored modulo: result sign matches divisor
		if result != 0 && (result < 0) != (bf < 0) {
			result += bf
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}

	// Both ints — floored modulo
	result := aInt.Val % bInt.Val
	if result != 0 && (result < 0) != (bInt.Val < 0) {
		result += bInt.Val
	}
	vm.Push(types.IntValue{Val: result})
	return nil
}

func (vm *VM) executePow() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	var af, bf float64
	if aIsInt {
		af = float64(aInt.Val)
	} else if aIsFloat {
		af = aFloat.Val
	} else {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}
	if bIsInt {
		bf = float64(bInt.Val)
	} else if bIsFloat {
		bf = bFloat.Val
	} else {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}

	if aIsInt && bIsFloat {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}

	if aIsInt && bIsInt {
		// Toast semantics: 0 ^ negative is division by zero.
		if aInt.Val == 0 && bInt.Val < 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		// Negative exponents with integer operands truncate toward zero.
		if bInt.Val < 0 {
			vm.Push(types.IntValue{Val: int64(math.Pow(af, bf))})
			return nil
		}

		// Non-negative exponent: integer exponentiation.
		result := int64(1)
		base := aInt.Val
		exp := bInt.Val
		for exp > 0 {
			if exp&1 == 1 {
				result *= base
			}
			exp >>= 1
			if exp > 0 {
				base *= base
			}
		}
		vm.Push(types.IntValue{Val: result})
		return nil
	}

	result := math.Pow(af, bf)

	if math.IsNaN(result) || math.IsInf(result, 0) {
		return fmt.Errorf("E_FLOAT: result is NaN or Inf")
	}

	vm.Push(types.FloatValue{Val: result})
	return nil
}

func (vm *VM) executeNeg() error {
	a := vm.Pop()

	if aInt, ok := a.(types.IntValue); ok {
		vm.Push(types.IntValue{Val: -aInt.Val})
		return nil
	}

	if aFloat, ok := a.(types.FloatValue); ok {
		vm.Push(types.FloatValue{Val: -aFloat.Val})
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operand for unary -")
}
