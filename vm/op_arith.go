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
	if aInt, ok := a.AsInt(); ok {
		if bInt, ok := b.AsInt(); ok {
			vm.Push(types.NewInt(aInt + bInt))
			return nil
		}
	}
	if aFloat, ok := a.AsFloat(); ok {
		if bFloat, ok := b.AsFloat(); ok {
			vm.Push(types.NewFloat(aFloat + bFloat))
			return nil
		}
	}

	// Handle string concatenation
	if aStr, ok := a.AsStr(); ok {
		if bStr, ok := b.AsStr(); ok {
			resultStr := aStr + bStr
			if errCode := builtins.CheckStringLimit(resultStr); errCode != types.E_NONE {
				return fmt.Errorf("E_QUOTA: string too long")
			}
			vm.Push(types.NewStr(resultStr))
			return nil
		}
	}

	// Handle list concatenation (list + list) and append (list + any)
	if aList, aIsList := a.AsList(); aIsList {
		if bList, bIsList := b.AsList(); bIsList {
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
		vm.Push(aList.Append(b).AsValue())
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for +")
}

func (vm *VM) executeSub() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.AsInt()
	bInt, bIsInt := b.AsInt()
	aFloat, aIsFloat := a.AsFloat()
	bFloat, bIsFloat := b.AsFloat()

	if aIsInt && bIsInt {
		vm.Push(types.NewInt(aInt - bInt))
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat
		bf := bFloat
		vm.Push(types.NewFloat(af - bf))
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for -")
}

func (vm *VM) executeMul() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.AsInt()
	bInt, bIsInt := b.AsInt()
	aFloat, aIsFloat := a.AsFloat()
	bFloat, bIsFloat := b.AsFloat()

	if aIsInt && bIsInt {
		vm.Push(types.NewInt(aInt * bInt))
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat
		bf := bFloat
		vm.Push(types.NewFloat(af * bf))
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for *")
}

func (vm *VM) executeDiv() error {
	b := vm.Pop()
	a := vm.Pop()

	bInt, bIsInt := b.AsInt()
	if bIsInt && bInt == 0 {
		return fmt.Errorf("E_DIV: division by zero")
	}

	aInt, aIsInt := a.AsInt()
	aFloat, aIsFloat := a.AsFloat()
	bFloat, bIsFloat := b.AsFloat()

	if aIsInt && bIsInt {
		// Toast special case: MININT / -1 returns MININT to prevent overflow
		if aInt == MININT && bInt == -1 {
			vm.Push(types.NewInt(MININT))
		} else {
			vm.Push(types.NewInt(aInt / bInt))
		}
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat
		bf := bFloat
		if bf == 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		vm.Push(types.NewFloat(af / bf))
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operands for /")
}

func (vm *VM) executeMod() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.AsInt()
	bInt, bIsInt := b.AsInt()
	aFloat, aIsFloat := a.AsFloat()
	bFloat, bIsFloat := b.AsFloat()

	if !(aIsInt || aIsFloat) || !(bIsInt || bIsFloat) {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}
	if aIsInt != bIsInt {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}

	// Check for division by zero
	if bIsInt && bInt == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}
	if bIsFloat && bFloat == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}

	// Both are floats.
	if aIsFloat {
		af := aFloat
		bf := bFloat
		result := math.Mod(af, bf)
		// Floored modulo: result sign matches divisor
		if result != 0 && (result < 0) != (bf < 0) {
			result += bf
		}
		vm.Push(types.NewFloat(result))
		return nil
	}

	// Both ints — floored modulo
	result := aInt % bInt
	if result != 0 && (result < 0) != (bInt < 0) {
		result += bInt
	}
	vm.Push(types.NewInt(result))
	return nil
}

func (vm *VM) executePow() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.AsInt()
	bInt, bIsInt := b.AsInt()
	aFloat, aIsFloat := a.AsFloat()
	bFloat, bIsFloat := b.AsFloat()

	var af, bf float64
	if aIsInt {
		af = float64(aInt)
	} else if aIsFloat {
		af = aFloat
	} else {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}
	if bIsInt {
		bf = float64(bInt)
	} else if bIsFloat {
		bf = bFloat
	} else {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}

	if aIsInt && bIsFloat {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}

	if aIsInt && bIsInt {
		// Toast semantics: 0 ^ negative is division by zero.
		if aInt == 0 && bInt < 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		// Negative exponents with integer operands truncate toward zero.
		if bInt < 0 {
			vm.Push(types.NewInt(int64(math.Pow(af, bf))))
			return nil
		}

		// Non-negative exponent: integer exponentiation.
		result := int64(1)
		base := aInt
		exp := bInt
		for exp > 0 {
			if exp&1 == 1 {
				result *= base
			}
			exp >>= 1
			if exp > 0 {
				base *= base
			}
		}
		vm.Push(types.NewInt(result))
		return nil
	}

	result := math.Pow(af, bf)

	if math.IsNaN(result) || math.IsInf(result, 0) {
		return fmt.Errorf("E_FLOAT: result is NaN or Inf")
	}

	vm.Push(types.NewFloat(result))
	return nil
}

func (vm *VM) executeNeg() error {
	a := vm.Pop()

	if aInt, ok := a.AsInt(); ok {
		vm.Push(types.NewInt(-aInt))
		return nil
	}

	if aFloat, ok := a.AsFloat(); ok {
		vm.Push(types.NewFloat(-aFloat))
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operand for unary -")
}
