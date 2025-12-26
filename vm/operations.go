package vm

import (
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
		vm.Push(types.IntValue{Val: aInt.Val / bInt.Val})
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

	if !aIsInt || !bIsInt {
		return fmt.Errorf("E_TYPE: invalid operands for %%")
	}

	if bInt.Val == 0 {
		return fmt.Errorf("E_DIV: modulo by zero")
	}

	vm.Push(types.IntValue{Val: aInt.Val % bInt.Val})
	return nil
}

func (vm *VM) executePow() error {
	b := vm.Pop()
	a := vm.Pop()

	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	var result float64
	if aIsInt {
		if bIsInt {
			result = math.Pow(float64(aInt.Val), float64(bInt.Val))
		} else if bIsFloat {
			result = math.Pow(float64(aInt.Val), bFloat.Val)
		} else {
			return fmt.Errorf("E_TYPE: invalid operands for ^")
		}
	} else if aIsFloat {
		if bIsInt {
			result = math.Pow(aFloat.Val, float64(bInt.Val))
		} else if bIsFloat {
			result = math.Pow(aFloat.Val, bFloat.Val)
		} else {
			return fmt.Errorf("E_TYPE: invalid operands for ^")
		}
	} else {
		return fmt.Errorf("E_TYPE: invalid operands for ^")
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
	vm.Push(types.BoolValue{Val: a.Equal(b)})
	return nil
}

func (vm *VM) executeNe() error {
	b := vm.Pop()
	a := vm.Pop()
	vm.Push(types.BoolValue{Val: !a.Equal(b)})
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

	vm.Push(types.BoolValue{Val: result < 0})
	return nil
}

func (vm *VM) executeLe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	vm.Push(types.BoolValue{Val: result <= 0})
	return nil
}

func (vm *VM) executeGt() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	vm.Push(types.BoolValue{Val: result > 0})
	return nil
}

func (vm *VM) executeGe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	vm.Push(types.BoolValue{Val: result >= 0})
	return nil
}

func (vm *VM) executeIn() error {
	collection := vm.Pop()
	element := vm.Pop()

	// Check if element is in collection
	switch coll := collection.(type) {
	case types.ListValue:
		for i := 0; i < coll.Len(); i++ {
			if element.Equal(coll.Get(i + 1)) {
				vm.Push(types.BoolValue{Val: true})
				return nil
			}
		}
		vm.Push(types.BoolValue{Val: false})
		return nil

	case types.StrValue:
		if elem, ok := element.(types.StrValue); ok {
			// Check if substring
			result := false
			// Simple substring check (TODO: use proper algorithm)
			for i := 0; i <= len(coll.Value())-len(elem.Value()); i++ {
				if coll.Value()[i:i+len(elem.Value())] == elem.Value() {
					result = true
					break
				}
			}
			vm.Push(types.BoolValue{Val: result})
			return nil
		}
		return fmt.Errorf("E_TYPE: invalid element type for 'in' with string")

	default:
		return fmt.Errorf("E_TYPE: 'in' requires list or string")
	}
}

// Logical operations

func (vm *VM) executeNot() error {
	a := vm.Pop()
	vm.Push(types.BoolValue{Val: !a.Truthy()})
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

	vm.Push(types.IntValue{Val: aInt.Val >> uint(bInt.Val)})
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
