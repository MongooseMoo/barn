package builtins

import (
	"barn/db"
	"barn/types"
	"sync"
)

// ============================================================================
// STRING AND VALUE LIMIT CHECKING
// ============================================================================

// Global cache for server options (matches ToastStunt's _server_int_option_cache)
// This is updated by load_server_options() and read by limit-checking functions
var (
	serverOptionsCache = struct {
		sync.RWMutex
		maxStringConcat int // -1 means not set, use default
		maxListValueBytes int
		maxMapValueBytes int
	}{
		maxStringConcat: -1, // Not set initially
		maxListValueBytes: -1,
		maxMapValueBytes: -1,
	}
)

// GetMaxStringConcat returns the cached max_string_concat limit.
// Returns -1 if not set (use default from TaskContext).
func GetMaxStringConcat() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	return serverOptionsCache.maxStringConcat
}

// findPropertyInherited finds a property anywhere in the inheritance chain
// Returns the property or nil if not found
func findPropertyInherited(objID types.ObjID, name string, store *db.Store) *db.Property {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := store.Get(currentID)
		if current == nil {
			continue
		}

		// Check if property exists on this object
		if prop, ok := current.Properties[name]; ok {
			return prop
		}

		// Add parents to queue
		queue = append(queue, current.Parents...)
	}

	return nil
}

// LoadServerOptionsFromStore reads limits from $server_options object and caches them.
// This is called by the load_server_options() builtin.
// Returns the number of options successfully loaded.
func LoadServerOptionsFromStore(store *db.Store) int {
	if store == nil {
		return 0
	}

	// Look up the server_options property on #0 (searching inheritance chain)
	serverOptsProp := findPropertyInherited(0, "server_options", store)
	if serverOptsProp == nil {
		return 0 // No server_options property
	}

	// The property value should be an object reference
	serverOptsRef, ok := serverOptsProp.Value.(types.ObjValue)
	if !ok {
		return 0 // server_options is not an object
	}

	// Get the actual server_options object ID
	serverOptsID := serverOptsRef.ID()

	loaded := 0

	// Read max_string_concat (searching inheritance chain)
	if prop := findPropertyInherited(serverOptsID, "max_string_concat", store); prop != nil {
		if intVal, ok := prop.Value.(types.IntValue); ok {
			serverOptionsCache.Lock()
			serverOptionsCache.maxStringConcat = int(intVal.Val)
			serverOptionsCache.Unlock()
			loaded++
		}
	}

	// Read max_list_value_bytes
	if prop := findPropertyInherited(serverOptsID, "max_list_value_bytes", store); prop != nil {
		if intVal, ok := prop.Value.(types.IntValue); ok {
			serverOptionsCache.Lock()
			serverOptionsCache.maxListValueBytes = int(intVal.Val)
			serverOptionsCache.Unlock()
			loaded++
		}
	}

	// Read max_map_value_bytes
	if prop := findPropertyInherited(serverOptsID, "max_map_value_bytes", store); prop != nil {
		if intVal, ok := prop.Value.(types.IntValue); ok {
			serverOptionsCache.Lock()
			serverOptionsCache.maxMapValueBytes = int(intVal.Val)
			serverOptionsCache.Unlock()
			loaded++
		}
	}

	return loaded
}

// UpdateContextLimits updates a TaskContext with current cached limits from load_server_options().
// This should be called by string-producing builtins before creating output.
// If no cached limit is set, the context's default limit is used.
func UpdateContextLimits(ctx *types.TaskContext) {
	cachedLimit := GetMaxStringConcat()
	if cachedLimit > 0 {
		ctx.MaxStringConcat = cachedLimit
	}
}

// ============================================================================
// VALUE_BYTES() BUILTIN AND HELPERS
// ============================================================================

// builtinValueBytes implements the value_bytes(value) builtin.
// Returns the size in bytes of any MOO value.
func builtinValueBytes(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	size := ValueBytes(args[0])
	return types.Ok(types.NewInt(int64(size)))
}

// ValueBytes calculates the byte size of a MOO value.
// This matches the algorithm from spec/builtins/limits.md.
func ValueBytes(v types.Value) int {
	base := 8 // sizeof pointer/interface
	switch val := v.(type) {
	case types.IntValue:
		return base + 8
	case types.FloatValue:
		return base + 8
	case types.StrValue:
		return base + len(val.Value()) + 1
	case types.ObjValue:
		return base + 8
	case types.ErrValue:
		return base + 4
	case types.ListValue:
		size := base + 8 // list header
		for i := 1; i <= val.Len(); i++ {
			size += ValueBytes(val.Get(i))
		}
		return size
	case types.MapValue:
		size := base + 8 // map header
		for _, pair := range val.Pairs() {
			size += ValueBytes(pair[0]) + ValueBytes(pair[1])
		}
		return size
	case types.WaifValue:
		// Waif size = base + class ref + property values
		size := base + 16
		// Add property values if accessible
		return size
	default:
		return base
	}
}

// GetMaxListValueBytes returns the cached max_list_value_bytes limit.
// Returns 0 if not set (unlimited).
func GetMaxListValueBytes() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	if serverOptionsCache.maxListValueBytes > 0 {
		return serverOptionsCache.maxListValueBytes
	}
	return 0 // 0 means unlimited
}

// GetMaxMapValueBytes returns the cached max_map_value_bytes limit.
// Returns 0 if not set (unlimited).
func GetMaxMapValueBytes() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	if serverOptionsCache.maxMapValueBytes > 0 {
		return serverOptionsCache.maxMapValueBytes
	}
	return 0
}

// CheckListLimit checks if a list exceeds the max_list_value_bytes limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func CheckListLimit(list types.ListValue) types.ErrorCode {
	limit := GetMaxListValueBytes()
	if limit > 0 && ValueBytes(list) > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckMapLimit checks if a map exceeds the max_map_value_bytes limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func CheckMapLimit(m types.MapValue) types.ErrorCode {
	limit := GetMaxMapValueBytes()
	if limit > 0 && ValueBytes(m) > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckStringLimit checks if a string exceeds the max_string_concat limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func CheckStringLimit(s string) types.ErrorCode {
	limit := GetMaxStringConcat()
	if limit > 0 && len(s) > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}
