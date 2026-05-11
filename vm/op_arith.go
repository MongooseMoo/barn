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

	aInt, bInt, aFloat, bFloat, useFloat, ok := numericPair(a, b)
	if !ok {
		return fmt.Errorf("E_TYPE: invalid operands for +")
	}
	if useFloat {
		result := aFloat + bFloat
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}
	vm.Push(types.IntValue{Val: aInt + bInt})
	return nil
}

func (vm *VM) executeSub() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, bInt, aFloat, bFloat, useFloat, ok := numericPair(a, b)
	if !ok {
		return fmt.Errorf("E_TYPE: invalid operands for -")
	}
	if useFloat {
		result := aFloat - bFloat
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}
	vm.Push(types.IntValue{Val: aInt - bInt})
	return nil
}

func (vm *VM) executeMul() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, bInt, aFloat, bFloat, useFloat, ok := numericPair(a, b)
	if !ok {
		return fmt.Errorf("E_TYPE: invalid operands for *")
	}
	if useFloat {
		result := aFloat * bFloat
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}
	vm.Push(types.IntValue{Val: aInt * bInt})
	return nil
}

func (vm *VM) executeDiv() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, bInt, aFloat, bFloat, useFloat, ok := numericPair(a, b)
	if !ok {
		return fmt.Errorf("E_TYPE: invalid operands for /")
	}
	if useFloat {
		if bFloat == 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		result := aFloat / bFloat
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}
	if bInt == 0 {
		return fmt.Errorf("E_DIV: division by zero")
	}
	{
		// Toast special case: MININT / -1 returns MININT to prevent overflow
		if aInt == MININT && bInt == -1 {
			vm.Push(types.IntValue{Val: MININT})
		} else {
			vm.Push(types.IntValue{Val: aInt / bInt})
		}
		return nil
	}
}

func (vm *VM) executeMod() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, bInt, aFloat, bFloat, useFloat, ok := numericPair(a, b)
	if !ok {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}
	if useFloat {
		if bFloat == 0 {
			return fmt.Errorf("E_DIV: modulo by zero")
		}
		result := math.Mod(aFloat, bFloat)
		if result != 0 && (result < 0) != (bFloat < 0) {
			result += bFloat
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}

	if bInt == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}
	result := aInt % bInt
	if result != 0 && (result < 0) != (bInt < 0) {
		result += bInt
	}
	vm.Push(types.IntValue{Val: result})
	return nil
}

func (vm *VM) executePow() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, bInt, aFloat, bFloat, useFloat, ok := numericPair(a, b)
	if !ok {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}
	if !useFloat {
		// Toast semantics: 0 ^ negative is division by zero.
		if aInt == 0 && bInt < 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		// Negative exponents with integer operands truncate toward zero.
		if bInt < 0 {
			vm.Push(types.IntValue{Val: int64(math.Pow(float64(aInt), float64(bInt)))})
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
		vm.Push(types.IntValue{Val: result})
		return nil
	}

	result := math.Pow(aFloat, bFloat)

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
