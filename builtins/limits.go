package builtins

import (
	"math"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

// ============================================================================
// STRING AND VALUE LIMIT CHECKING
// ============================================================================

// Each Registry caches server options (matching ToastStunt's
// _server_int_option_cache). load_server_options() updates only the executing
// registry and limit-checking functions read that same registry.
const (
	defaultMaxStringConcat    = 64537861
	defaultMaxListValueBytes  = 64537861
	defaultMaxMapValueBytes   = 64537861
	defaultFgTicks            = 60000
	defaultBgTicks            = 30000
	defaultFgSeconds          = 5.0
	defaultBgSeconds          = 3.0
	defaultMaxStackDepth      = 50
	defaultMaxCryptBcryptCost = 14
	defaultMaxCryptSHARounds  = 1_000_000
	minStringConcatLimit      = 1021
	minListValueBytesLimit    = 1021
	minMapValueBytesLimit     = 1021
	maxStringConcatLimit      = math.MaxInt32 - minStringConcatLimit
	maxListValueBytesLimit    = math.MaxInt32 - minListValueBytesLimit
	maxMapValueBytesLimit     = math.MaxInt32 - minMapValueBytesLimit
)

// GetMaxStringConcat returns the cached max_string_concat limit.
// Returns -1 if not set (use default from TaskContext).
func (r *Session) GetMaxStringConcat() int {
	state := &r.runtime.serverOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.maxStringConcat
}

func (r *Session) GetCryptWorkLimits() (int, int) {
	state := &r.runtime.serverOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.maxCryptBcryptCost, state.maxCryptSHARounds
}

func (r *Session) GetTaskLimits(background bool) (int64, float64) {
	state := &r.runtime.serverOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	if background {
		return state.bgTicks, state.bgSeconds
	}
	return state.fgTicks, state.fgSeconds
}

func (r *Session) GetMaxStackDepth(store *dbstore.Store) int {
	if store != nil {
		serverOpts, errCode := store.DirectTxn().FindProperty(0, "server_options")
		if errCode == types.E_NONE && serverOpts.Value.Type() == types.TYPE_OBJ {
			if option, errCode := store.DirectTxn().FindProperty(serverOpts.Value.Obj(), "max_stack_depth"); errCode == types.E_NONE {
				if option.Value.Type() == types.TYPE_INT && option.Value.Int() > defaultMaxStackDepth {
					return int(option.Value.Int())
				}
			}
		}
		return defaultMaxStackDepth
	}

	state := &r.runtime.serverOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.maxStackDepth < defaultMaxStackDepth {
		return defaultMaxStackDepth
	}
	return state.maxStackDepth
}

func findDefinedProperty(objID types.ObjID, name string, store *dbstore.Store) (dbstore.PropertyView, bool) {
	prop, ok, err := store.DefinedProperty(objID, name)
	if err != types.E_NONE || !ok {
		return dbstore.PropertyView{}, false
	}
	return prop, true
}

type propertyReader func(types.ObjID, string) (dbstore.PropertyView, bool)

func (r *Session) cacheServerOptionsDefaults() {
	snapshot := defaultServerOptionsSnapshot()
	r.applyServerOptionsSnapshot(&snapshot)
}

func defaultServerOptionsSnapshot() kernel.PendingServerOptions {
	return kernel.PendingServerOptions{
		MaxStringConcat:    defaultMaxStringConcat,
		MaxListValueBytes:  defaultMaxListValueBytes,
		MaxMapValueBytes:   defaultMaxMapValueBytes,
		FgTicks:            defaultFgTicks,
		BgTicks:            defaultBgTicks,
		FgSeconds:          defaultFgSeconds,
		BgSeconds:          defaultBgSeconds,
		MaxStackDepth:      defaultMaxStackDepth,
		MaxCryptBcryptCost: defaultMaxCryptBcryptCost,
		MaxCryptSHARounds:  defaultMaxCryptSHARounds,
	}
}

func (r *Session) applyServerOptionsSnapshot(snapshot *kernel.PendingServerOptions) {
	if snapshot == nil {
		r.cacheServerOptionsDefaults()
		return
	}
	state := &r.runtime.serverOptions
	state.mu.Lock()
	state.maxStringConcat = snapshot.MaxStringConcat
	state.maxListValueBytes = snapshot.MaxListValueBytes
	state.maxMapValueBytes = snapshot.MaxMapValueBytes
	state.fgTicks = snapshot.FgTicks
	state.bgTicks = snapshot.BgTicks
	state.fgSeconds = snapshot.FgSeconds
	state.bgSeconds = snapshot.BgSeconds
	state.maxStackDepth = snapshot.MaxStackDepth
	state.maxCryptBcryptCost = snapshot.MaxCryptBcryptCost
	state.maxCryptSHARounds = snapshot.MaxCryptSHARounds
	state.mu.Unlock()
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
	if prop, ok := findDefined(serverOptsID, "max_crypt_bcrypt_cost"); ok {
		if prop.Value.Type() == types.TYPE_INT && prop.Value.Int() >= 4 && prop.Value.Int() <= 31 {
			snapshot.MaxCryptBcryptCost = int(prop.Value.Int())
			snapshot.Loaded++
		}
	}
	if prop, ok := findDefined(serverOptsID, "max_crypt_sha_rounds"); ok {
		if prop.Value.Type() == types.TYPE_INT && prop.Value.Int() >= shaCryptRoundsMin && prop.Value.Int() <= shaCryptRoundsMax {
			snapshot.MaxCryptSHARounds = int(prop.Value.Int())
			snapshot.Loaded++
		}
	}

	return snapshot
}

func (r *Session) loadServerOptions(findProperty propertyReader, findDefined propertyReader) int {
	snapshot := collectServerOptions(findProperty, findDefined)
	r.applyServerOptionsSnapshot(&snapshot)
	return snapshot.Loaded
}

// LoadServerOptionsFromStore reads limits from $server_options object and caches them.
// This is called at server startup and when no task transaction is active.
// Returns the number of options successfully loaded.
func (r *Session) LoadServerOptionsFromStore(store *dbstore.Store) int {
	if store == nil {
		return r.loadServerOptions(nil, nil)
	}
	return r.loadServerOptions(
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, err := store.DirectTxn().FindProperty(objID, name)
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
func (r *Session) LoadServerOptionsForTask(ctx *Execution) int {
	if ctx == nil {
		return r.loadServerOptions(nil, nil)
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
	if ctx.StoreTxn.HasWrites() {
		enqueuePendingEffect(ctx, kernel.PendingEffect{
			Kind:          kernel.PendingEffectServerOptions,
			ServerOptions: snapshot,
		})
		return snapshot.Loaded
	}
	r.applyServerOptionsSnapshot(&snapshot)
	return snapshot.Loaded
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
func (r *Session) UpdateContextLimits(ctx *kernel.TaskContext) {
	cachedLimit := r.GetMaxStringConcat()
	if cachedLimit > 0 {
		ctx.MaxStringConcat = cachedLimit
	}
}

// ============================================================================
// VALUE_BYTES() BUILTIN AND HELPERS
// ============================================================================

// builtinValueBytes implements the value_bytes(value) builtin.
// Returns the size in bytes of any MOO value.
func builtinValueBytes(ctx *Execution, args []types.Value) types.Result {
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
func (r *Session) GetMaxListValueBytes() int {
	state := &r.runtime.serverOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.maxListValueBytes
}

// GetMaxMapValueBytes returns the cached max_map_value_bytes limit.
// Returns the currently cached effective limit.
func (r *Session) GetMaxMapValueBytes() int {
	state := &r.runtime.serverOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.maxMapValueBytes
}

// CheckListLimit checks if a list exceeds the max_list_value_bytes limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
// The limit is exclusive - a list with exactly limit bytes is not allowed.
func (r *Session) CheckListLimit(list types.Value) types.ErrorCode {
	limit := r.GetMaxListValueBytes()
	if limit > 0 && ValueBytes(list) >= limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

func (r *Session) CheckListLimitForTask(ctx *kernel.TaskContext, list types.Value) types.ErrorCode {
	limit := r.GetMaxListValueBytes()
	if pending := pendingServerOptions(ctx); pending != nil {
		limit = pending.MaxListValueBytes
	}
	if limit > 0 && ValueBytes(list) >= limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckMapLimit checks if a map exceeds the max_map_value_bytes limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func (r *Session) CheckMapLimit(m types.Value) types.ErrorCode {
	limit := r.GetMaxMapValueBytes()
	if limit > 0 && ValueBytes(m) > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckMapLimitForTask checks a map against the task's pending server options,
// falling back to the currently cached max_map_value_bytes limit.
func (r *Session) CheckMapLimitForTask(ctx *kernel.TaskContext, m types.Value) types.ErrorCode {
	limit := r.GetMaxMapValueBytes()
	if pending := pendingServerOptions(ctx); pending != nil {
		limit = pending.MaxMapValueBytes
	}
	if limit > 0 && ValueBytes(m) > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckStringLength checks if a string length exceeds the max_string_concat
// limit. Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func (r *Session) CheckStringLength(length int) types.ErrorCode {
	limit := r.GetMaxStringConcat()
	if limit > 0 && length > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}

// CheckStringLimit checks if a string exceeds the max_string_concat limit.
// Returns E_QUOTA if limit exceeded, E_NONE otherwise.
func (r *Session) CheckStringLimit(s string) types.ErrorCode {
	return r.CheckStringLength(len(s))
}
