package builtins

import (
	"barn/types"
	"sort"
)

// ============================================================================
// LAYER 7.2: LIST BUILTINS
// ============================================================================

// builtinListappend inserts value after the specified position
// listappend(list, value [, index]) -> list
// Index range: 0 to length(list), default: length(list) (appends)
func builtinListappend(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := args[1]

	// Default: append to end
	index := list.Len()
	if len(args) == 3 {
		idx, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		index = int(idx.Val)
		if index < 0 || index > list.Len() {
			return types.Err(types.E_RANGE)
		}
	}

	// Insert after index
	return types.Ok(list.InsertAt(index+1, value))
}

// builtinListinsert inserts value before the specified position
// listinsert(list, value [, index]) -> list
// Index range: 1 to length(list)+1, default: 1 (prepend)
// Out of bounds indices are clamped
func builtinListinsert(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := args[1]

	// Default: insert at beginning
	index := 1
	if len(args) == 3 {
		idx, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		index = int(idx.Val)
		// Clamp to valid range
		if index <= 0 {
			index = 1
		} else if index > list.Len()+1 {
			index = list.Len() + 1
		}
	}

	// Insert at index (1-based)
	return types.Ok(list.InsertAt(index, value))
}

// builtinListdelete removes element at index
// listdelete(list, index) -> list
func builtinListdelete(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	idx, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	index := int(idx.Val)
	if index < 1 || index > list.Len() {
		return types.Err(types.E_RANGE)
	}

	return types.Ok(list.DeleteAt(index))
}

// builtinListset replaces element at index
// listset(list, value, index) -> list
func builtinListset(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := args[1]

	idx, ok := args[2].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	index := int(idx.Val)
	if index < 1 || index > list.Len() {
		return types.Err(types.E_RANGE)
	}

	return types.Ok(list.Set(index, value))
}

// builtinSetadd adds value if not already present
// setadd(list, value) -> list
func builtinSetadd(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := args[1]

	// Check if value already exists
	for i := 1; i <= list.Len(); i++ {
		if list.Get(i).Equal(value) {
			return types.Ok(list) // Already present, return unchanged
		}
	}

	// Not present, append
	return types.Ok(list.Append(value))
}

// builtinSetremove removes first occurrence of value
// setremove(list, value) -> list
func builtinSetremove(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := args[1]

	// Find first occurrence
	for i := 1; i <= list.Len(); i++ {
		if list.Get(i).Equal(value) {
			return types.Ok(list.DeleteAt(i))
		}
	}

	// Not found, return unchanged
	return types.Ok(list)
}

// builtinIsMember tests if value is in list
// is_member(value, list) -> int (1-based index or 0)
func builtinIsMember(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	value := args[0]

	switch collection := args[1].(type) {
	case types.ListValue:
		// Find value in list
		for i := 1; i <= collection.Len(); i++ {
			if collection.Get(i).Equal(value) {
				return types.Ok(types.IntValue{Val: int64(i)})
			}
		}
		return types.Ok(types.IntValue{Val: 0})

	case types.MapValue:
		// For maps, check if key exists and return 1 if found
		// Note: This is a simplified implementation - full MOO semantics
		// would search values and return position, but that requires
		// ordered maps which we don't have yet
		_, ok := collection.Get(value)
		if ok {
			return types.Ok(types.IntValue{Val: 1})
		}
		return types.Ok(types.IntValue{Val: 0})

	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinSort sorts a list
// sort(list [, keys] [, natural] [, reverse]) -> list
func builtinSort(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// For now, implement simple sort (ignoring keys, natural, reverse)
	// TODO: Implement full sort with all parameters

	// Copy list elements
	elements := make([]types.Value, list.Len())
	for i := 1; i <= list.Len(); i++ {
		elements[i-1] = list.Get(i)
	}

	// Sort using Go's sort package
	sort.Slice(elements, func(i, j int) bool {
		return compareValues(elements[i], elements[j]) < 0
	})

	return types.Ok(types.NewList(elements))
}

// builtinReverse reverses a list
// reverse(list) -> list
func builtinReverse(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Copy and reverse
	elements := make([]types.Value, list.Len())
	for i := 1; i <= list.Len(); i++ {
		elements[list.Len()-i] = list.Get(i)
	}

	return types.Ok(types.NewList(elements))
}

// builtinUnique removes duplicate elements
// unique(list) -> list
func builtinUnique(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Use map to track seen values
	seen := make(map[string]bool)
	var unique []types.Value

	for i := 1; i <= list.Len(); i++ {
		elem := list.Get(i)
		key := elem.String() // Use string representation as key
		if !seen[key] {
			seen[key] = true
			unique = append(unique, elem)
		}
	}

	return types.Ok(types.NewList(unique))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// compareValues compares two MOO values for sorting
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func compareValues(a, b types.Value) int {
	// Type codes for ordering
	aType := a.Type()
	bType := b.Type()

	if aType != bType {
		// Different types: order by type code
		if aType < bType {
			return -1
		}
		return 1
	}

	// Same type: compare values
	switch av := a.(type) {
	case types.IntValue:
		bv := b.(types.IntValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0

	case types.FloatValue:
		bv := b.(types.FloatValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0

	case types.StrValue:
		bv := b.(types.StrValue)
		if av.Value() < bv.Value() {
			return -1
		} else if av.Value() > bv.Value() {
			return 1
		}
		return 0

	case types.ObjValue:
		bv := b.(types.ObjValue)
		if av.ID() < bv.ID() {
			return -1
		} else if av.ID() > bv.ID() {
			return 1
		}
		return 0

	case types.ErrValue:
		bv := b.(types.ErrValue)
		if av.Code() < bv.Code() {
			return -1
		} else if av.Code() > bv.Code() {
			return 1
		}
		return 0

	default:
		// Lists, maps, etc.: compare by string representation
		as := a.String()
		bs := b.String()
		if as < bs {
			return -1
		} else if as > bs {
			return 1
		}
		return 0
	}
}
