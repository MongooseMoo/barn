package vm

import (
	"fmt"

	"github.com/MongooseMoo/barn/types"
)

func (vm *VM) executeMakeList() error {
	count := vm.FetchByte()
	elements := vm.PopN(int(count))
	result := types.NewList(elements)
	if errCode := vm.Builtins.CheckListLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: list too large")
	}
	vm.Push(result)
	return nil
}

func (vm *VM) executeMakeMap() error {
	count := vm.FetchByte()
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
	if errCode := vm.Builtins.CheckMapLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: map too large")
	}
	vm.Push(result)
	return nil
}

func (vm *VM) executeCheckMapLimit() error {
	if errCode := vm.Builtins.CheckMapLimitForTask(vm.Context, vm.Peek(0)); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: map too large")
	}
	return nil
}

func (vm *VM) executeLength() error {
	coll := vm.Pop()

	switch coll.Type() {
	case types.TYPE_LIST:
		vm.Push(types.NewInt(int64(coll.Len())))
	case types.TYPE_STR:
		vm.Push(types.NewInt(int64(coll.Len())))
	case types.TYPE_MAP:
		vm.Push(types.NewInt(int64(coll.Len())))
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

	if listVal.Type() != types.TYPE_LIST {
		return fmt.Errorf("E_TYPE: LIST_APPEND requires a list")
	}

	// Append (COW). list.Append maintains the cached byte-size incrementally so
	// the quota check below stays O(1) instead of re-walking the whole list.
	result := listVal.Append(elem)
	if errCode := vm.Builtins.CheckListLimitForTask(vm.Context, result); errCode != types.E_NONE {
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

	if listVal.Type() != types.TYPE_LIST {
		return fmt.Errorf("E_TYPE: LIST_EXTEND requires a list base")
	}

	if srcVal.Type() != types.TYPE_LIST {
		return fmt.Errorf("E_TYPE: splice requires a list operand")
	}

	// Concat (COW). list.Concat maintains the cached byte-size incrementally so
	// the quota check below stays O(1) instead of re-walking the whole list.
	result := listVal.Concat(srcVal)
	if errCode := vm.Builtins.CheckListLimit(result); errCode != types.E_NONE {
		return fmt.Errorf("E_QUOTA: list too large")
	}

	vm.Push(result)
	return nil
}

func (vm *VM) executeSplice() error {
	val := vm.Pop()

	// Standalone @expr: operand must be a list, otherwise E_TYPE.
	if val.Type() != types.TYPE_LIST {
		return fmt.Errorf("E_TYPE: splice (@) requires a list operand")
	}

	vm.Push(val)
	return nil
}
