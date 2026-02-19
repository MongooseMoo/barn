package builtins

import (
	"barn/types"
	"sort"
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
		return types.CompareMapKeys(keys[i], keys[j]) < 0
	})
}

// sortMapPairs sorts map pairs by their keys in MOO canonical order
func sortMapPairs(pairs [][2]types.Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return types.CompareMapKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// builtinMapvalues returns a list of all values in the map, sorted by key order
// mapvalues(map) -> list
func builtinMapvalues(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// mapvalues(map, key1, key2, ...) -> list of selected values in arg order.
	// Lookup is case-sensitive for string keys in this multi-key form.
	if len(args) > 1 {
		values := make([]types.Value, 0, len(args)-1)
		for i := 1; i < len(args); i++ {
			key := args[i]
			if !types.IsValidBuiltinMapKey(key) {
				return types.Err(types.E_TYPE)
			}
			val, found := m.GetWithCase(key, true)
			if !found {
				return types.Err(types.E_RANGE)
			}
			values = append(values, val)
		}
		return types.Ok(types.NewList(values))
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

	keyOrList := args[1]

	// mapdelete(map, {k1, k2, ...}) deletes multiple keys.
	if keyList, ok := keyOrList.(types.ListValue); ok {
		result := m
		for _, key := range keyList.Elements() {
			if !types.IsValidBuiltinMapKey(key) {
				return types.Err(types.E_TYPE)
			}
			if _, found := result.Get(key); !found {
				return types.Err(types.E_RANGE)
			}
			result = result.Delete(key)
		}
		if err := CheckMapLimit(result); err != types.E_NONE {
			return types.Err(err)
		}
		return types.Ok(result)
	}

	key := keyOrList
	if !types.IsValidBuiltinMapKey(key) {
		return types.Err(types.E_TYPE)
	}

	_, found := m.Get(key)
	if !found {
		return types.Err(types.E_RANGE)
	}

	result := m.Delete(key)

	// Check size limit
	if err := CheckMapLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinMaphaskey tests if a key exists in the map
// maphaskey(map, key) -> int (1 if found, 0 if not)
func builtinMaphaskey(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	key := args[1]

	// Map keys must be scalar types (not list/map/waif).
	if !types.IsValidBuiltinMapKey(key) {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) == 3 {
		caseVal, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		caseSensitive = caseVal.Val != 0
	}

	_, found := m.GetWithCase(key, caseSensitive)
	if found {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
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
