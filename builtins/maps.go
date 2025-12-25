package builtins

import (
	"barn/types"
)

// ============================================================================
// LAYER 7.5: MAP BUILTINS
// ============================================================================

// builtinMapkeys returns a list of all keys in the map
// mapkeys(map) -> list
func builtinMapkeys(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	keys := m.Keys()
	return types.Ok(types.NewList(keys))
}

// builtinMapvalues returns a list of all values in the map
// mapvalues(map) -> list
func builtinMapvalues(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	m, ok := args[0].(types.MapValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Get all pairs and extract values
	pairs := m.Pairs()
	values := make([]types.Value, len(pairs))
	for i, pair := range pairs {
		values[i] = pair[1]
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

	return types.Ok(m.Delete(key))
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

	return types.Ok(result)
}
