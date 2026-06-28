package builtins

import (
	"math"
	"sync"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
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
	defaultFgTicks           = 60000
	defaultBgTicks           = 30000
	defaultFgSeconds         = 5.0
	defaultBgSeconds         = 3.0
	defaultMaxStackDepth     = 50
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
		fgTicks           int64
		bgTicks           int64
		fgSeconds         float64
		bgSeconds         float64
		maxStackDepth     int
	}{
		maxStringConcat:   defaultMaxStringConcat,
		maxListValueBytes: defaultMaxListValueBytes,
		maxMapValueBytes:  defaultMaxMapValueBytes,
		fgTicks:           defaultFgTicks,
		bgTicks:           defaultBgTicks,
		fgSeconds:         defaultFgSeconds,
		bgSeconds:         defaultBgSeconds,
		maxStackDepth:     defaultMaxStackDepth,
	}
)

// GetMaxStringConcat returns the cached max_string_concat limit.
// Returns -1 if not set (use default from TaskContext).
func GetMaxStringConcat() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	return serverOptionsCache.maxStringConcat
}

func GetTaskLimits(background bool) (int64, float64) {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	if background {
		return serverOptionsCache.bgTicks, serverOptionsCache.bgSeconds
	}
	return serverOptionsCache.fgTicks, serverOptionsCache.fgSeconds
}

func GetMaxStackDepth() int {
	serverOptionsCache.RLock()
	defer serverOptionsCache.RUnlock()
	return serverOptionsCache.maxStackDepth
}

func findDefinedProperty(objID types.ObjID, name string, store *dbstore.Store) (dbstore.PropertyView, bool) {
	prop, ok, err := store.DefinedProperty(objID, name)
	if err != types.E_NONE || !ok {
		return dbstore.PropertyView{}, false
	}
	return prop, true
}

type propertyReader func(types.ObjID, string) (dbstore.PropertyView, bool)

func cacheServerOptionsDefaults() {
	snapshot := defaultServerOptionsSnapshot()
	applyServerOptionsSnapshot(&snapshot)
}

func defaultServerOptionsSnapshot() kernel.PendingServerOptions {
	return kernel.PendingServerOptions{
		MaxStringConcat:   defaultMaxStringConcat,
		MaxListValueBytes: defaultMaxListValueBytes,
		MaxMapValueBytes:  defaultMaxMapValueBytes,
		FgTicks:           defaultFgTicks,
		BgTicks:           defaultBgTicks,
		FgSeconds:         defaultFgSeconds,
		BgSeconds:         defaultBgSeconds,
		MaxStackDepth:     defaultMaxStackDepth,
	}
}

func applyServerOptionsSnapshot(snapshot *kernel.PendingServerOptions) {
	if snapshot == nil {
		cacheServerOptionsDefaults()
		return
	}
	serverOptionsCache.Lock()
	serverOptionsCache.maxStringConcat = snapshot.MaxStringConcat
	serverOptionsCache.maxListValueBytes = snapshot.MaxListValueBytes
	serverOptionsCache.maxMapValueBytes = snapshot.MaxMapValueBytes
	serverOptionsCache.fgTicks = snapshot.FgTicks
	serverOptionsCache.bgTicks = snapshot.BgTicks
	serverOptionsCache.fgSeconds = snapshot.FgSeconds
	serverOptionsCache.bgSeconds = snapshot.BgSeconds
	serverOptionsCache.maxStackDepth = snapshot.MaxStackDepth
	serverOptionsCache.Unlock()
}

func collectServerOptions(findProperty propertyReader, findDefined propertyReader) kernel.PendingServerOptions {
	// Reset to defaults on every load, matching Toast's cache refresh behavior.
	snapshot := defaultServerOptionsSnapshot()

	if findProperty == nil || findDefined == nil {
		return snapshot
	}

	// Look up the server_options property on #0 (searching inheritance chain)
	serverOptsProp, ok := findProperty(0, "server_options")
	if !ok {
		return snapshot // No server_options property
	}

	// The property value should be an object reference
	serverOptsRef := serverOptsProp.Value
	if serverOptsRef.Type() != types.TYPE_OBJ {
		return snapshot // server_options is not an object
	}

	// Get the actual server_options object ID
	serverOptsID := serverOptsRef.Obj()

	// Read max_string_concat (searching inheritance chain)
	if prop, ok := findDefined(serverOptsID, "max_string_concat"); ok {
		if prop.Value.Type() == types.TYPE_INT {
			snapshot.MaxStringConcat = canonicalizeLimit(int(prop.Value.Int()), minStringConcatLimit, maxStringConcatLimit)
			snapshot.Loaded++
		}
	}

	// Read max_list_value_bytes
	if prop, ok := findDefined(serverOptsID, "max_list_value_bytes"); ok {
		if prop.Value.Type() == types.TYPE_INT {
			snapshot.MaxListValueBytes = canonicalizeLimit(int(prop.Value.Int()), minListValueBytesLimit, maxListValueBytesLimit)
			snapshot.Loaded++
		}
	}

	// Read max_map_value_bytes
	if prop, ok := findDefined(serverOptsID, "max_map_value_bytes"); ok {
		if prop.Value.Type() == types.TYPE_INT {
			snapshot.MaxMapValueBytes = canonicalizeLimit(int(prop.Value.Int()), minMapValueBytesLimit, maxMapValueBytesLimit)
			snapshot.Loaded++
		}
	}

	if prop, ok := findDefined(serverOptsID, "fg_ticks"); ok {
		if prop.Value.Type() == types.TYPE_INT && prop.Value.Int() > 0 {
			snapshot.FgTicks = prop.Value.Int()
			snapshot.Loaded++
		}
	}
	if prop, ok := findDefined(serverOptsID, "bg_ticks"); ok {
		if prop.Value.Type() == types.TYPE_INT && prop.Value.Int() > 0 {
			snapshot.BgTicks = prop.Value.Int()
			snapshot.Loaded++
		}
	}
	if prop, ok := findDefined(serverOptsID, "fg_seconds"); ok {
		if seconds, ok := numericSeconds(prop.Value); ok && seconds > 0 {
			snapshot.FgSeconds = seconds
			snapshot.Loaded++
		}
	}
	if prop, ok := findDefined(serverOptsID, "bg_seconds"); ok {
		if seconds, ok := numericSeconds(prop.Value); ok && seconds > 0 {
			snapshot.BgSeconds = seconds
			snapshot.Loaded++
		}
	}
	if prop, ok := findDefined(serverOptsID, "max_stack_depth"); ok {
		if prop.Value.Type() == types.TYPE_INT && prop.Value.Int() > 0 {
			snapshot.MaxStackDepth = int(prop.Value.Int())
			snapshot.Loaded++
		}
	}

	return snapshot
}

func loadServerOptions(findProperty propertyReader, findDefined propertyReader) int {
	snapshot := collectServerOptions(findProperty, findDefined)
	applyServerOptionsSnapshot(&snapshot)
	return snapshot.Loaded
}

// LoadServerOptionsFromStore reads limits from $server_options object and caches them.
// This is called at server startup and when no task transaction is active.
// Returns the number of options successfully loaded.
func LoadServerOptionsFromStore(store *dbstore.Store) int {
	if store == nil {
		return loadServerOptions(nil, nil)
	}
	return loadServerOptions(
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, err := store.FindProperty(objID, name)
			if err != types.E_NONE {
				return dbstore.PropertyView{}, false
			}
			return prop, true
		},
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			return findDefinedProperty(objID, name, store)
		},
	)
}

// LoadServerOptionsForTask reads limits through the active task view so
// same-task updates to $server_options take effect before the task commits.
func LoadServerOptionsForTask(ctx *kernel.TaskContext) int {
	if ctx == nil {
		return loadServerOptions(nil, nil)
	}
	snapshot := collectServerOptions(
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, err := findPropertyForRead(ctx, objID, name)
			if err != types.E_NONE {
				return dbstore.PropertyView{}, false
			}
			return prop, true
		},
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, ok, err := localPropertyForRead(ctx, objID, name)
			if err != types.E_NONE || !ok || !prop.Defined {
				return dbstore.PropertyView{}, false
			}
			return prop, true
		},
	)
	if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
		ctx.PendingServerOptions = &snapshot
		return snapshot.Loaded
	}
	applyServerOptionsSnapshot(&snapshot)
	return snapshot.Loaded
}

func FlushPendingServerOptions(ctx *kernel.TaskContext) types.ErrorCode {
	if ctx == nil || ctx.PendingServerOptions == nil {
		return types.E_NONE
	}
	pending := ctx.PendingServerOptions
	applyServerOptionsSnapshot(pending)
	if pending.ProtectedBuiltins != nil {
		applyProtectedBuiltins(pending.ProtectedBuiltins)
	}
	ctx.PendingServerOptions = nil
	return types.E_NONE
}

func DiscardPendingServerOptions(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.PendingServerOptions = nil
}

func numericSeconds(value types.Value) (float64, bool) {
	switch value.Type() {
	case types.TYPE_INT:
		return float64(value.Int()), true
	case types.TYPE_FLOAT:
		return value.Float(), true
	default:
		return 0, false
	}
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
func UpdateContextLimits(ctx *kernel.TaskContext) {
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
func builtinValueBytes(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	size := ValueBytes(args[0])
	return types.Ok(types.NewInt(int64(size)))
}

// ValueBytes calculates the byte size of a MOO value.
// This matches Toast's value_bytes() algorithm from src/utils.cc.
// Toast uses sizeof(Var) = 16 bytes as the base size for all values.
// The canonical implementation lives in the types package so list values can
// cache their own size incrementally (O(1) append accounting); this wrapper
// preserves the existing builtins.ValueBytes call sites.
func ValueBytes(v types.Value) int {
	return types.ValueBytes(v)
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
func CheckListLimit(list types.Value) types.ErrorCode {
	limit := GetMaxListValueBytes()
	if limit > 0 && ValueBytes(list) >= limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckMapLimit checks if a map exceeds the max_map_value_bytes limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func CheckMapLimit(m types.Value) types.ErrorCode {
	limit := GetMaxMapValueBytes()
	if limit > 0 && ValueBytes(m) > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckStringLength checks if a string length exceeds the max_string_concat
// limit. Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func CheckStringLength(length int) types.ErrorCode {
	limit := GetMaxStringConcat()
	if limit > 0 && length > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckStringLimit checks if a string exceeds the max_string_concat limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func CheckStringLimit(s string) types.ErrorCode {
	return CheckStringLength(len(s))
}
