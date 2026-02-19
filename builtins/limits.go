package builtins

import (
	"barn/db"
	"barn/types"
	"math"
	"sync"
)

// ============================================================================
// STRING AND VALUE LIMIT CHECKING
// ============================================================================

// Global cache for server options (matches ToastStunt's _server_int_option_cache)
// This is updated by load_server_options() and read by limit-checking functions
const (
	defaultMaxStringConcat   = 64537861
	defaultMaxListValueBytes = 64537861
	defaultMaxMapValueBytes  = 64537861
	minStringConcatLimit     = 1021
	minListValueBytesLimit   = 1021
	minMapValueBytesLimit    = 1021
	maxStringConcatLimit     = math.MaxInt32 - minStringConcatLimit
	maxListValueBytesLimit   = math.MaxInt32 - minListValueBytesLimit
	maxMapValueBytesLimit    = math.MaxInt32 - minMapValueBytesLimit
)

var (
	serverOptionsCache = struct {
		sync.RWMutex
		maxStringConcat   int
		maxListValueBytes int
		maxMapValueBytes  int
	}{
		maxStringConcat:   defaultMaxStringConcat,
		maxListValueBytes: defaultMaxListValueBytes,
		maxMapValueBytes:  defaultMaxMapValueBytes,
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
	// Reset to defaults on every load, matching Toast's cache refresh behavior.
	nextString := defaultMaxStringConcat
	nextList := defaultMaxListValueBytes
	nextMap := defaultMaxMapValueBytes
	loaded := 0

	if store == nil {
		serverOptionsCache.Lock()
		serverOptionsCache.maxStringConcat = nextString
		serverOptionsCache.maxListValueBytes = nextList
		serverOptionsCache.maxMapValueBytes = nextMap
		serverOptionsCache.Unlock()
		return 0
	}

	// Look up the server_options property on #0 (searching inheritance chain)
	serverOptsProp := findPropertyInherited(0, "server_options", store)
	if serverOptsProp == nil {
		serverOptionsCache.Lock()
		serverOptionsCache.maxStringConcat = nextString
		serverOptionsCache.maxListValueBytes = nextList
		serverOptionsCache.maxMapValueBytes = nextMap
		serverOptionsCache.Unlock()
		return 0 // No server_options property
	}

	// The property value should be an object reference
	serverOptsRef, ok := serverOptsProp.Value.(types.ObjValue)
	if !ok {
		serverOptionsCache.Lock()
		serverOptionsCache.maxStringConcat = nextString
		serverOptionsCache.maxListValueBytes = nextList
		serverOptionsCache.maxMapValueBytes = nextMap
		serverOptionsCache.Unlock()
		return 0 // server_options is not an object
	}

	// Get the actual server_options object ID
	serverOptsID := serverOptsRef.ID()

	// Read max_string_concat (searching inheritance chain)
	if prop := findPropertyInherited(serverOptsID, "max_string_concat", store); prop != nil {
		if intVal, ok := prop.Value.(types.IntValue); ok {
			nextString = canonicalizeLimit(int(intVal.Val), minStringConcatLimit, maxStringConcatLimit)
			loaded++
		}
	}

	// Read max_list_value_bytes
	if prop := findPropertyInherited(serverOptsID, "max_list_value_bytes", store); prop != nil {
		if intVal, ok := prop.Value.(types.IntValue); ok {
			nextList = canonicalizeLimit(int(intVal.Val), minListValueBytesLimit, maxListValueBytesLimit)
			loaded++
		}
	}

	// Read max_map_value_bytes
	if prop := findPropertyInherited(serverOptsID, "max_map_value_bytes", store); prop != nil {
		if intVal, ok := prop.Value.(types.IntValue); ok {
			nextMap = canonicalizeLimit(int(intVal.Val), minMapValueBytesLimit, maxMapValueBytesLimit)
			loaded++
		}
	}

	serverOptionsCache.Lock()
	serverOptionsCache.maxStringConcat = nextString
	serverOptionsCache.maxListValueBytes = nextList
	serverOptionsCache.maxMapValueBytes = nextMap
	serverOptionsCache.Unlock()

	return loaded
}

func canonicalizeLimit(value, min, max int) int {
	if value > 0 && value < min {
		return min
	}
	if value <= 0 || value > max {
		return max
	}
	return value
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
// This matches Toast's value_bytes() algorithm from src/utils.cc.
// Toast uses sizeof(Var) = 16 bytes as the base size for all values.
func ValueBytes(v types.Value) int {
	const varSize = 16 // sizeof(Var) in Toast - base size for any value

	switch val := v.(type) {
	case types.IntValue:
		// Integer fits in Var structure
		return varSize
	case types.FloatValue:
		// Var + separate double storage
		return varSize + 8
	case types.StrValue:
		// Var + string data + null terminator
		return varSize + len(val.Value()) + 1
	case types.ObjValue:
		// Object ID fits in Var structure
		return varSize
	case types.ErrValue:
		// Error code fits in Var structure
		return varSize
	case types.ListValue:
		// List contains: Var for the list itself + Var for length + elements
		// Toast: sizeof(Var) + list_sizeof() where list_sizeof = sizeof(Var) + elements
		size := varSize + varSize // list Var + length Var
		for i := 1; i <= val.Len(); i++ {
			size += ValueBytes(val.Get(i))
		}
		return size
	case types.MapValue:
		// Similar to list: Var for map + overhead + entries
		size := varSize + varSize // map Var + overhead
		for _, pair := range val.Pairs() {
			size += ValueBytes(pair[0]) + ValueBytes(pair[1])
		}
		return size
	case types.WaifValue:
		// Waif: Var + class reference
		size := varSize + varSize // waif Var + class ref
		// Note: actual waif properties not included here (matches Toast behavior)
		return size
	default:
		return varSize
	}
}

// GetMaxListValueBytes returns the cached max_list_value_bytes limit.
// Returns the currently cached effective limit.
func GetMaxListValueBytes() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	return serverOptionsCache.maxListValueBytes
}

// GetMaxMapValueBytes returns the cached max_map_value_bytes limit.
// Returns the currently cached effective limit.
func GetMaxMapValueBytes() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	return serverOptionsCache.maxMapValueBytes
}

// CheckListLimit checks if a list exceeds the max_list_value_bytes limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
// The limit is exclusive - a list with exactly limit bytes is not allowed.
func CheckListLimit(list types.ListValue) types.ErrorCode {
	limit := GetMaxListValueBytes()
	if limit > 0 && ValueBytes(list) >= limit {
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
