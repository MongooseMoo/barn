package builtins

import (
	"github.com/MongooseMoo/barn/types"
)

// ============================================================================
// LAYER 7.5: MAP BUILTINS
// ============================================================================

// builtinMapkeys returns a list of all keys in the map in tree order.
// mapkeys(map) -> list
// Toast walks the rbtree (mapforeach) with NO separate sort; Keys() is that
// traversal, so re-sorting here would diverge (CompareMapKeys ranks err/float
// and bool/str opposite Toast's runtime type ordinals).
func builtinMapkeys(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_MAP {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewList(args[0].Keys()))
}

// builtinMapvalues returns a list of all values in the map, sorted by key order
// mapvalues(map) -> list
func builtinMapvalues(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_MAP {
		return types.Err(types.E_TYPE)
	}
	m := args[0]

	// mapvalues(map, key1, key2, ...) -> list of selected values in arg order.
	// Lookup is case-sensitive for string keys in this multi-key form.
	if len(args) > 1 {
		values := make([]types.Value, 0, len(args)-1)
		for i := 1; i < len(args); i++ {
			key := args[i]
			val, found := m.GetWithCase(key, true)
			if !found {
				return types.Err(types.E_RANGE)
			}
			values = append(values, val)
		}
		return types.Ok(types.NewList(values))
	}

	// Values in tree order, matching mapkeys.
	pairs := m.Pairs()
	values := make([]types.Value, len(pairs))
	for i := range pairs {
		values[i] = pairs[i][1]
	}

	return types.Ok(types.NewList(values))
}

// builtinMapdelete returns a new map with the key removed
// mapdelete(map, key) -> map
func builtinMapdelete(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_MAP {
		return types.Err(types.E_TYPE)
	}
	m := args[0]

	keyOrList := args[1]

	// mapdelete(map, {k1, k2, ...}) deletes multiple keys.
	if keyOrList.Type() == types.TYPE_LIST {
		keyList := keyOrList
		result := m
		for _, key := range keyList.Elements() {
			if !types.IsValidBuiltinMapKey(key) {
				return types.Err(types.E_TYPE)
			}
			if _, found := result.MapGet(key); !found {
				exceptionList := types.NewList([]types.Value{
					types.NewErr(types.E_RANGE),
					types.NewStr("Key " + key.String() + " not found in map"),
					key,
				})
				return types.Result{
					Flow:  types.FlowException,
					Error: types.E_RANGE,
					Val:   exceptionList,
				}
			}
			result = result.MapDelete(key)
		}
		if err := CheckMapLimitForTask(ctx.TaskContext, result); err != types.E_NONE {
			return types.Err(err)
		}
		return types.Ok(result)
	}

	key := keyOrList
	if !types.IsValidBuiltinMapKey(key) {
		return types.Err(types.E_TYPE)
	}

	_, found := m.MapGet(key)
	if !found {
		return types.Err(types.E_RANGE)
	}

	result := m.MapDelete(key)

	// Check the map limit, independently of the list value byte limit.
	if err := CheckMapLimitForTask(ctx.TaskContext, result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinMaphaskey tests if a key exists in the map
// maphaskey(map, key) -> int (1 if found, 0 if not)
func builtinMaphaskey(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_MAP {
		return types.Err(types.E_TYPE)
	}
	m := args[0]

	key := args[1]

	// Map keys must be scalar types (not list/map/waif).
	if !types.IsValidBuiltinMapKey(key) {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		caseSensitive = args[2].Int() != 0
	}

	_, found := m.GetWithCase(key, caseSensitive)
	if found {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinMapmerge merges two maps (map2 values override map1 on duplicates)
// mapmerge(map1, map2) -> map
func builtinMapmerge(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_MAP || args[1].Type() != types.TYPE_MAP {
		return types.Err(types.E_TYPE)
	}
	m1 := args[0]
	m2 := args[1]

	// Start with a copy of map1
	result := m1

	// Add all entries from map2 (overriding any duplicates)
	for _, pair := range m2.Pairs() {
		result = result.MapSet(pair[0], pair[1])
	}

	// Check size limit
	if err := CheckMapLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}
