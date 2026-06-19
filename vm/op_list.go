package vm

import (
	"barn/builtins"
	"barn/types"
	"fmt"
)

func (vm *VM) executeMakeList() error {
	count := vm.ReadByte()
	elements := vm.PopN(int(count))
	result := types.NewList(elements)
	if errCode := builtins.CheckListLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: list too large")
	}
	vm.Push(result)
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

	result := types.NewMap(pairs)
	if errCode := builtins.CheckMapLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: map too large")
	}
	vm.Push(result)
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

// executeListAppend handles OP_LIST_APPEND: pop elem, pop list; push list with elem appended.
// Used for building lists with splices (non-splice elements).
func (vm *VM) executeListAppend() error {
	elem := vm.Pop()
	listVal := vm.Pop()

	list, ok := listVal.(types.ListValue)
	if !ok {
		return fmt.Errorf("E_TYPE: LIST_APPEND requires a list")
	}

	// Build new list with element appended. Bulk-copy the backing slice rather
	// than re-fetching each element through Get (interface dispatch + bounds
	// check + 1-based indexing) one at a time.
	src := list.Elements()
	newElems := make([]types.Value, len(src)+1)
	copy(newElems, src)
	newElems[len(src)] = elem

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

	// Build new list with all elements of src appended. Bulk-copy both backing
	// slices rather than re-fetching each element through Get one at a time.
	baseElems := list.Elements()
	srcElems := src.Elements()
	newElems := make([]types.Value, len(baseElems)+len(srcElems))
	copy(newElems, baseElems)
	copy(newElems[len(baseElems):], srcElems)

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
