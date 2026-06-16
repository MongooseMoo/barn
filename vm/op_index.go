package vm

import (
	"barn/builtins"
	"barn/types"
	"fmt"
	"sort"
)

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
		case types.E_QUOTA:
			return fmt.Errorf("E_QUOTA: value too large")
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
// MOO ranges ascend if start <= end and descend if start > end.
// Accepts INT and OBJ types (OBJ treated as its ID value).
func (vm *VM) executeListRange() error {
	endVal := vm.Pop()
	startVal := vm.Pop()

	// Extract integer values; object IDs are accepted as integer-like indices.
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
