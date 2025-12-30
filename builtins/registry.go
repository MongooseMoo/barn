package builtins

import (
	"barn/db"
	"barn/types"
)

// BuiltinFunc is a function type for builtin functions
// Takes a task context and list of arguments, returns a Result
type BuiltinFunc func(ctx *types.TaskContext, args []types.Value) types.Result

// VerbCallerFunc is a callback for calling verbs on objects
// Returns the result of calling the verb, or E_VERBNF if verb not found
type VerbCallerFunc func(objID types.ObjID, verbName string, args []types.Value, ctx *types.TaskContext) types.Result

// Registry holds all registered builtin functions
type Registry struct {
	funcs      map[string]BuiltinFunc
	byID       map[int]BuiltinFunc
	nameToID   map[string]int
	nextID     int
	verbCaller VerbCallerFunc // Callback for calling verbs (set by evaluator)
}

// NewRegistry creates a new builtin function registry
func NewRegistry() *Registry {
	r := &Registry{
		funcs:    make(map[string]BuiltinFunc),
		byID:     make(map[int]BuiltinFunc),
		nameToID: make(map[string]int),
		nextID:   0,
	}

	// Register type conversion builtins
	r.Register("typeof", builtinTypeof)
	r.Register("tostr", builtinTostr)
	r.Register("toint", builtinToint)
	r.Register("tofloat", builtinTofloat)
	r.Register("toliteral", builtinToliteral)
	r.Register("toobj", builtinToobj)
	r.Register("equal", builtinEqual)

	// Register string builtins (Layer 7.1)
	r.Register("length", builtinLength)
	r.Register("strsub", builtinStrsub)
	r.Register("strtr", builtinStrtr)
	r.Register("index", builtinIndex)
	r.Register("rindex", builtinRindex)
	r.Register("strcmp", builtinStrcmp)
	r.Register("upcase", builtinUpcase)
	r.Register("downcase", builtinDowncase)
	r.Register("capitalize", builtinCapitalize)
	r.Register("explode", builtinExplode)
	r.Register("implode", builtinImplode)
	r.Register("trim", builtinTrim)
	r.Register("ltrim", builtinLtrim)
	r.Register("rtrim", builtinRtrim)
	r.Register("match", builtinMatch)
	r.Register("rmatch", builtinRmatch)
	r.Register("substitute", builtinSubstitute)

	// Register list builtins (Layer 7.2)
	r.Register("listappend", builtinListappend)
	r.Register("listinsert", builtinListinsert)
	r.Register("listdelete", builtinListdelete)
	r.Register("listset", builtinListset)
	r.Register("setadd", builtinSetadd)
	r.Register("setremove", builtinSetremove)
	r.Register("is_member", builtinIsMember)
	r.Register("sort", builtinSort)
	r.Register("reverse", builtinReverse)
	r.Register("unique", builtinUnique)

	// Register math builtins (Layer 7.3)
	r.Register("abs", builtinAbs)
	r.Register("min", builtinMin)
	r.Register("max", builtinMax)
	r.Register("random", builtinRandom)
	r.Register("sqrt", builtinSqrt)
	r.Register("sin", builtinSin)
	r.Register("cos", builtinCos)
	r.Register("tan", builtinTan)
	r.Register("asin", builtinAsin)
	r.Register("acos", builtinAcos)
	r.Register("atan", builtinAtan)
	r.Register("sinh", builtinSinh)
	r.Register("cosh", builtinCosh)
	r.Register("tanh", builtinTanh)
	r.Register("exp", builtinExp)
	r.Register("log", builtinLog)
	r.Register("log10", builtinLog10)
	r.Register("ceil", builtinCeil)
	r.Register("floor", builtinFloor)
	r.Register("trunc", builtinTrunc)
	r.Register("floatstr", builtinFloatstr)

	// Register map builtins (Layer 7.5)
	r.Register("mapkeys", builtinMapkeys)
	r.Register("mapvalues", builtinMapvalues)
	r.Register("mapdelete", builtinMapdelete)
	r.Register("maphaskey", builtinMaphaskey)
	r.Register("mapmerge", builtinMapmerge)

	// Register JSON builtins (Layer 10.1)
	r.Register("generate_json", builtinGenerateJson)
	r.Register("parse_json", builtinParseJson)

	// Register network builtins (Layer 12.5)
	r.Register("notify", builtinNotify)
	r.Register("listeners", builtinListeners)
	r.Register("connected_players", builtinConnectedPlayers)
	r.Register("connection_name", builtinConnectionName)
	r.Register("connection_name_lookup", builtinConnectionNameLookup)
	r.Register("boot_player", builtinBootPlayer)
	r.Register("switch_player", builtinSwitchPlayer)
	r.Register("idle_seconds", builtinIdleSeconds)
	r.Register("connected_seconds", builtinConnectedSeconds)
	r.Register("set_connection_option", builtinSetConnectionOption)
	r.Register("connection_option", builtinConnectionOption)

	// Register crypto/encoding builtins (except crypt which needs store)
	r.Register("encode_base64", builtinEncodeBase64)
	r.Register("decode_base64", builtinDecodeBase64)
	r.Register("encode_binary", builtinEncodeBinary)
	r.Register("decode_binary", builtinDecodeBinary)

	// Register hash builtins
	r.Register("string_hash", builtinStringHash)
	r.Register("binary_hash", builtinBinaryHash)
	r.Register("value_hash", builtinValueHash)

	// Register HMAC builtins
	r.Register("string_hmac", builtinStringHmac)
	r.Register("binary_hmac", builtinBinaryHmac)
	r.Register("value_hmac", builtinValueHmac)

	// Register salt and random builtins
	r.Register("salt", builtinSalt)
	r.Register("random_bytes", builtinRandomBytes)

	// Register system builtins
	r.Register("getenv", builtinGetenv)
	r.Register("task_local", builtinTaskLocal)
	r.Register("set_task_local", builtinSetTaskLocal)
	r.Register("task_id", builtinTaskID)
	r.Register("ticks_left", builtinTicksLeft)
	r.Register("seconds_left", builtinSecondsLeft)
	r.Register("exec", builtinExec)
	r.Register("server_log", builtinServerLog)
	r.Register("server_version", builtinServerVersion)
	r.Register("time", builtinTime)
	r.Register("ctime", builtinCtime)

	// GC builtins
	r.Register("run_gc", builtinRunGC)
	r.Register("gc_stats", builtinGCStats)

	// Task management builtins
	r.Register("queued_tasks", builtinQueuedTasks)
	r.Register("kill_task", builtinKillTask)
	r.Register("suspend", builtinSuspend)
	r.Register("resume", builtinResume)
	r.Register("callers", builtinCallers)
	r.Register("set_task_perms", builtinSetTaskPerms)
	r.Register("caller_perms", builtinCallerPerms)
	r.Register("raise", builtinRaise)

	// Note: eval() builtin is registered by the Evaluator via RegisterEvalBuiltin()
	// to avoid circular dependencies (eval needs parser which needs eval)

	return r
}

// Register adds a builtin function to the registry
func (r *Registry) Register(name string, fn BuiltinFunc) {
	r.funcs[name] = fn
	id := r.nextID
	r.byID[id] = fn
	r.nameToID[name] = id
	r.nextID++
}

// GetID returns the ID for a builtin function name
func (r *Registry) GetID(name string) (int, bool) {
	id, ok := r.nameToID[name]
	return id, ok
}

// CallByID calls a builtin function by its ID
func (r *Registry) CallByID(id int, ctx *types.TaskContext, args []types.Value) types.Result {
	fn, ok := r.byID[id]
	if !ok {
		return types.Err(types.E_VERBNF)
	}
	return fn(ctx, args)
}

// Get retrieves a builtin function by name
// Returns (function, true) if found, (nil, false) if not found
func (r *Registry) Get(name string) (BuiltinFunc, bool) {
	fn, ok := r.funcs[name]
	return fn, ok
}

// Has checks if a builtin function is registered
func (r *Registry) Has(name string) bool {
	_, ok := r.funcs[name]
	return ok
}

// SetVerbCaller sets the callback for calling verbs
func (r *Registry) SetVerbCaller(caller VerbCallerFunc) {
	r.verbCaller = caller
}

// CallVerb calls a verb on an object using the registered verb caller
// Returns E_VERBNF if no verb caller is set or if the verb is not found
func (r *Registry) CallVerb(objID types.ObjID, verbName string, args []types.Value, ctx *types.TaskContext) types.Result {
	if r.verbCaller == nil {
		return types.Err(types.E_VERBNF)
	}
	return r.verbCaller(objID, verbName, args, ctx)
}

// RegisterCryptoBuiltins registers crypto builtins that need store access
func (r *Registry) RegisterCryptoBuiltins(store *db.Store) {
	r.Register("crypt", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinCrypt(ctx, args, store)
	})
}

// RegisterSystemBuiltins registers system builtins that need store access
func (r *Registry) RegisterSystemBuiltins(store *db.Store) {
	r.Register("load_server_options", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinLoadServerOptions(ctx, args, store)
	})
	r.Register("value_bytes", builtinValueBytes)
}
