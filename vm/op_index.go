package vm

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func (vm *VM) executeIndex() error {
	index := vm.Pop()
	collection := vm.Pop()

	switch collection.Type() {
	case types.TYPE_LIST:
		if index.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: list index must be integer")
		}
		indexVal := index.Int()
		if indexVal < 1 || indexVal > int64(collection.Len()) {
			return fmt.Errorf("E_RANGE: list index out of range")
		}
		vm.Push(collection.Get(int(indexVal)))
		return nil

	case types.TYPE_STR:
		if index.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: string index must be integer")
		}
		indexVal := index.Int()
		if indexVal < 1 || indexVal > int64(len(collection.Str())) {
			return fmt.Errorf("E_RANGE: string index out of range")
		}
		vm.Push(types.NewStr(string(collection.Str()[indexVal-1])))
		return nil

	case types.TYPE_MAP:
		// Map keys must be scalar types (not list or map)
		switch index.Type() {
		case types.TYPE_LIST, types.TYPE_MAP:
			return fmt.Errorf("E_TYPE: invalid map key type")
		}
		val, ok := collection.MapGet(index)
		if !ok {
			return fmt.Errorf("E_RANGE: map key not found")
		}
		vm.Push(val)
		return nil

	case types.TYPE_WAIF:
		if vm.Store == nil {
			return fmt.Errorf("E_INVIND: no object store available")
		}
		owner, errCode := vm.Context.StoreTxn.ObjectOwner(collection.Class())
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: invalid waif class", errCode.String())
		}
		ownerIsWizard, errCode := hasObjectFlagForRead(vm.Context.StoreTxn, owner, dbstore.FlagWizard)
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: invalid waif class owner", errCode.String())
		}
		if !ownerIsWizard {
			return fmt.Errorf("E_TYPE: waif class owner is not a wizard")
		}
		err := vm.startVerbCall(collection, "_index", []types.Value{index})
		if err != nil && err.Error() == "E_VERBNF: verb not found: _index" {
			return fmt.Errorf("E_TYPE: waif has no _index handler")
		}
		return err

	default:
		return fmt.Errorf("E_TYPE: cannot index %s", collection.Type().String())
	}
}

func (vm *VM) executeIndexSet() error {
	// Bytecode: OP_INDEX_SET <varIdx:byte>
	// Stack: [... value_copy index] (value_copy and index on top)
	// After: modifies collection in locals[varIdx], pops index and value_copy
	varIdx := vm.FetchByte()
	index := vm.Pop()
	value := vm.Pop()

	// Read the collection from the variable slot
	coll := vm.CurrentFrame().Locals[varIdx]
	if coll.Type() == types.TYPE_WAIF {
		if err := vm.startVerbCall(coll, "_set_index", []types.Value{index, value}); err != nil {
			return err
		}
		vm.CurrentFrame().DiscardReturn = true
		return nil
	}

	// Perform the index assignment using the shared setAtIndex helper
	newColl, errCode := setAtIndex(vm.Builtins, vm.Context, coll, index, value)
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
			//lint:ignore ST1005 The E_* prefix is parsed by the VM error handler.
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
	varIdx := vm.FetchByte()
	end := vm.Pop()
	start := vm.Pop()
	value := vm.Pop()

	// Read the collection from the variable slot
	coll := vm.CurrentFrame().Locals[varIdx]

	// Perform range assignment based on collection type
	var newColl types.Value
	switch coll.Type() {
	case types.TYPE_LIST:
		if start.Type() != types.TYPE_INT || end.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: range indices must be integers")
		}
		startIdx := start.Int()
		endIdx := end.Int()

		// Value must be a list
		if value.Type() != types.TYPE_LIST {
			return fmt.Errorf("E_TYPE: list range assignment requires a list value")
		}
		newVals := value

		length := coll.Len()

		// Bounds check
		if startIdx < 1 || startIdx > int64(length)+1 {
			return fmt.Errorf("E_RANGE: list range start out of bounds")
		}
		if endIdx < 0 {
			return fmt.Errorf("E_RANGE: list range end out of bounds")
		}

		// Build new list: [1..start-1] + newVals + [end+1..$]
		result := make([]types.Value, 0)
		for i := 1; i < int(startIdx); i++ {
			result = append(result, coll.Get(i))
		}
		for i := 1; i <= newVals.Len(); i++ {
			result = append(result, newVals.Get(i))
		}
		for i := int(endIdx) + 1; i <= length; i++ {
			result = append(result, coll.Get(i))
		}
		newColl = types.NewList(result)

	case types.TYPE_STR:
		if start.Type() != types.TYPE_INT || end.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: range indices must be integers")
		}
		startIdx := start.Int()
		endIdx := end.Int()

		// Value must be a string
		if value.Type() != types.TYPE_STR {
			return fmt.Errorf("E_TYPE: string range assignment requires a string value")
		}
		newStr := value

		s := coll.Str()
		strLen := int64(len(s))

		// Bounds check
		if startIdx < 1 || startIdx > strLen+1 {
			return fmt.Errorf("E_RANGE: string range start out of bounds")
		}
		if endIdx < 0 {
			return fmt.Errorf("E_RANGE: string range end out of bounds")
		}

		// Clamp endIdx to actual string length for slicing
		effectiveEnd := endIdx
		if effectiveEnd > strLen {
			effectiveEnd = strLen
		}

		// Build new string: s[1..start-1] + newStr + s[end+1..$]
		newColl = types.NewStr(s[:startIdx-1] + newStr.Str() + s[effectiveEnd:])

	case types.TYPE_MAP:
		var startIdx int64
		if start.Type() == types.TYPE_INT {
			startIdx = start.Int()
		} else {
			switch start.Type() {
			case types.TYPE_LIST, types.TYPE_MAP:
				return fmt.Errorf("E_TYPE: range indices must be integers or map keys")
			}
			startIdx = coll.KeyPosition(start)
			if startIdx == 0 {
				return fmt.Errorf("E_RANGE: map range start key not found")
			}
		}

		var endIdx int64
		if end.Type() == types.TYPE_INT {
			endIdx = end.Int()
		} else {
			switch end.Type() {
			case types.TYPE_LIST, types.TYPE_MAP:
				return fmt.Errorf("E_TYPE: range indices must be integers or map keys")
			}
			endIdx = coll.KeyPosition(end)
			if endIdx == 0 {
				return fmt.Errorf("E_RANGE: map range end key not found")
			}
		}

		// Value must be a map
		if value.Type() != types.TYPE_MAP {
			return fmt.Errorf("E_TYPE: map range assignment requires a map value")
		}
		newMap := value

		length := coll.Len()
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
		pairs := coll.Pairs()
		result := make([][2]types.Value, 0)
		for i := 0; i < int(startIdx)-1; i++ {
			result = append(result, pairs[i])
		}
		result = append(result, newMap.Pairs()...)
		for i := int(endIdx); i < length; i++ {
			result = append(result, pairs[i])
		}
		newColl = types.NewMap(result)

	default:
		return fmt.Errorf("E_TYPE: cannot range-assign to %s", coll.Type().String())
	}

	// Check size limits on the result
	switch newColl.Type() {
	case types.TYPE_LIST:
		if errCode := vm.Builtins.CheckListLimit(newColl); errCode != types.E_NONE {
			return fmt.Errorf("E_QUOTA: list too large")
		}
	case types.TYPE_STR:
		if errCode := vm.Builtins.CheckStringLimit(newColl.Str()); errCode != types.E_NONE {
			return fmt.Errorf("E_QUOTA: string too long")
		}
	case types.TYPE_MAP:
		if errCode := vm.Builtins.CheckListLimitForTask(vm.Context, newColl); errCode != types.E_NONE {
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

	switch collection.Type() {
	case types.TYPE_LIST:
		if start.Type() != types.TYPE_INT || end.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: range indices must be integers")
		}
		startIdx := start.Int()
		endIdx := end.Int()
		length := int64(collection.Len())

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
			result = append(result, collection.Get(int(i)))
		}
		vm.Push(types.NewList(result))
		return nil

	case types.TYPE_STR:
		if start.Type() != types.TYPE_INT || end.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: range indices must be integers")
		}
		startIdx := start.Int()
		endIdx := end.Int()
		s := collection.Str()
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

	case types.TYPE_MAP:
		var startIdx int64
		if start.Type() == types.TYPE_INT {
			startIdx = start.Int()
		} else {
			if !types.IsValidMapKey(start) {
				return fmt.Errorf("E_TYPE: range indices must be integers or map keys")
			}
			startIdx = collection.KeyPosition(start)
			if startIdx == 0 {
				return fmt.Errorf("E_RANGE: map range start key not found")
			}
		}

		var endIdx int64
		if end.Type() == types.TYPE_INT {
			endIdx = end.Int()
		} else {
			if !types.IsValidMapKey(end) {
				return fmt.Errorf("E_TYPE: range indices must be integers or map keys")
			}
			endIdx = collection.KeyPosition(end)
			if endIdx == 0 {
				return fmt.Errorf("E_RANGE: map range end key not found")
			}
		}
		length := int64(collection.Len())

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

		pairs := collection.Pairs()
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

// executeIndexMarker resolves ^/$ markers against a collection. Index markers
// resolve map boundaries to keys; range markers remain positional.
func (vm *VM) executeIndexMarker() error {
	marker := vm.FetchByte()
	coll := vm.Pop()
	if marker == bytecode.RangeMarkerFirst || marker == bytecode.RangeMarkerLast {
		if coll.Type() != types.TYPE_LIST && coll.Type() != types.TYPE_STR && coll.Type() != types.TYPE_MAP {
			return fmt.Errorf("E_TYPE: invalid range marker context")
		}
		if marker == bytecode.RangeMarkerFirst {
			vm.Push(types.NewInt(1))
		} else {
			vm.Push(types.NewInt(int64(coll.Len())))
		}
		return nil
	}

	switch coll.Type() {
	case types.TYPE_LIST:
		if marker == bytecode.IndexMarkerFirst {
			vm.Push(types.NewInt(1))
		} else if marker == bytecode.IndexMarkerLast {
			vm.Push(types.NewInt(int64(coll.Len())))
		} else {
			return fmt.Errorf("E_INVARG: invalid index marker")
		}
		return nil

	case types.TYPE_STR:
		if marker == bytecode.IndexMarkerFirst {
			vm.Push(types.NewInt(1))
		} else if marker == bytecode.IndexMarkerLast {
			vm.Push(types.NewInt(int64(len(coll.Str()))))
		} else {
			return fmt.Errorf("E_INVARG: invalid index marker")
		}
		return nil

	case types.TYPE_MAP:
		keys := coll.Keys()
		if len(keys) == 0 {
			// Preserve empty-collection marker shape; downstream index ops return E_RANGE.
			if marker == bytecode.IndexMarkerFirst {
				vm.Push(types.NewInt(1))
			} else if marker == bytecode.IndexMarkerLast {
				vm.Push(types.NewInt(0))
			} else {
				return fmt.Errorf("E_INVARG: invalid index marker")
			}
			return nil
		}

		// Keys() is already rbtree traversal order (Toast's first/last).
		if marker == bytecode.IndexMarkerFirst {
			vm.Push(keys[0])
		} else if marker == bytecode.IndexMarkerLast {
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

	switch startVal.Type() {
	case types.TYPE_INT:
		start = startVal.Int()
	case types.TYPE_OBJ, types.TYPE_ANON:
		start = int64(startVal.ID())
	default:
		return fmt.Errorf("E_TYPE: list range requires integer start")
	}

	switch endVal.Type() {
	case types.TYPE_INT:
		end = endVal.Int()
	case types.TYPE_OBJ, types.TYPE_ANON:
		end = int64(endVal.ID())
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
