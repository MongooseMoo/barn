package vm

import (
	"barn/builtins"
	"barn/types"
	"fmt"
	"math"
)

// Arithmetic operations

// promoting reports whether PROMOTE_NUMBERS is enabled for the current task.
func (vm *VM) promoting() bool {
	return vm.Context != nil && vm.Context.RuntimeOptions.PromoteNumbers
}

// promoteNumericPair returns the float64 values of a and b when each is numeric
// (int or float). It is only consulted on the slow path after the strict
// same-type fast paths have already been tried, so it never affects the hot
// int+int / float+float branches. The bothNumeric return is false when either
// operand is not a number, in which case the caller falls through to E_TYPE.
func promoteNumericPair(a, b types.Value) (af, bf float64, bothNumeric bool) {
	af, aok := numericToFloat(a)
	bf, bok := numericToFloat(b)
	return af, bf, aok && bok
}

// numericToFloat converts an int or float Value to float64.
func numericToFloat(v types.Value) (float64, bool) {
	switch n := v.(type) {
	case types.IntValue:
		return float64(n.Val), true
	case types.FloatValue:
		return n.Val, true
	}
	return 0, false
}

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
			result := aFloat.Val + bFloat.Val
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.FloatValue{Val: result})
			return nil
		}
	}

	// Handle string concatenation
	if aStr, ok := a.(types.StrValue); ok {
		if bStr, ok := b.(types.StrValue); ok {
			resultStr := aStr.Value() + bStr.Value()
			if errCode := builtins.CheckStringLength(len(resultStr)); errCode != types.E_NONE {
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

	// PROMOTE_NUMBERS: mixed int/float -> float add (SIMPLE_BINARY).
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			result := af + bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.FloatValue{Val: result})
			return nil
		}
	}

	return fmt.Errorf("E_TYPE: invalid operands for +")
}

func (vm *VM) executeStringAppend() error {
	b := vm.Pop()
	a := vm.Pop()

	// Numeric-first: this handler is also reached by the `x = x + expr`
	// self-accumulation peephole for int/float accumulators, so test the
	// numeric types before string to avoid a failed StrValue assertion every
	// iteration of a numeric hot loop (mirrors executeAdd's ordering). Pure
	// reordering — no behavior change: a Value has exactly one dynamic type, so
	// no operand matches two branches, and the string-a + non-string-b early
	// E_TYPE below still fires unchanged.
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

	if aStr, ok := a.(types.StrValue); ok {
		bStr, ok := b.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: invalid operands for +")
		}

		if errCode := builtins.CheckStringLength(aStr.Len() + bStr.Len()); errCode != types.E_NONE {
			return fmt.Errorf("E_QUOTA: string too long")
		}
		vm.Push(aStr.Append(bStr))
		return nil
	}

	if aList, aIsList := a.(types.ListValue); aIsList {
		if bList, bIsList := b.(types.ListValue); bIsList {
			aElems := aList.Elements()
			bElems := bList.Elements()
			newElems := make([]types.Value, len(aElems)+len(bElems))
			copy(newElems, aElems)
			copy(newElems[len(aElems):], bElems)
			vm.Push(types.NewList(newElems))
			return nil
		}
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
		result := aFloat.Val - bFloat.Val
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}

	// PROMOTE_NUMBERS: mixed int/float -> float subtract (SIMPLE_BINARY).
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			result := af - bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.FloatValue{Val: result})
			return nil
		}
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
		result := aFloat.Val * bFloat.Val
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}

	// PROMOTE_NUMBERS: mixed int/float -> float multiply (SIMPLE_BINARY).
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			result := af * bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.FloatValue{Val: result})
			return nil
		}
	}

	return fmt.Errorf("E_TYPE: invalid operands for *")
}

func (vm *VM) executeDiv() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	// Pure int/int branch (unchanged): b==0 -> E_DIV, MININT/-1 special case.
	if aIsInt && bIsInt {
		if bInt.Val == 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
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
		result := af / bf
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
	}

	// PROMOTE_NUMBERS: mixed int/float -> promote both to float, then divide.
	// Divisor 0.0 -> E_DIV (before E_FLOAT); non-real result -> E_FLOAT.
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			if bf == 0 {
				return fmt.Errorf("E_DIV: division by zero")
			}
			result := af / bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.FloatValue{Val: result})
			return nil
		}
	}

	// Strict mixed/invalid: preserve prior behavior. Note the old code's early
	// b==0 check fired for any int-zero divisor regardless of a's type; replicate
	// that so strict-mode results are byte-identical (e.g. 5.0 / 0 -> E_DIV).
	if bIsInt && bInt.Val == 0 {
		return fmt.Errorf("E_DIV: division by zero")
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
	// PROMOTE_NUMBERS: mixed int/float -> promote both to float, then run the
	// floored float modulo (same algorithm as the float/float branch below).
	// Zero divisor -> E_DIV.
	if aIsInt != bIsInt && vm.promoting() {
		af, _ := numericToFloat(a)
		bf, _ := numericToFloat(b)
		if bf == 0 {
			return fmt.Errorf("E_DIV: modulo by zero")
		}
		result := math.Mod(af, bf)
		if result != 0 && (result < 0) != (bf < 0) {
			result += bf
		}
		vm.Push(types.FloatValue{Val: result})
		return nil
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

	// Strict: int ^ float is E_TYPE. Under PROMOTE_NUMBERS, it becomes a legal
	// float pow (both operands already coerced to af/bf above; falls through to
	// the math.Pow path below, which returns E_FLOAT on a non-real result).
	if aIsInt && bIsFloat && !vm.promoting() {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}

	if aIsInt && bIsInt {
		// PROMOTE_NUMBERS: int ^ (negative int) returns a raw float pow
		// (Toast mongoose do_power, PROMOTE branch). NO E_DIV special-case
		// (0 ^ -1 -> +Inf as a float) and NO IS_REAL/E_FLOAT rejection.
		// Non-negative int exponents stay integer (handled below, unchanged).
		if vm.promoting() && bInt.Val < 0 {
			vm.Push(types.FloatValue{Val: math.Pow(af, bf)})
			return nil
		}
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
