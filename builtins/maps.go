package builtins

import (
	"barn/types"
	"sort"
	"strings"
)

// ============================================================================
// LAYER 7.5: MAP BUILTINS
// ============================================================================

// builtinMapkeys returns a list of all keys in the map, sorted
// mapkeys(map) -> list
// Sorting order: integers (by value), floats (by value), objects (by ID),
// errors (by code), strings (case-insensitive alphabetical)
func builtinMapkeys(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	keys := m.Keys()
	sortMapKeys(keys)
	return types.Ok(types.NewList(keys))
}

// sortMapKeys sorts map keys in MOO canonical order:
// integers < floats < objects < errors < strings
// Within each type, sorted by value
func sortMapKeys(keys []types.Value) {
	sort.Slice(keys, func(i, j int) bool {
		return compareMapKeys(keys[i], keys[j]) < 0
	})
}

// sortMapPairs sorts map pairs by their keys in MOO canonical order
func sortMapPairs(pairs [][2]types.Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return compareMapKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// compareMapKeys returns negative if a < b, 0 if equal, positive if a > b
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4)
// This matches MOO/ToastStunt map key ordering
func compareMapKeys(a, b types.Value) int {
	typeOrder := func(v types.Value) int {
		switch v.Type() {
		case types.TYPE_INT:
			return 0
		case types.TYPE_OBJ:
			return 1
		case types.TYPE_FLOAT:
			return 2
		case types.TYPE_ERR:
			return 3
		case types.TYPE_STR:
			return 4
		default:
			return 5
		}
	}

	aOrder := typeOrder(a)
	bOrder := typeOrder(b)
	if aOrder != bOrder {
		return aOrder - bOrder
	}

	// Same type, compare values
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
	case types.StrValue:
		bv := b.(types.StrValue)
		// Case-insensitive comparison for strings
		return strings.Compare(strings.ToLower(av.Value()), strings.ToLower(bv.Value()))
	}
	return 0
}

// builtinMapvalues returns a list of all values in the map, sorted by key order
// mapvalues(map) -> list
func builtinMapvalues(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Get sorted keys first
	keys := m.Keys()
	sortMapKeys(keys)

	// Extract values in sorted key order
	values := make([]types.Value, len(keys))
	for i, key := range keys {
		val, _ := m.Get(key)
		values[i] = val
	}

	return types.Ok(types.NewList(values))
}

// builtinMapdelete returns a new map with the key removed
// mapdelete(map, key) -> map
func builtinMapdelete(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	key := args[1]

	// Map keys must be scalar types (not list or map)
	if !isValidMapKey(key) {
		return types.Err(types.E_TYPE)
	}

	// Check if key exists - E_RANGE if not found
	if _, found := m.Get(key); !found {
		return types.Err(types.E_RANGE)
	}

	result := m.Delete(key)

	// Check size limit
	if err := CheckMapLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// isValidMapKey checks if a value can be used as a map key
// Only scalar types (int, obj, str, err, float, bool) are valid keys
func isValidMapKey(v types.Value) bool {
	switch v.Type() {
	case types.TYPE_INT, types.TYPE_OBJ, types.TYPE_STR, types.TYPE_ERR, types.TYPE_FLOAT, types.TYPE_BOOL:
		return true
	default:
		return false
	}
}

// builtinMaphaskey tests if a key exists in the map
// maphaskey(map, key) -> bool
func builtinMaphaskey(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	key := args[1]

	// Map keys must be scalar types (not list or map)
	if !isValidMapKey(key) {
		return types.Err(types.E_TYPE)
	}

	_, found := m.Get(key)
	return types.Ok(types.BoolValue{Val: found})
}

// builtinMapmerge merges two maps (map2 values override map1 on duplicates)
// mapmerge(map1, map2) -> map
func builtinMapmerge(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	m1, ok1 := args[0].(types.MapValue)
	m2, ok2 := args[1].(types.MapValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}

	// Start with a copy of map1
	result := m1

	// Add all entries from map2 (overriding any duplicates)
	for _, pair := range m2.Pairs() {
		result = result.Set(pair[0], pair[1])
	}

	// Check size limit
	if err := CheckMapLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}
