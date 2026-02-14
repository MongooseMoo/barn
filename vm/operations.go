package vm

import (
	"barn/types"
	"fmt"
	"math"
	"strings"
)

// Arithmetic operations

func (vm *VM) executeAdd() error {
	b := vm.Pop()
	a := vm.Pop()

	// Handle string concatenation
	if _, ok := a.(types.StrValue); ok {
		if _, ok := b.(types.StrValue); ok {
			result := types.NewStr(a.(types.StrValue).Value() + b.(types.StrValue).Value())
			vm.Push(result)
			return nil
		}
	}

	// Handle numeric addition
	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if aIsInt && bIsInt {
		vm.Push(types.IntValue{Val: aInt.Val + bInt.Val})
		return nil
	}

	if (aIsInt || aIsFloat) && (bIsInt || bIsFloat) {
		var af, bf float64
		if aIsInt {
			af = float64(aInt.Val)
		} else {
			af = aFloat.Val
		}
		if bIsInt {
			bf = float64(bInt.Val)
		} else {
			bf = bFloat.Val
		}
		vm.Push(types.FloatValue{Val: af + bf})
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

	if (aIsInt || aIsFloat) && (bIsInt || bIsFloat) {
		var af, bf float64
		if aIsInt {
			af = float64(aInt.Val)
		} else {
			af = aFloat.Val
		}
		if bIsInt {
			bf = float64(bInt.Val)
		} else {
			bf = bFloat.Val
		}
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

	if (aIsInt || aIsFloat) && (bIsInt || bIsFloat) {
		var af, bf float64
		if aIsInt {
			af = float64(aInt.Val)
		} else {
			af = aFloat.Val
		}
		if bIsInt {
			bf = float64(bInt.Val)
		} else {
			bf = bFloat.Val
		}
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

	if (aIsInt || aIsFloat) && (bIsInt || bIsFloat) {
		var af, bf float64
		if aIsInt {
			af = float64(aInt.Val)
		} else {
			af = aFloat.Val
		}
		if bIsInt {
			bf = float64(bInt.Val)
		} else {
			bf = bFloat.Val
		}
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

	// Check for division by zero
	if bIsInt && bInt.Val == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}
	if bIsFloat && bFloat.Val == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}

	// If either is float, result is float
	if aIsFloat || bIsFloat {
		var af, bf float64
		if aIsInt {
			af = float64(aInt.Val)
		} else {
			af = aFloat.Val
		}
		if bIsInt {
			bf = float64(bInt.Val)
		} else {
			bf = bFloat.Val
		}
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

	result := math.Pow(af, bf)

	if math.IsNaN(result) || math.IsInf(result, 0) {
		return fmt.Errorf("E_FLOAT: result is NaN or Inf")
	}

	// If both operands were int, try to return int
	if aIsInt && bIsInt {
		if result == math.Floor(result) && result >= float64(math.MinInt64) && result <= float64(math.MaxInt64) {
			vm.Push(types.IntValue{Val: int64(result)})
			return nil
		}
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

// Comparison operations

func (vm *VM) executeEq() error {
	b := vm.Pop()
	a := vm.Pop()
	if a.Equal(b) {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeNe() error {
	b := vm.Pop()
	a := vm.Pop()
	if !a.Equal(b) {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeLt() error {
	b := vm.Pop()
	a := vm.Pop()

	// Type-specific comparison
	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result < 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeLe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result <= 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeGt() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result > 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeGe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result >= 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeIn() error {
	collection := vm.Pop()
	element := vm.Pop()

	// Check if element is in collection
	switch coll := collection.(type) {
	case types.ListValue:
		for i := 1; i <= coll.Len(); i++ {
			if element.Equal(coll.Get(i)) {
				vm.Push(types.IntValue{Val: int64(i)})
				return nil
			}
		}
		vm.Push(types.IntValue{Val: 0})
		return nil

	case types.StrValue:
		if elem, ok := element.(types.StrValue); ok {
			if strings.Contains(coll.Value(), elem.Value()) {
				vm.Push(types.IntValue{Val: 1})
			} else {
				vm.Push(types.IntValue{Val: 0})
			}
			return nil
		}
		return fmt.Errorf("E_TYPE: invalid element type for 'in' with string")

	case types.MapValue:
		// For maps, `in` checks if element is a VALUE and returns the position
		pairs := coll.Pairs()
		sortMapPairsForIn(pairs)
		for i, pair := range pairs {
			if pair[1].Equal(element) {
				vm.Push(types.IntValue{Val: int64(i + 1)})
				return nil
			}
		}
		vm.Push(types.IntValue{Val: 0})
		return nil

	default:
		return fmt.Errorf("E_TYPE: 'in' requires list, string, or map")
	}
}

// Logical operations

func (vm *VM) executeNot() error {
	a := vm.Pop()
	if !a.Truthy() {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeAnd() error {
	// Short-circuit AND
	// Stack has left value, offset is in bytecode
	offset := vm.ReadShort()
	val := vm.Peek(0)

	if !val.Truthy() {
		// Left is false, skip right and keep left value
		vm.CurrentFrame().IP += int(offset)
	} else {
		// Left is true, pop it and evaluate right
		vm.Pop()
	}

	return nil
}

func (vm *VM) executeOr() error {
	// Short-circuit OR
	// Stack has left value, offset is in bytecode
	offset := vm.ReadShort()
	val := vm.Peek(0)

	if val.Truthy() {
		// Left is true, skip right and keep left value
		vm.CurrentFrame().IP += int(offset)
	} else {
		// Left is false, pop it and evaluate right
		vm.Pop()
	}

	return nil
}

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

	// Use unsigned cast for logical right shift (zero-fill, not sign-extending)
	result := int64(uint64(aInt.Val) >> uint(bInt.Val))
	vm.Push(types.IntValue{Val: result})
	return nil
}

// Collection operations

func (vm *VM) executeIndex() error {
	index := vm.Pop()
	collection := vm.Pop()

	indexInt, indexOk := index.(types.IntValue)
	if !indexOk {
		return fmt.Errorf("E_TYPE: index must be integer")
	}

	switch coll := collection.(type) {
	case types.ListValue:
		if indexInt.Val < 1 || indexInt.Val > int64(coll.Len()) {
			return fmt.Errorf("E_RANGE: list index out of range")
		}
		vm.Push(coll.Get(int(indexInt.Val)))
		return nil

	case types.StrValue:
		if indexInt.Val < 1 || indexInt.Val > int64(len(coll.Value())) {
			return fmt.Errorf("E_RANGE: string index out of range")
		}
		vm.Push(types.NewStr(string(coll.Value()[indexInt.Val-1])))
		return nil

	default:
		return fmt.Errorf("E_TYPE: cannot index %s", collection.Type().String())
	}
}

func (vm *VM) executeIndexSet() error {
	// Bytecode: OP_INDEX_SET <varIdx:byte>
	// Stack: [... value_copy index] (value_copy and index on top)
	// After: modifies collection in locals[varIdx], pops index and value_copy
	varIdx := vm.ReadByte()
	index := vm.Pop()
	value := vm.Pop()

	// Read the collection from the variable slot
	coll := vm.CurrentFrame().Locals[varIdx]

	// Perform the index assignment using the shared setAtIndex helper
	newColl, errCode := setAtIndex(coll, index, value)
	if errCode != types.E_NONE {
		// Map error codes to error strings for the VM error handler
		switch errCode {
		case types.E_TYPE:
			return fmt.Errorf("E_TYPE: invalid index assignment")
		case types.E_RANGE:
			return fmt.Errorf("E_RANGE: index out of range")
		case types.E_INVARG:
			return fmt.Errorf("E_INVARG: invalid argument for index assignment")
		default:
			return fmt.Errorf("E_%d: index assignment error", errCode)
		}
	}

	// Write the modified collection back to the variable slot
	vm.CurrentFrame().Locals[varIdx] = newColl

	return nil
}

func (vm *VM) executeRange() error {
	end := vm.Pop()
	start := vm.Pop()
	collection := vm.Pop()

	startInt, startOk := start.(types.IntValue)
	endInt, endOk := end.(types.IntValue)

	if !startOk || !endOk {
		return fmt.Errorf("E_TYPE: range indices must be integers")
	}

	switch coll := collection.(type) {
	case types.ListValue:
		// Handle range slicing
		s := int(startInt.Val)
		e := int(endInt.Val)

		// Adjust for 1-based indexing
		if s < 1 {
			s = 1
		}
		if e > coll.Len() {
			e = coll.Len()
		}

		if s > e {
			// Reverse range
			elements := make([]types.Value, 0, s-e+1)
			for i := s; i >= e; i-- {
				elements = append(elements, coll.Get(i))
			}
			vm.Push(types.NewList(elements))
		} else {
			// Forward range
			vm.Push(coll.Slice(s, e))
		}
		return nil

	case types.StrValue:
		// Handle substring
		s := int(startInt.Val)
		e := int(endInt.Val)

		if s < 1 {
			s = 1
		}
		if e > len(coll.Value()) {
			e = len(coll.Value())
		}

		if s > e {
			return fmt.Errorf("E_INVARG: invalid string range")
		}

		vm.Push(types.NewStr(coll.Value()[s-1 : e]))
		return nil

	default:
		return fmt.Errorf("E_TYPE: cannot slice %s", collection.Type().String())
	}
}

func (vm *VM) executeMakeList() error {
	count := vm.ReadByte()
	elements := vm.PopN(int(count))
	vm.Push(types.NewList(elements))
	return nil
}

func (vm *VM) executeMakeMap() error {
	count := vm.ReadByte()
	pairs := make([][2]types.Value, count)

	for i := int(count) - 1; i >= 0; i-- {
		val := vm.Pop()
		key := vm.Pop()
		pairs[i] = [2]types.Value{key, val}
	}

	vm.Push(types.NewMap(pairs))
	return nil
}

func (vm *VM) executeLength() error {
	coll := vm.Pop()

	switch c := coll.(type) {
	case types.ListValue:
		vm.Push(types.IntValue{Val: int64(c.Len())})
	case types.StrValue:
		vm.Push(types.IntValue{Val: int64(len(c.Value()))})
	case types.MapValue:
		vm.Push(types.IntValue{Val: int64(c.Len())})
	default:
		return fmt.Errorf("E_TYPE: cannot get length of %s", coll.Type().String())
	}
	return nil
}

func (vm *VM) executeCallBuiltin() error {
	funcID := vm.ReadByte()
	argc := vm.ReadByte()
	args := vm.PopN(int(argc))

	// Call builtin
	result := vm.Builtins.CallByID(int(funcID), vm.Context, args)
	if result.Flow == types.FlowException {
		// Try to handle exception
		mooErr := MooError{Code: result.Error}
		if !vm.HandleError(mooErr) {
			return mooErr
		}
		return nil
	}

	vm.Push(result.Val)
	return nil
}

// Helper function to compare values
func compareValues(a, b types.Value) (int, error) {
	// Integer comparison
	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)

	if aIsInt && bIsInt {
		if aInt.Val < bInt.Val {
			return -1, nil
		} else if aInt.Val > bInt.Val {
			return 1, nil
		}
		return 0, nil
	}

	// Float comparison
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if (aIsInt || aIsFloat) && (bIsInt || bIsFloat) {
		var af, bf float64
		if aIsInt {
			af = float64(aInt.Val)
		} else {
			af = aFloat.Val
		}
		if bIsInt {
			bf = float64(bInt.Val)
		} else {
			bf = bFloat.Val
		}

		if af < bf {
			return -1, nil
		} else if af > bf {
			return 1, nil
		}
		return 0, nil
	}

	// String comparison
	aStr, aIsStr := a.(types.StrValue)
	bStr, bIsStr := b.(types.StrValue)

	if aIsStr && bIsStr {
		if aStr.Value() < bStr.Value() {
			return -1, nil
		} else if aStr.Value() > bStr.Value() {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
}

// executeScatter handles OP_SCATTER: validate that the top of stack is a list
// with the right number of elements for the scatter pattern.
//
// Bytecode format: OP_SCATTER <numRequired:byte> <numOptional:byte> <hasRest:byte>
//
// Pops the list value from the stack.
// Validates:
//   - Value is a list (E_TYPE if not)
//   - length >= numRequired (E_ARGS if too few)
//   - If !hasRest: length <= numRequired + numOptional (E_ARGS if too many)
func (vm *VM) executeScatter() error {
	numRequired := int(vm.ReadByte())
	numOptional := int(vm.ReadByte())
	hasRest := vm.ReadByte() != 0

	val := vm.Pop()
	listVal, ok := val.(types.ListValue)
	if !ok {
		return fmt.Errorf("E_TYPE: scatter assignment requires a list")
	}

	length := listVal.Len()
	if length < numRequired {
		return fmt.Errorf("E_ARGS: too few elements for scatter assignment")
	}
	if !hasRest && length > numRequired+numOptional {
		return fmt.Errorf("E_ARGS: too many elements for scatter assignment")
	}

	return nil
}
