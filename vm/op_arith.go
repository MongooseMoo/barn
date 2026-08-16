package vm

import (
	"fmt"
	"math"

	"github.com/MongooseMoo/barn/types"
)

// Arithmetic operations

// Toast defines MININT as one more than INT64_MIN to avoid overflow issues
// when negating or dividing MININT by -1. We match this for compatibility.
const MININT int64 = -9223372036854775807

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
	switch v.Type() {
	case types.TYPE_INT:
		return float64(v.Int()), true
	case types.TYPE_FLOAT:
		return v.Float(), true
	}
	return 0, false
}

func (vm *VM) executeAdd() error {
	b := vm.Pop()
	a := vm.Pop()

	// Fast path: numeric addition is by far the most common case in hot loops,
	// so test it before string/list to avoid failed type assertions per op.
	if a.Type() == types.TYPE_INT {
		if b.Type() == types.TYPE_INT {
			vm.Push(types.NewInt(a.Int() + b.Int()))
			return nil
		}
	}
	if a.Type() == types.TYPE_FLOAT {
		if b.Type() == types.TYPE_FLOAT {
			result := a.Float() + b.Float()
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.NewFloat(result))
			return nil
		}
	}

	// Handle string concatenation
	if a.Type() == types.TYPE_STR {
		if b.Type() == types.TYPE_STR {
			resultStr := a.Str() + b.Str()
			if errCode := vm.Builtins.CheckStringLength(len(resultStr)); errCode != types.E_NONE {
				return fmt.Errorf("E_QUOTA: string too long")
			}
			vm.Push(types.NewStr(resultStr))
			return nil
		}
	}

	// Handle list concatenation (list + list) and append (list + any)
	if a.Type() == types.TYPE_LIST {
		if b.Type() == types.TYPE_LIST {
			// list + list → concatenation (new list)
			aElems := a.Elements()
			bElems := b.Elements()
			newElems := make([]types.Value, len(aElems)+len(bElems))
			copy(newElems, aElems)
			copy(newElems[len(aElems):], bElems)
			vm.Push(types.NewList(newElems))
			return nil
		}
		// list + any → append (new list)
		vm.Push(a.Append(b))
		return nil
	}

	// PROMOTE_NUMBERS: mixed int/float -> float add (SIMPLE_BINARY).
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			result := af + bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.NewFloat(result))
			return nil
		}
	}

	return fmt.Errorf("E_TYPE: invalid operands for +")
}

func (vm *VM) executeStringAppend() error {
	// OP_STRING_APPEND is retained for bytecode compatibility, but the
	// self-add peephole can route every kind of addition here. Keep a single
	// implementation so options, quotas, and arithmetic errors cannot drift.
	return vm.executeAdd()
}

func (vm *VM) executeSub() error {
	b := vm.Pop()
	a := vm.Pop()

	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT

	if aIsInt && bIsInt {
		vm.Push(types.NewInt(a.Int() - b.Int()))
		return nil
	}

	if aIsFloat && bIsFloat {
		result := a.Float() - b.Float()
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.NewFloat(result))
		return nil
	}

	// PROMOTE_NUMBERS: mixed int/float -> float subtract (SIMPLE_BINARY).
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			result := af - bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.NewFloat(result))
			return nil
		}
	}

	return fmt.Errorf("E_TYPE: invalid operands for -")
}

func (vm *VM) executeMul() error {
	b := vm.Pop()
	a := vm.Pop()

	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT

	if aIsInt && bIsInt {
		vm.Push(types.NewInt(a.Int() * b.Int()))
		return nil
	}

	if aIsFloat && bIsFloat {
		result := a.Float() * b.Float()
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.NewFloat(result))
		return nil
	}

	// PROMOTE_NUMBERS: mixed int/float -> float multiply (SIMPLE_BINARY).
	if vm.promoting() {
		if af, bf, ok := promoteNumericPair(a, b); ok {
			result := af * bf
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("E_FLOAT: result is NaN or Inf")
			}
			vm.Push(types.NewFloat(result))
			return nil
		}
	}

	return fmt.Errorf("E_TYPE: invalid operands for *")
}

func (vm *VM) executeDiv() error {
	b := vm.Pop()
	a := vm.Pop()

	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT

	// Pure int/int branch (unchanged): b==0 -> E_DIV, MININT/-1 special case.
	if aIsInt && bIsInt {
		if b.Int() == 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		// Toast special case: MININT / -1 returns MININT to prevent overflow
		if a.Int() == MININT && b.Int() == -1 {
			vm.Push(types.NewInt(MININT))
		} else {
			vm.Push(types.NewInt(a.Int() / b.Int()))
		}
		return nil
	}

	if aIsFloat && bIsFloat {
		af := a.Float()
		bf := b.Float()
		if bf == 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		result := af / bf
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return fmt.Errorf("E_FLOAT: result is NaN or Inf")
		}
		vm.Push(types.NewFloat(result))
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
			vm.Push(types.NewFloat(result))
			return nil
		}
	}

	// Strict mixed/invalid: preserve prior behavior. Note the old code's early
	// b==0 check fired for any int-zero divisor regardless of a's type; replicate
	// that so strict-mode results are byte-identical (e.g. 5.0 / 0 -> E_DIV).
	if bIsInt && b.Int() == 0 {
		return fmt.Errorf("E_DIV: division by zero")
	}

	return fmt.Errorf("E_TYPE: invalid operands for /")
}

func (vm *VM) executeMod() error {
	b := vm.Pop()
	a := vm.Pop()

	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT

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
		vm.Push(types.NewFloat(result))
		return nil
	}
	if aIsInt != bIsInt {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}

	// Check for division by zero
	if bIsInt && b.Int() == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}
	if bIsFloat && b.Float() == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}

	// Both are floats.
	if aIsFloat {
		af := a.Float()
		bf := b.Float()
		result := math.Mod(af, bf)
		// Floored modulo: result sign matches divisor
		if result != 0 && (result < 0) != (bf < 0) {
			result += bf
		}
		vm.Push(types.NewFloat(result))
		return nil
	}

	// Both ints — floored modulo
	result := a.Int() % b.Int()
	if result != 0 && (result < 0) != (b.Int() < 0) {
		result += b.Int()
	}
	vm.Push(types.NewInt(result))
	return nil
}

func (vm *VM) executePow() error {
	b := vm.Pop()
	a := vm.Pop()

	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT

	var af, bf float64
	if aIsInt {
		af = float64(a.Int())
	} else if aIsFloat {
		af = a.Float()
	} else {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
	}
	if bIsInt {
		bf = float64(b.Int())
	} else if bIsFloat {
		bf = b.Float()
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
		if vm.promoting() && b.Int() < 0 {
			vm.Push(types.NewFloat(math.Pow(af, bf)))
			return nil
		}
		// Toast semantics: 0 ^ negative is division by zero.
		if a.Int() == 0 && b.Int() < 0 {
			return fmt.Errorf("E_DIV: division by zero")
		}
		// Negative exponents with integer operands truncate toward zero.
		if b.Int() < 0 {
			vm.Push(types.NewInt(int64(math.Pow(af, bf))))
			return nil
		}

		// Non-negative exponent: integer exponentiation.
		result := int64(1)
		base := a.Int()
		exp := b.Int()
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

	if a.Type() == types.TYPE_INT {
		vm.Push(types.NewInt(-a.Int()))
		return nil
	}

	if a.Type() == types.TYPE_FLOAT {
		vm.Push(types.NewFloat(-a.Float()))
		return nil
	}

	return fmt.Errorf("E_TYPE: invalid operand for unary -")
}
