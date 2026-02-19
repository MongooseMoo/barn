package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/task"
	"barn/types"
	"fmt"
	"math"
	"sort"
	"strings"
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

	// Handle numeric addition
	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if aIsInt && bIsInt {
		vm.Push(types.IntValue{Val: aInt.Val + bInt.Val})
		return nil
	}

	if aIsFloat && bIsFloat {
		af := aFloat.Val
		bf := bFloat.Val
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

// Comparison operations

func (vm *VM) executeEq() error {
	b := vm.Pop()
	a := vm.Pop()
	if eq, ok := boolIntEqual(a, b); ok {
		if eq {
			vm.Push(types.IntValue{Val: 1})
		} else {
			vm.Push(types.IntValue{Val: 0})
		}
		return nil
	}
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
	if eq, ok := boolIntEqual(a, b); ok {
		if eq {
			vm.Push(types.IntValue{Val: 0})
		} else {
			vm.Push(types.IntValue{Val: 1})
		}
		return nil
	}
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
			haystack := strings.ToLower(coll.Value())
			needle := strings.ToLower(elem.Value())
			if pos := strings.Index(haystack, needle); pos >= 0 {
				vm.Push(types.IntValue{Val: int64(pos + 1)})
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

	switch coll := collection.(type) {
	case types.ListValue:
		indexInt, indexOk := index.(types.IntValue)
		if !indexOk {
			return fmt.Errorf("E_TYPE: list index must be integer")
		}
		if indexInt.Val < 1 || indexInt.Val > int64(coll.Len()) {
			return fmt.Errorf("E_RANGE: list index out of range")
		}
		vm.Push(coll.Get(int(indexInt.Val)))
		return nil

	case types.StrValue:
		indexInt, indexOk := index.(types.IntValue)
		if !indexOk {
			return fmt.Errorf("E_TYPE: string index must be integer")
		}
		if indexInt.Val < 1 || indexInt.Val > int64(len(coll.Value())) {
			return fmt.Errorf("E_RANGE: string index out of range")
		}
		vm.Push(types.NewStr(string(coll.Value()[indexInt.Val-1])))
		return nil

	case types.MapValue:
		// Map keys must be scalar types (not list or map)
		switch index.(type) {
		case types.ListValue, types.MapValue:
			return fmt.Errorf("E_TYPE: invalid map key type")
		}
		val, ok := coll.Get(index)
		if !ok {
			return fmt.Errorf("E_RANGE: map key not found")
		}
		vm.Push(val)
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

func (vm *VM) executeRangeSet() error {
	// Bytecode: OP_RANGE_SET <varIdx:byte>
	// Stack: [... value_copy start end] (end on top)
	// After: modifies collection in locals[varIdx], pops end, start, and value_copy
	varIdx := vm.ReadByte()
	end := vm.Pop()
	start := vm.Pop()
	value := vm.Pop()

	// Read the collection from the variable slot
	coll := vm.CurrentFrame().Locals[varIdx]

	// Perform range assignment based on collection type
	var newColl types.Value
	switch c := coll.(type) {
	case types.ListValue:
		startInt, startOk := start.(types.IntValue)
		endInt, endOk := end.(types.IntValue)
		if !startOk || !endOk {
			return fmt.Errorf("E_TYPE: range indices must be integers")
		}
		startIdx := startInt.Val
		endIdx := endInt.Val

		// Value must be a list
		newVals, ok := value.(types.ListValue)
		if !ok {
			return fmt.Errorf("E_TYPE: list range assignment requires a list value")
		}

		length := c.Len()
		isInverted := startIdx > endIdx+1

		// Bounds check
		if !isInverted {
			if startIdx < 1 || startIdx > int64(length)+1 {
				return fmt.Errorf("E_RANGE: list range start out of bounds")
			}
			if endIdx < 0 || endIdx > int64(length) {
				return fmt.Errorf("E_RANGE: list range end out of bounds")
			}
		} else {
			if startIdx < 1 || startIdx > int64(length)+1 {
				return fmt.Errorf("E_RANGE: list range start out of bounds")
			}
			if endIdx < 0 || endIdx > int64(length) {
				return fmt.Errorf("E_RANGE: list range end out of bounds")
			}
		}

		// Build new list: [1..start-1] + newVals + [end+1..$]
		result := make([]types.Value, 0)
		for i := 1; i < int(startIdx); i++ {
			result = append(result, c.Get(i))
		}
		for i := 1; i <= newVals.Len(); i++ {
			result = append(result, newVals.Get(i))
		}
		for i := int(endIdx) + 1; i <= length; i++ {
			result = append(result, c.Get(i))
		}
		newColl = types.NewList(result)

	case types.StrValue:
		startInt, startOk := start.(types.IntValue)
		endInt, endOk := end.(types.IntValue)
		if !startOk || !endOk {
			return fmt.Errorf("E_TYPE: range indices must be integers")
		}
		startIdx := startInt.Val
		endIdx := endInt.Val

		// Value must be a string
		newStr, ok := value.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: string range assignment requires a string value")
		}

		s := c.Value()
		strLen := int64(len(s))
		isInverted := startIdx > endIdx+1

		// Bounds check
		if !isInverted {
			if startIdx < 1 || startIdx > strLen+1 {
				return fmt.Errorf("E_RANGE: string range start out of bounds")
			}
			if endIdx < 0 {
				return fmt.Errorf("E_RANGE: string range end out of bounds")
			}
		} else {
			if startIdx < 1 || startIdx > strLen+1 {
				return fmt.Errorf("E_RANGE: string range start out of bounds")
			}
			if endIdx < 0 {
				return fmt.Errorf("E_RANGE: string range end out of bounds")
			}
		}

		// Clamp endIdx to actual string length for slicing
		effectiveEnd := endIdx
		if effectiveEnd > strLen {
			effectiveEnd = strLen
		}

		// Build new string: s[1..start-1] + newStr + s[end+1..$]
		newColl = types.NewStr(s[:startIdx-1] + newStr.Value() + s[effectiveEnd:])

	case types.MapValue:
		var startIdx int64
		startInt, startIsInt := start.(types.IntValue)
		if startIsInt {
			startIdx = startInt.Val
		} else {
			switch start.(type) {
			case types.ListValue, types.MapValue:
				return fmt.Errorf("E_TYPE: range indices must be integers or map keys")
			}
			startIdx = c.KeyPosition(start)
			if startIdx == 0 {
				return fmt.Errorf("E_RANGE: map range start key not found")
			}
		}

		var endIdx int64
		endInt, endIsInt := end.(types.IntValue)
		if endIsInt {
			endIdx = endInt.Val
		} else {
			switch end.(type) {
			case types.ListValue, types.MapValue:
				return fmt.Errorf("E_TYPE: range indices must be integers or map keys")
			}
			endIdx = c.KeyPosition(end)
			if endIdx == 0 {
				return fmt.Errorf("E_RANGE: map range end key not found")
			}
		}

		// Value must be a map
		newMap, ok := value.(types.MapValue)
		if !ok {
			return fmt.Errorf("E_TYPE: map range assignment requires a map value")
		}

		length := c.Len()
		isInverted := startIdx > endIdx

		// Bounds check
		if startIdx < 1 || startIdx > int64(length)+1 {
			return fmt.Errorf("E_RANGE: map range start out of bounds")
		}
		if endIdx < 0 || endIdx > int64(length) {
			return fmt.Errorf("E_RANGE: map range end out of bounds")
		}
		if isInverted {
			if startIdx > int64(length) || endIdx < 1 {
				return fmt.Errorf("E_RANGE: map range inverted out of bounds")
			}
		}

		// Build new map: pairs[1..start-1] + newMap + pairs[end+1..$]
		pairs := c.Pairs()
		result := make([][2]types.Value, 0)
		for i := 0; i < int(startIdx)-1; i++ {
			result = append(result, pairs[i])
		}
		for _, pair := range newMap.Pairs() {
			result = append(result, pair)
		}
		for i := int(endIdx); i < length; i++ {
			result = append(result, pairs[i])
		}
		newColl = types.NewMap(result)

	default:
		return fmt.Errorf("E_TYPE: cannot range-assign to %s", coll.Type().String())
	}

	// Check size limits on the result
	switch result := newColl.(type) {
	case types.ListValue:
		if errCode := builtins.CheckListLimit(result); errCode != types.E_NONE {
			return fmt.Errorf("E_QUOTA: list too large")
		}
	case types.StrValue:
		if errCode := builtins.CheckStringLimit(result.Value()); errCode != types.E_NONE {
			return fmt.Errorf("E_QUOTA: string too long")
		}
	case types.MapValue:
		if errCode := builtins.CheckMapLimit(result); errCode != types.E_NONE {
			return fmt.Errorf("E_QUOTA: map too large")
		}
	}

	// Write modified collection back to variable slot
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
		startIdx := startInt.Val
		endIdx := endInt.Val
		length := int64(coll.Len())

		if startIdx > endIdx {
			vm.Push(types.NewList([]types.Value{}))
			return nil
		}
		if startIdx < 1 || startIdx > length {
			return fmt.Errorf("E_RANGE: list range start out of range")
		}
		if endIdx < 1 || endIdx > length {
			return fmt.Errorf("E_RANGE: list range end out of range")
		}

		result := make([]types.Value, 0, endIdx-startIdx+1)
		for i := startIdx; i <= endIdx; i++ {
			result = append(result, coll.Get(int(i)))
		}
		vm.Push(types.NewList(result))
		return nil

	case types.StrValue:
		startIdx := startInt.Val
		endIdx := endInt.Val
		s := coll.Value()
		length := int64(len(s))

		if startIdx > endIdx {
			vm.Push(types.NewStr(""))
			return nil
		}
		if startIdx < 1 || startIdx > length {
			return fmt.Errorf("E_RANGE: string range start out of range")
		}
		if endIdx < 1 || endIdx > length {
			return fmt.Errorf("E_RANGE: string range end out of range")
		}

		vm.Push(types.NewStr(s[startIdx-1 : endIdx]))
		return nil

	case types.MapValue:
		startIdx := startInt.Val
		endIdx := endInt.Val
		length := int64(coll.Len())

		if startIdx > endIdx {
			vm.Push(types.NewEmptyMap())
			return nil
		}
		if startIdx < 1 || startIdx > length {
			return fmt.Errorf("E_RANGE: map range start out of range")
		}
		if endIdx < 1 || endIdx > length {
			return fmt.Errorf("E_RANGE: map range end out of range")
		}

		pairs := coll.Pairs()
		result := make([][2]types.Value, 0, endIdx-startIdx+1)
		for i := startIdx; i <= endIdx; i++ {
			result = append(result, pairs[i-1])
		}
		vm.Push(types.NewMap(result))
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
		if !types.IsValidMapKey(key) {
			return fmt.Errorf("E_TYPE: invalid map key type")
		}
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

// executeIndexMarker resolves ^/$ markers against a collection.
// Bytecode: OP_INDEX_MARKER <marker:byte> where 0 = ^ and 1 = $.
func (vm *VM) executeIndexMarker() error {
	marker := vm.ReadByte()
	coll := vm.Pop()

	switch c := coll.(type) {
	case types.ListValue:
		if marker == 0 {
			vm.Push(types.NewInt(1))
		} else if marker == 1 {
			vm.Push(types.NewInt(int64(c.Len())))
		} else {
			return fmt.Errorf("E_INVARG: invalid index marker")
		}
		return nil

	case types.StrValue:
		if marker == 0 {
			vm.Push(types.NewInt(1))
		} else if marker == 1 {
			vm.Push(types.NewInt(int64(len(c.Value()))))
		} else {
			return fmt.Errorf("E_INVARG: invalid index marker")
		}
		return nil

	case types.MapValue:
		keys := c.Keys()
		if len(keys) == 0 {
			// Preserve empty-collection marker shape; downstream index ops return E_RANGE.
			if marker == 0 {
				vm.Push(types.NewInt(1))
			} else if marker == 1 {
				vm.Push(types.NewInt(0))
			} else {
				return fmt.Errorf("E_INVARG: invalid index marker")
			}
			return nil
		}

		sort.Slice(keys, func(i, j int) bool {
			return types.CompareMapKeys(keys[i], keys[j]) < 0
		})

		if marker == 0 {
			vm.Push(keys[0])
		} else if marker == 1 {
			vm.Push(keys[len(keys)-1])
		} else {
			return fmt.Errorf("E_INVARG: invalid index marker")
		}
		return nil

	default:
		return fmt.Errorf("E_TYPE: invalid index marker context")
	}
}

// executeListRange handles OP_LIST_RANGE: pop end, start; push {start..end} list.
// Matches tree-walker behavior: ascending if start <= end, descending if start > end.
// Accepts INT and OBJ types (OBJ treated as its ID value).
func (vm *VM) executeListRange() error {
	endVal := vm.Pop()
	startVal := vm.Pop()

	// Extract integer values (accept both INT and OBJ types, matching tree-walker)
	var start, end int64

	switch v := startVal.(type) {
	case types.IntValue:
		start = v.Val
	case types.ObjValue:
		start = int64(v.ID())
	default:
		return fmt.Errorf("E_TYPE: list range requires integer start")
	}

	switch v := endVal.(type) {
	case types.IntValue:
		end = v.Val
	case types.ObjValue:
		end = int64(v.ID())
	default:
		return fmt.Errorf("E_TYPE: list range requires integer end")
	}

	// Build the list
	var elements []types.Value
	if start <= end {
		// Ascending range
		elements = make([]types.Value, 0, end-start+1)
		for i := start; i <= end; i++ {
			elements = append(elements, types.NewInt(i))
		}
	} else {
		// Descending range
		elements = make([]types.Value, 0, start-end+1)
		for i := start; i >= end; i-- {
			elements = append(elements, types.NewInt(i))
		}
	}

	vm.Push(types.NewList(elements))
	return nil
}

// executeListAppend handles OP_LIST_APPEND: pop elem, pop list; push list with elem appended.
// Used for building lists with splices (non-splice elements).
func (vm *VM) executeListAppend() error {
	elem := vm.Pop()
	listVal := vm.Pop()

	list, ok := listVal.(types.ListValue)
	if !ok {
		return fmt.Errorf("E_TYPE: LIST_APPEND requires a list")
	}

	// Build new list with element appended
	newElems := make([]types.Value, list.Len()+1)
	for i := 1; i <= list.Len(); i++ {
		newElems[i-1] = list.Get(i)
	}
	newElems[list.Len()] = elem

	result := types.NewList(newElems)
	if errCode := builtins.CheckListLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: list too large")
	}

	vm.Push(result)
	return nil
}

// executeListExtend handles OP_LIST_EXTEND: pop src, pop list; push list with all elements of src appended.
// Used for building lists with splices (splice elements -- @list extends the accumulator).
func (vm *VM) executeListExtend() error {
	srcVal := vm.Pop()
	listVal := vm.Pop()

	list, ok := listVal.(types.ListValue)
	if !ok {
		return fmt.Errorf("E_TYPE: LIST_EXTEND requires a list base")
	}

	src, ok := srcVal.(types.ListValue)
	if !ok {
		return fmt.Errorf("E_TYPE: splice requires a list operand")
	}

	// Build new list with all elements of src appended
	newElems := make([]types.Value, list.Len()+src.Len())
	for i := 1; i <= list.Len(); i++ {
		newElems[i-1] = list.Get(i)
	}
	for i := 1; i <= src.Len(); i++ {
		newElems[list.Len()+i-1] = src.Get(i)
	}

	result := types.NewList(newElems)
	if errCode := builtins.CheckListLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: list too large")
	}

	vm.Push(result)
	return nil
}

func (vm *VM) executeSplice() error {
	val := vm.Pop()

	// Standalone @expr: operand must be a list, otherwise E_TYPE.
	if _, ok := val.(types.ListValue); !ok {
		return fmt.Errorf("E_TYPE: splice (@) requires a list operand")
	}

	vm.Push(val)
	return nil
}

func (vm *VM) executeCallBuiltin() error {
	funcID := vm.ReadByte()
	argc := vm.ReadByte()

	var args []types.Value
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		list, ok := listVal.(types.ListValue)
		if !ok {
			return fmt.Errorf("E_TYPE: expected list for spliced builtin args")
		}
		args = make([]types.Value, list.Len())
		for i := 1; i <= list.Len(); i++ {
			args[i-1] = list.Get(i)
		}
	} else {
		args = vm.PopN(int(argc))
	}

	// Call builtin
	result := vm.Builtins.CallByID(int(funcID), vm.Context, args)
	if result.Flow == types.FlowException {
		// Try to handle exception
		vmErr := VMException{Code: result.Error, Value: result.Val}
		if !vm.HandleError(vmErr) {
			return vmErr
		}
		return nil
	}

	// Handle FlowSuspend: yield control back to the caller (scheduler).
	// Push 0 onto the stack first as the return value of suspend() — when
	// Resume() is called, execution continues after the builtin call with
	// this value already on the stack.
	if result.Flow == types.FlowSuspend {
		vm.Push(types.NewInt(0)) // suspend() returns 0 in MOO
		vm.yielded = true
		vm.yieldResult = result
		return nil
	}

	vm.Push(result.Val)
	return nil
}

// Property operations

func (vm *VM) executeGetProp() error {
	propNameIdx := vm.ReadByte()

	// Determine property name: static (from constant pool) or dynamic (from stack)
	var propName string
	if propNameIdx == 0xFF {
		// Dynamic property: name is on top of stack
		nameVal := vm.Pop()
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: dynamic property name must be a string")
		}
		propName = strVal.Value()
	} else {
		// Static property: name from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[propNameIdx]
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("internal error: property name constant is not a string")
		}
		propName = strVal.Value()
	}

	// Pop the object
	objVal := vm.Pop()

	// Check if it's a waif (must check before ObjValue since waifs are a different type)
	if waifVal, ok := objVal.(types.WaifValue); ok {
		return vm.vmGetWaifProp(waifVal, propName)
	}

	// Check if it's an object reference
	objRef, ok := objVal.(types.ObjValue)
	if !ok {
		return fmt.Errorf("E_TYPE: property access requires an object")
	}

	objID := objRef.ID()

	// Need a store to look up properties
	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	obj := vm.Store.Get(objID)
	if obj == nil {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Look up defined property first (with inheritance via breadth-first search)
	prop, errCode := vmFindProperty(vm.Store, obj, propName)
	if errCode == types.E_NONE {
		// Check read permission
		if err := vm.checkPropertyReadPerm(prop); err != nil {
			return err
		}
		vm.Push(prop.Value)
		return nil
	}

	// Check for built-in properties (flag properties like .name, .owner, .wizard, etc.)
	if val, ok := vmGetBuiltinProperty(obj, propName); ok {
		vm.Push(val)
		return nil
	}

	// Property not found
	return fmt.Errorf("E_PROPNF: property not found: %s", propName)
}

// vmGetWaifProp handles property read on a waif value.
// Mirrors the tree-walker's waifProperty() logic.
func (vm *VM) vmGetWaifProp(waif types.WaifValue, propName string) error {
	// Special waif properties
	switch propName {
	case "owner":
		vm.Push(types.NewObj(waif.Owner()))
		return nil
	case "class":
		classID := waif.Class()
		// Check if class object has been recycled
		if vm.Store != nil {
			classObj := vm.Store.Get(classID)
			if classObj == nil {
				// Class has been recycled - return #-1
				vm.Push(types.NewObj(types.ObjNothing))
				return nil
			}
		}
		vm.Push(types.NewObj(classID))
		return nil
	}

	// Check waif's own properties first
	if val, ok := waif.GetProperty(propName); ok {
		vm.Push(val)
		return nil
	}

	// Fall back to class object's properties
	if vm.Store == nil {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	classID := waif.Class()
	classObj := vm.Store.Get(classID)
	if classObj == nil {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Look up property on class object
	prop, errCode := vmFindProperty(vm.Store, classObj, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	vm.Push(prop.Value)
	return nil
}

func (vm *VM) executeSetProp() error {
	propNameIdx := vm.ReadByte()

	// Determine property name: static (from constant pool) or dynamic (from stack)
	var propName string
	if propNameIdx == 0xFF {
		// Dynamic property: name is on top of stack, then obj, then value_copy
		nameVal := vm.Pop()
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: dynamic property name must be a string")
		}
		propName = strVal.Value()
	} else {
		// Static property: name from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[propNameIdx]
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("internal error: property name constant is not a string")
		}
		propName = strVal.Value()
	}

	// Pop the object
	objVal := vm.Pop()

	// Pop the value to assign
	value := vm.Pop()

	// Check if it's a waif (must check before ObjValue since waifs are a different type)
	if waifVal, ok := objVal.(types.WaifValue); ok {
		return vm.vmSetWaifProp(waifVal, propName, value)
	}

	// Check if it's an object reference
	objRef, ok := objVal.(types.ObjValue)
	if !ok {
		return fmt.Errorf("E_TYPE: property assignment requires an object")
	}

	objID := objRef.ID()

	// Need a store to set properties
	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	obj := vm.Store.Get(objID)
	if obj == nil {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Check for built-in property assignment first
	if isBuiltin, errCode := vmSetBuiltinProperty(obj, propName, value, vm.Context); isBuiltin {
		if errCode != types.E_NONE {
			return fmt.Errorf("E_%s: cannot set built-in property %s", errCode, propName)
		}
		return nil
	}

	// Check if property exists directly on this object
	prop, ok := obj.Properties[propName]
	if ok {
		// Check write permission
		if err := vm.checkPropertyWritePerm(prop); err != nil {
			return err
		}
		// Property exists locally - update it
		prop.Clear = false
		prop.Value = value
		return nil
	}

	// Property not on this object - check if inherited
	inheritedProp, errCode := vmFindProperty(vm.Store, obj, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Check write permission on the inherited property
	if err := vm.checkPropertyWritePerm(inheritedProp); err != nil {
		return err
	}

	// Property is inherited - create a local copy with the new value
	newProp := &db.Property{
		Name:    propName,
		Value:   value,
		Owner:   inheritedProp.Owner,
		Perms:   inheritedProp.Perms,
		Clear:   false,
		Defined: false,
	}
	obj.Properties[propName] = newProp

	return nil
}

// vmSetWaifProp handles property assignment on a waif value.
// Mirrors the tree-walker's assignWaifProperty() logic.
func (vm *VM) vmSetWaifProp(waif types.WaifValue, propName string, value types.Value) error {
	// These properties cannot be set on waifs
	switch propName {
	case "owner", "class", "wizard", "programmer":
		return fmt.Errorf("E_PERM: cannot set .%s on a waif", propName)
	}

	// Check for self-reference (circular reference)
	if containsWaif(value, waif) {
		return fmt.Errorf("E_RECMOVE: value contains the waif itself")
	}

	// Set property on waif (creates a new waif with the property set)
	// Note: Waifs use copy-on-write semantics. The VM does not currently
	// propagate the new waif back to the source variable. This matches
	// the tree-walker's limitation for non-simple-identifier cases.
	_ = waif.SetProperty(propName, value)

	return nil
}

// checkPropertyReadPerm checks if the current programmer has read permission on a property.
// Wizards and property owners always have access.
func (vm *VM) checkPropertyReadPerm(prop *db.Property) error {
	if vm.Context == nil {
		return nil // No context = no permission check
	}
	if vm.Context.IsWizard {
		return nil
	}
	if vm.Context.Programmer == prop.Owner {
		return nil
	}
	if !prop.Perms.Has(db.PropRead) {
		return fmt.Errorf("E_PERM: property not readable")
	}
	return nil
}

// checkPropertyWritePerm checks if the current programmer has write permission on a property.
// Wizards and property owners always have access.
func (vm *VM) checkPropertyWritePerm(prop *db.Property) error {
	if vm.Context == nil {
		return nil // No context = no permission check
	}
	if vm.Context.IsWizard {
		return nil
	}
	if vm.Context.Programmer == prop.Owner {
		return nil
	}
	if !prop.Perms.Has(db.PropWrite) {
		return fmt.Errorf("E_PERM: property not writable")
	}
	return nil
}

// vmFindProperty finds a property on an object with inheritance (breadth-first search).
// This mirrors the tree-walker's findProperty logic.
func vmFindProperty(store *db.Store, obj *db.Object, name string) (*db.Property, types.ErrorCode) {
	queue := []types.ObjID{obj.ID}
	visited := make(map[types.ObjID]bool)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := store.Get(currentID)
		if current == nil {
			continue
		}

		prop, ok := current.Properties[name]
		if ok && !prop.Clear {
			return prop, types.E_NONE
		}

		queue = append(queue, current.Parents...)
	}

	return nil, types.E_PROPNF
}

// vmGetBuiltinProperty returns built-in object properties (name, owner, location, etc.).
// This mirrors the tree-walker's getBuiltinProperty logic.
func vmGetBuiltinProperty(obj *db.Object, name string) (types.Value, bool) {
	switch name {
	case "name":
		return types.NewStr(obj.Name), true
	case "owner":
		return types.NewObj(obj.Owner), true
	case "location":
		return types.NewObj(obj.Location), true
	case "contents":
		vals := make([]types.Value, len(obj.Contents))
		for i, id := range obj.Contents {
			vals[i] = types.NewObj(id)
		}
		return types.NewList(vals), true
	case "parents":
		vals := make([]types.Value, len(obj.Parents))
		for i, id := range obj.Parents {
			vals[i] = types.NewObj(id)
		}
		return types.NewList(vals), true
	case "parent":
		if len(obj.Parents) > 0 {
			return types.NewObj(obj.Parents[0]), true
		}
		return types.NewObj(types.ObjNothing), true
	case "children":
		vals := make([]types.Value, len(obj.Children))
		for i, id := range obj.Children {
			vals[i] = types.NewObj(id)
		}
		return types.NewList(vals), true
	case "programmer":
		if obj.Flags.Has(db.FlagProgrammer) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "wizard":
		if obj.Flags.Has(db.FlagWizard) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "player":
		if obj.Flags.Has(db.FlagUser) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "r":
		if obj.Flags.Has(db.FlagRead) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "w":
		if obj.Flags.Has(db.FlagWrite) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "f":
		if obj.Flags.Has(db.FlagFertile) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "a":
		if obj.Flags.Has(db.FlagAnonymous) || obj.Anonymous {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	default:
		return nil, false
	}
}

// vmSetBuiltinProperty sets a built-in object property.
// This mirrors the tree-walker's setBuiltinProperty logic.
func vmSetBuiltinProperty(obj *db.Object, name string, value types.Value, ctx *types.TaskContext) (bool, types.ErrorCode) {
	switch name {
	case "name":
		if str, ok := value.(types.StrValue); ok {
			obj.Name = str.Value()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "owner":
		if objVal, ok := value.(types.ObjValue); ok {
			if obj.Anonymous && ctx != nil && !ctx.IsWizard {
				return true, types.E_PERM
			}
			obj.Owner = objVal.ID()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "location":
		if objVal, ok := value.(types.ObjValue); ok {
			obj.Location = objVal.ID()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "programmer":
		if intVal, ok := value.(types.IntValue); ok {
			if obj.Anonymous {
				if ctx != nil && ctx.IsWizard {
					return true, types.E_INVARG
				}
				return true, types.E_PERM
			}
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagProgrammer)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagProgrammer)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "wizard":
		if intVal, ok := value.(types.IntValue); ok {
			if obj.Anonymous {
				if ctx != nil && ctx.IsWizard {
					return true, types.E_INVARG
				}
				return true, types.E_PERM
			}
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagWizard)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagWizard)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "player":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagUser)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagUser)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "r":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagRead)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagRead)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "w":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagWrite)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagWrite)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "f":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagFertile)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagFertile)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "a":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagAnonymous)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagAnonymous)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	default:
		return false, types.E_NONE
	}
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

	if aIsFloat && bIsFloat {
		if aFloat.Val < bFloat.Val {
			return -1, nil
		} else if aFloat.Val > bFloat.Val {
			return 1, nil
		}
		return 0, nil
	}

	if (aIsInt && bIsFloat) || (aIsFloat && bIsInt) {
		return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
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

// executeIterPrep handles OP_ITER_PREP: normalize a container for iteration.
//
// Bytecode format: OP_ITER_PREP <hasIndex:byte>
//
// Pops the container from the stack.
// Pushes two values: the normalized list and an isPairs flag (1 = pairs, 0 = plain).
//
// Behavior by type:
//   - List + no index: push list as-is, push 0
//   - List + has index: push list of {element, position} pairs, push 1
//   - Map (always): sort pairs by key, push list of {value, key} pairs, push 1
//   - String + no index: push list of single-char strings, push 0
//   - String + has index: push list of {char, position} pairs, push 1
//   - Other: E_TYPE
func (vm *VM) executeIterPrep() error {
	hasIndex := vm.ReadByte() != 0
	container := vm.Pop()

	switch c := container.(type) {
	case types.ListValue:
		if hasIndex {
			// Wrap each element in {element, 1-based-index}
			elements := make([]types.Value, c.Len())
			for i := 1; i <= c.Len(); i++ {
				pair := types.NewList([]types.Value{c.Get(i), types.IntValue{Val: int64(i)}})
				elements[i-1] = pair
			}
			vm.Push(types.NewList(elements))
			vm.Push(types.IntValue{Val: 1})
		} else {
			// Pass through as-is
			vm.Push(c)
			vm.Push(types.IntValue{Val: 0})
		}

	case types.MapValue:
		// Sort pairs by key in MOO canonical order, produce {value, key} pairs
		pairs := c.Pairs()
		sortForMapPairs(pairs)
		elements := make([]types.Value, len(pairs))
		for i, pair := range pairs {
			// pair[0] = key, pair[1] = value
			elements[i] = types.NewList([]types.Value{pair[1], pair[0]})
		}
		vm.Push(types.NewList(elements))
		vm.Push(types.IntValue{Val: 1})

	case types.StrValue:
		s := c.Value()
		runes := []rune(s)
		if hasIndex {
			// Produce {char, 1-based-index} pairs
			elements := make([]types.Value, len(runes))
			for i, r := range runes {
				pair := types.NewList([]types.Value{types.NewStr(string(r)), types.IntValue{Val: int64(i + 1)}})
				elements[i] = pair
			}
			vm.Push(types.NewList(elements))
			vm.Push(types.IntValue{Val: 1})
		} else {
			// Convert to list of single-char strings
			elements := make([]types.Value, len(runes))
			for i, r := range runes {
				elements[i] = types.NewStr(string(r))
			}
			vm.Push(types.NewList(elements))
			vm.Push(types.IntValue{Val: 0})
		}

	default:
		return fmt.Errorf("E_TYPE: for loop requires list, map, or string")
	}

	return nil
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

// setLocalByName sets a local variable in a stack frame by name.
// Looks up the name in the program's VarNames table and sets the corresponding
// slot in frame.Locals. If the name is not found (verb doesn't reference it),
// silently does nothing.
func setLocalByName(frame *StackFrame, prog *Program, name string, value types.Value) {
	for i, varName := range prog.VarNames {
		if varName == name {
			frame.Locals[i] = value
			return
		}
	}
}

// executeCallVerb handles OP_CALL_VERB: call a verb on an object.
//
// Bytecode format: OP_CALL_VERB <verb_name_idx:byte> <argc:byte>
// verb_name_idx = 0xFF means dynamic (verb name string is on top of stack, above args).
//
// Stack layout (top to bottom):
//
//	[verb_name_str] (only if dynamic, i.e. verb_name_idx == 0xFF)
//	arg_N
//	...
//	arg_1
//	obj
//
// Native frame push: compiles the verb to bytecode and pushes a new StackFrame.
// Returns a compile error if bytecode compilation fails.
func (vm *VM) executeCallVerb() error {
	verbNameIdx := vm.ReadByte()
	argc := int(vm.ReadByte())

	// Resolve verb name
	var verbName string
	if verbNameIdx == 0xFF {
		// Dynamic verb name: pop from stack (above args)
		nameVal := vm.Pop()
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: dynamic verb name must be a string")
		}
		verbName = strVal.Value()
	} else {
		// Static verb name: from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[verbNameIdx]
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("internal error: verb name constant is not a string")
		}
		verbName = strVal.Value()
	}

	// Pop arguments
	var args []types.Value
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		list, ok := listVal.(types.ListValue)
		if !ok {
			return fmt.Errorf("E_TYPE: expected list for spliced verb args")
		}
		args = make([]types.Value, list.Len())
		for i := 1; i <= list.Len(); i++ {
			args[i-1] = list.Get(i)
		}
	} else {
		args = vm.PopN(argc)
	}

	// Pop the object
	objVal := vm.Pop()

	// Resolve the object ID from the target value.
	// Handles ObjValue (including anonymous), WaifValue, and primitive prototypes.
	var objID types.ObjID
	var thisValue types.Value // Non-nil for waif, primitive, and anonymous targets

	switch target := objVal.(type) {
	case types.ObjValue:
		objID = target.ID()
		if target.IsAnonymous() {
			thisValue = target // "this" = the anonymous ObjValue itself
		}
	case types.WaifValue:
		objID = target.Class() // Verb lookup goes to the waif's class
		thisValue = target     // "this" = the waif itself
	default:
		// Check for primitive prototype dispatch (str, int, float, list, map, err, bool)
		if vm.Store != nil {
			protoID := getPrimitivePrototypeFromStore(vm.Store, objVal)
			if protoID != types.ObjNothing {
				objID = protoID
				thisValue = objVal // "this" = the primitive value itself
			} else {
				return fmt.Errorf("E_TYPE: verb call requires an object")
			}
		} else {
			return fmt.Errorf("E_TYPE: verb call requires an object")
		}
	}

	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	// Check object validity
	if !vm.Store.Valid(objID) {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Look up verb via store (with inheritance)
	verb, defObjID, err := vm.Store.FindVerb(objID, verbName)
	if err != nil {
		return fmt.Errorf("E_VERBNF: verb not found: %s", verbName)
	}

	// Check execute permission
	if !verb.Perms.Has(db.VerbExecute) {
		return fmt.Errorf("E_PERM: verb %s is not executable", verbName)
	}

	// Try to compile verb to bytecode
	prog, compileErr := CompileVerbBytecode(verb, vm.Builtins)
	if compileErr != nil {
		return fmt.Errorf("E_VERBNF: compile error in %s: %v", verbName, compileErr)
	}

	// --- Native frame push ---

	// Get current frame's context for caller/player
	currentFrame := vm.CurrentFrame()
	callerObj := currentFrame.This
	player := currentFrame.Player
	if vm.Context != nil && vm.Context.Player != types.ObjNothing {
		player = vm.Context.Player
	}

	// Save current context fields for restore on return/unwind
	var savedThisObj types.ObjID
	var savedThisValue types.Value
	var savedVerb string
	if vm.Context != nil {
		savedThisObj = vm.Context.ThisObj
		savedThisValue = vm.Context.ThisValue
		savedVerb = vm.Context.Verb
	}

	// Push new stack frame
	frame := &StackFrame{
		Program:        prog,
		IP:             0,
		BasePointer:    vm.SP,
		Locals:         make([]types.Value, prog.NumLocals),
		This:           objID,
		Player:         player,
		Verb:           verbName,
		Caller:         callerObj,
		VerbLoc:        defObjID,
		Args:           args,
		LoopStack:      make([]LoopState, 0, 4),
		ExceptStack:    make([]Handler, 0, 4),
		IsVerbCall:     true,
		SavedThisObj:   savedThisObj,
		SavedThisValue: savedThisValue,
		SavedVerb:      savedVerb,
	}

	// Initialize locals to 0
	for i := range frame.Locals {
		frame.Locals[i] = types.IntValue{Val: 0}
	}

	// Pre-populate built-in variables using VarNames.
	// For waif/primitive/anonymous targets, "this" is the actual value, not NewObj(objID).
	if thisValue != nil {
		setLocalByName(frame, prog, "this", thisValue)
	} else {
		setLocalByName(frame, prog, "this", types.NewObj(objID))
	}
	setLocalByName(frame, prog, "verb", types.NewStr(verbName))
	setLocalByName(frame, prog, "caller", types.NewObj(callerObj))
	setLocalByName(frame, prog, "args", types.NewList(args))
	setLocalByName(frame, prog, "player", types.NewObj(player))

	// Propagate command environment variables from the task's parsed command context.
	// These are set by the scheduler for command-dispatched verbs and should be
	// available in nested verb calls. setLocalByName silently skips if the verb
	// code doesn't reference the variable, so there's no overhead for verbs that
	// don't use them.
	if vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			setLocalByName(frame, prog, "argstr", types.NewStr(t.Argstr))
			setLocalByName(frame, prog, "dobjstr", types.NewStr(t.Dobjstr))
			setLocalByName(frame, prog, "iobjstr", types.NewStr(t.Iobjstr))
			setLocalByName(frame, prog, "prepstr", types.NewStr(t.Prepstr))
			setLocalByName(frame, prog, "dobj", types.NewObj(t.Dobj))
			setLocalByName(frame, prog, "iobj", types.NewObj(t.Iobj))
		}
	}

	// Update shared context for builtins
	if vm.Context != nil {
		vm.Context.ThisObj = objID
		vm.Context.ThisValue = thisValue // waif/primitive/anonymous value, or nil for normal
		vm.Context.Verb = verbName
	}

	// Push activation frame onto task call stack (if we have a task)
	if vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			actFrame := task.ActivationFrame{
				This:       objID,
				ThisValue:  thisValue, // Store waif/primitive/anonymous value for callers()/queued_tasks()
				Player:     player,
				Programmer: verb.Owner,
				Caller:     callerObj,
				Verb:       verbName,
				VerbLoc:    defObjID,
				Args:       args,
				LineNumber: 0,
			}
			t.PushFrame(actFrame)
		}
	}

	vm.Frames = append(vm.Frames, frame)

	// Return nil — Run() loop continues executing the new frame's bytecode
	return nil
}

// executePass handles OP_PASS: call the same verb on the parent object.
//
// Bytecode format: OP_PASS <argc:byte>
// argc = 0: inherit current frame's args
// argc > 0: pop argc args from stack
//
// Looks up the parent of VerbLoc (where the current verb is defined),
// finds the same verb name on an ancestor, compiles it to bytecode,
// and pushes a new frame. Preserves `this` (original target).
func (vm *VM) executePass() error {
	argc := int(vm.ReadByte())

	frame := vm.CurrentFrame()
	if frame == nil {
		return fmt.Errorf("E_INVIND: no active frame for pass()")
	}

	verbName := frame.Verb
	if verbName == "" {
		return fmt.Errorf("E_INVIND: pass() called outside of a verb")
	}

	verbLoc := frame.VerbLoc
	if verbLoc == types.ObjNothing {
		return fmt.Errorf("E_INVIND: pass() has no defining object")
	}

	// Get pass-through args
	var passArgs []types.Value
	if argc > 0 {
		passArgs = vm.PopN(argc)
	} else {
		// Inherit args from current frame's stored Args
		if frame.Args != nil {
			passArgs = frame.Args
		} else {
			passArgs = []types.Value{}
		}
	}

	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	// Get the object where the current verb is defined
	verbLocObj := vm.Store.Get(verbLoc)
	if verbLocObj == nil {
		return fmt.Errorf("E_INVIND: defining object #%d not found", verbLoc)
	}

	// No parents = no parent verb to call
	if len(verbLocObj.Parents) == 0 {
		return fmt.Errorf("E_VERBNF: no parent verb for pass()")
	}

	// Search for the verb on parent(s), starting from verbLoc's parents
	// Uses the store's FindVerb for each parent to do BFS through ancestors
	var verb *db.Verb
	var defObjID types.ObjID

	visited := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, len(verbLocObj.Parents))
	copy(queue, verbLocObj.Parents)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		obj := vm.Store.Get(current)
		if obj == nil || obj.Recycled {
			continue
		}

		// Check if verb exists on this object (exact name or alias match)
		if v, ok := obj.Verbs[verbName]; ok {
			verb = v
			defObjID = current
			break
		}
		// Check aliases
		for _, v := range obj.Verbs {
			for _, alias := range v.Names {
				if alias == verbName {
					verb = v
					defObjID = current
					break
				}
			}
			if verb != nil {
				break
			}
		}
		if verb != nil {
			break
		}

		queue = append(queue, obj.Parents...)
	}

	if verb == nil {
		return fmt.Errorf("E_VERBNF: no parent verb for pass()")
	}

	// Check execute permission
	if !verb.Perms.Has(db.VerbExecute) {
		return fmt.Errorf("E_PERM: parent verb %s is not executable", verbName)
	}

	// Compile the parent verb to bytecode
	prog, compileErr := CompileVerbBytecode(verb, vm.Builtins)
	if compileErr != nil {
		return fmt.Errorf("E_VERBNF: compile error in pass() for %s: %v", verbName, compileErr)
	}

	// --- Native frame push ---

	// Save current context fields for restore on return/unwind
	var savedThisObj types.ObjID
	var savedThisValue types.Value
	var savedVerb string
	if vm.Context != nil {
		savedThisObj = vm.Context.ThisObj
		savedThisValue = vm.Context.ThisValue
		savedVerb = vm.Context.Verb
	}

	// Push new stack frame with parent verb's bytecode
	// this = current frame's this (preserve original target)
	// VerbLoc = defObjID (where the parent verb was found, for chained pass())
	newFrame := &StackFrame{
		Program:        prog,
		IP:             0,
		BasePointer:    vm.SP,
		Locals:         make([]types.Value, prog.NumLocals),
		This:           frame.This,
		Player:         frame.Player,
		Verb:           verbName,
		Caller:         verbLoc,
		VerbLoc:        defObjID,
		Args:           passArgs,
		LoopStack:      make([]LoopState, 0, 4),
		ExceptStack:    make([]Handler, 0, 4),
		IsVerbCall:     true,
		SavedThisObj:   savedThisObj,
		SavedThisValue: savedThisValue,
		SavedVerb:      savedVerb,
	}

	// Initialize locals to 0
	for i := range newFrame.Locals {
		newFrame.Locals[i] = types.IntValue{Val: 0}
	}

	// Pre-populate built-in variables
	setLocalByName(newFrame, prog, "this", types.NewObj(frame.This))
	setLocalByName(newFrame, prog, "verb", types.NewStr(verbName))
	setLocalByName(newFrame, prog, "caller", types.NewObj(verbLoc))
	setLocalByName(newFrame, prog, "args", types.NewList(passArgs))
	setLocalByName(newFrame, prog, "player", types.NewObj(frame.Player))

	// Update shared context for builtins
	if vm.Context != nil {
		vm.Context.ThisObj = frame.This
		vm.Context.Verb = verbName
	}

	// Push activation frame onto task call stack (if we have a task)
	if vm.Context != nil && vm.Context.Task != nil {
		if t, ok := vm.Context.Task.(*task.Task); ok {
			actFrame := task.ActivationFrame{
				This:       frame.This,
				Player:     frame.Player,
				Programmer: verb.Owner,
				Caller:     verbLoc,
				Verb:       verbName,
				VerbLoc:    defObjID,
				Args:       passArgs,
				LineNumber: 0,
			}
			t.PushFrame(actFrame)
		}
	}

	vm.Frames = append(vm.Frames, newFrame)

	// Return nil — Run() loop continues executing the new frame's bytecode
	return nil
}

// getPrimitivePrototypeFromStore returns the prototype object ID for a primitive value
// by reading the appropriate property (str_proto, int_proto, etc.) from #0.
// Returns ObjNothing if no prototype is configured for this type.
// This mirrors Evaluator.getPrimitivePrototype() but takes a *db.Store directly.
func getPrimitivePrototypeFromStore(store *db.Store, val types.Value) types.ObjID {
	sysObj := store.Get(0)
	if sysObj == nil {
		return types.ObjNothing
	}

	var propName string
	switch val.(type) {
	case types.IntValue:
		propName = "int_proto"
	case types.FloatValue:
		propName = "float_proto"
	case types.StrValue:
		propName = "str_proto"
	case types.ListValue:
		propName = "list_proto"
	case types.MapValue:
		propName = "map_proto"
	case types.ErrValue:
		propName = "err_proto"
	case types.BoolValue:
		propName = "bool_proto"
	default:
		return types.ObjNothing
	}

	prop, ok := sysObj.Properties[propName]
	if !ok || prop == nil {
		return types.ObjNothing
	}

	if objVal, ok := prop.Value.(types.ObjValue); ok {
		protoID := objVal.ID()
		if store.Valid(protoID) {
			return protoID
		}
	}

	return types.ObjNothing
}
