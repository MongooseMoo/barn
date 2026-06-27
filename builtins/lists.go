package builtins

import (
	"fmt"
	"sort"

	"barn/kernel"
	"barn/types"
)

// ============================================================================
// LAYER 7.2: LIST BUILTINS
// ============================================================================

// builtinListappend inserts value after the specified position
// listappend(list, value [, index]) -> list
// Index range: 0 to length(list), default: length(list) (appends)
func builtinListappend(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Default: append to end
	index := list.Len()
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		index = int(args[2].Int())
		if index < 0 || index > list.Len() {
			return types.Err(types.E_RANGE)
		}
	}

	// Insert after index
	result := list.InsertAt(index+1, value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinListinsert inserts value before the specified position
// listinsert(list, value [, index]) -> list
// Index range: 1 to length(list)+1, default: 1 (prepend)
// Out of bounds indices are clamped
func builtinListinsert(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Default: insert at beginning
	index := 1
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		index = int(args[2].Int())
		// Clamp to valid range
		if index <= 0 {
			index = 1
		} else if index > list.Len()+1 {
			index = list.Len() + 1
		}
	}

	// Insert at index (1-based)
	result := list.InsertAt(index, value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinListdelete removes element at index
// listdelete(list, index) -> list
func builtinListdelete(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	index := int(args[1].Int())
	if index < 1 || index > list.Len() {
		return types.Err(types.E_RANGE)
	}

	result := list.DeleteAt(index)

	// Check size limit (even for deletions, to be thorough)
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinListset replaces element at index
// listset(list, value, index) -> list
func builtinListset(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	if args[2].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	index := int(args[2].Int())
	if index < 1 || index > list.Len() {
		return types.Err(types.E_RANGE)
	}

	result := list.Set(index, value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinSetadd adds value if not already present
// setadd(list, value) -> list
func builtinSetadd(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Check if value already exists
	for i := 1; i <= list.Len(); i++ {
		if list.Get(i).Equal(value) {
			return types.Ok(list) // Already present, return unchanged
		}
	}

	// Not present, append
	result := list.Append(value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinSetremove removes first occurrence of value
// setremove(list, value) -> list
func builtinSetremove(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Find first occurrence
	for i := 1; i <= list.Len(); i++ {
		if list.Get(i).Equal(value) {
			result := list.DeleteAt(i)

			// Check size limit
			if err := CheckListLimit(result); err != types.E_NONE {
				return types.Err(err)
			}

			return types.Ok(result)
		}
	}

	// Not found, return unchanged
	return types.Ok(list)
}

// builtinIsMember tests if value is in list
// is_member(value, list) -> int (1-based index or 0)
func builtinIsMember(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	value := args[0]

	switch args[1].Type() {
	case types.TYPE_LIST:
		collection := args[1]
		// Find value in list (case-sensitive for strings)
		for i := 1; i <= collection.Len(); i++ {
			if strictEqual(collection.Get(i), value) {
				return types.Ok(types.NewInt(int64(i)))
			}
		}
		return types.Ok(types.NewInt(0))

	case types.TYPE_MAP:
		collection := args[1]
		// For maps, is_member searches for a VALUE and returns the position
		// of its key in the sorted key list (1-based), or 0 if not found
		// This is case-SENSITIVE for string values (uses strictEqual)
		pairs := collection.Pairs()
		sortMapPairs(pairs)
		for i, pair := range pairs {
			if strictEqual(pair[1], value) {
				return types.Ok(types.NewInt(int64(i + 1)))
			}
		}
		return types.Ok(types.NewInt(0))

	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinSort sorts a list
// sort(list [, keys] [, natural] [, reverse]) -> list
func builtinSort(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

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

// builtinReverse reverses a list or string
// reverse(list) -> list
// reverse(str) -> str
func builtinReverse(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	switch args[0].Type() {
	case types.TYPE_LIST:
		v := args[0]
		// Copy and reverse list elements.
		elements := make([]types.Value, v.Len())
		for i := 1; i <= v.Len(); i++ {
			elements[v.Len()-i] = v.Get(i)
		}
		return types.Ok(types.NewList(elements))
	case types.TYPE_STR:
		runes := []rune(args[0].Str())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return types.Ok(types.NewStr(string(runes)))
	default:
		return types.Err(types.E_INVARG)
	}
}

// builtinUnique removes duplicate elements
// unique(list) -> list
func builtinUnique(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

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
	switch a.Type() {
	case types.TYPE_INT:
		if a.Int() < b.Int() {
			return -1
		} else if a.Int() > b.Int() {
			return 1
		}
		return 0

	case types.TYPE_FLOAT:
		if a.Float() < b.Float() {
			return -1
		} else if a.Float() > b.Float() {
			return 1
		}
		return 0

	case types.TYPE_STR:
		if a.Str() < b.Str() {
			return -1
		} else if a.Str() > b.Str() {
			return 1
		}
		return 0

	case types.TYPE_OBJ, types.TYPE_ANON:
		if a.ID() < b.ID() {
			return -1
		} else if a.ID() > b.ID() {
			return 1
		}
		return 0

	case types.TYPE_ERR:
		if a.Code() < b.Code() {
			return -1
		} else if a.Code() > b.Code() {
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

// builtinSlice: slice(list [, index] [, default_value]) → LIST
// Extracts elements from each item in a list of lists, strings, or maps.
func builtinSlice(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	// First arg must be a list
	if args[0].Type() != types.TYPE_LIST {
		fmt.Printf("[SLICE DEBUG] First arg not a list: %T = %v\n", args[0], args[0])
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	// Default index is 1
	var index types.Value = types.NewInt(1)
	if len(args) >= 2 {
		index = args[1]
	}

	// Optional default value (only for map key lookups)
	var defaultValue types.Value
	hasDefault := len(args) >= 3
	if hasDefault {
		defaultValue = args[2]
	}

	result := make([]types.Value, 0, list.Len())

	switch index.Type() {
	case types.TYPE_INT:
		// Single integer index
		i := int(index.Int())
		if i < 1 {
			return types.Err(types.E_RANGE)
		}

		for j := 1; j <= list.Len(); j++ {
			elem := list.Get(j)
			switch elem.Type() {
			case types.TYPE_LIST:
				if i > elem.Len() {
					return types.Err(types.E_RANGE)
				}
				result = append(result, elem.Get(i))
			case types.TYPE_STR:
				runes := []rune(elem.Str())
				if i > len(runes) {
					return types.Err(types.E_RANGE)
				}
				result = append(result, types.NewStr(string(runes[i-1])))
			default:
				fmt.Printf("[SLICE DEBUG] E_INVARG: element not list/str: %T = %v\n", elem, elem)
				return types.Err(types.E_INVARG)
			}
		}

	case types.TYPE_LIST:
		// List of indices
		if index.Len() == 0 {
			return types.Err(types.E_RANGE)
		}

		// Validate all indices are positive integers
		indices := make([]int, index.Len())
		for k := 1; k <= index.Len(); k++ {
			idxVal := index.Get(k)
			if idxVal.Type() != types.TYPE_INT {
				return types.Err(types.E_INVARG)
			}
			if idxVal.Int() < 1 {
				return types.Err(types.E_RANGE)
			}
			indices[k-1] = int(idxVal.Int())
		}

		for j := 1; j <= list.Len(); j++ {
			elem := list.Get(j)
			subResult := make([]types.Value, 0, len(indices))

			switch elem.Type() {
			case types.TYPE_LIST:
				for _, i := range indices {
					if i > elem.Len() {
						return types.Err(types.E_RANGE)
					}
					subResult = append(subResult, elem.Get(i))
				}
			case types.TYPE_STR:
				runes := []rune(elem.Str())
				for _, i := range indices {
					if i > len(runes) {
						return types.Err(types.E_RANGE)
					}
					subResult = append(subResult, types.NewStr(string(runes[i-1])))
				}
			default:
				return types.Err(types.E_INVARG)
			}

			result = append(result, types.NewList(subResult))
		}

	case types.TYPE_STR:
		// String key for map lookups
		key := index.Str()

		for j := 1; j <= list.Len(); j++ {
			elem := list.Get(j)
			if elem.Type() != types.TYPE_MAP {
				return types.Err(types.E_INVARG)
			}

			val, found := elem.MapGet(types.NewStr(key))
			if found {
				result = append(result, val)
			} else if hasDefault {
				result = append(result, defaultValue)
			}
			// If not found and no default, skip (don't append anything)
		}

	default:
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewList(result))
}
