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
	r.Register("all_members", builtinAllMembers)
	r.Register("chr", builtinChr)
	r.Register("parse_ansi", builtinParseAnsi)
	r.Register("remove_ansi", builtinRemoveAnsi)

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
	r.Register("slice", builtinSlice)

	// Register math builtins (Layer 7.3)
	r.Register("abs", builtinAbs)
	r.Register("min", builtinMin)
	r.Register("max", builtinMax)
	r.Register("random", builtinRandom)
	r.Register("frandom", builtinFrandom)
	r.Register("reseed_random", builtinReseedRandom)
	r.Register("sqrt", builtinSqrt)
	r.Register("sin", builtinSin)
	r.Register("cos", builtinCos)
	r.Register("tan", builtinTan)
	r.Register("asin", builtinAsin)
	r.Register("acos", builtinAcos)
	r.Register("acosh", builtinAcosh)
	r.Register("atan", builtinAtan)
	r.Register("atan2", builtinAtan2)
	r.Register("asinh", builtinAsinh)
	r.Register("atanh", builtinAtanh)
	r.Register("sinh", builtinSinh)
	r.Register("cosh", builtinCosh)
	r.Register("tanh", builtinTanh)
	r.Register("exp", builtinExp)
	r.Register("log", builtinLog)
	r.Register("log10", builtinLog10)
	r.Register("cbrt", builtinCbrt)
	r.Register("round", builtinRound)
	r.Register("ceil", builtinCeil)
	r.Register("floor", builtinFloor)
	r.Register("trunc", builtinTrunc)
	r.Register("floatstr", builtinFloatstr)
	r.Register("distance", builtinDistance)
	r.Register("relative_heading", builtinRelativeHeading)
	r.Register("simplex_noise", builtinSimplexNoise)

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
	r.Register("listen", builtinListen)
	r.Register("unlisten", builtinUnlisten)
	r.Register("connected_players", builtinConnectedPlayers)
	r.Register("connection_name", builtinConnectionName)
	r.Register("connection_name_lookup", builtinConnectionNameLookup)
	r.Register("connection_options", builtinConnectionOptions)
	r.Register("boot_player", builtinBootPlayer)
	r.Register("switch_player", builtinSwitchPlayer)
	r.Register("idle_seconds", builtinIdleSeconds)
	r.Register("connected_seconds", builtinConnectedSeconds)
	r.Register("connection_info", builtinConnectionInfo)
	r.Register("set_connection_option", builtinSetConnectionOption)
	r.Register("connection_option", builtinConnectionOption)
	r.Register("open_network_connection", builtinOpenNetworkConnection)
	r.Register("read_http", builtinReadHTTP)
	r.Register("flush_input", builtinFlushInput)
	r.Register("force_input", builtinForceInput)
	r.Register("read", builtinRead)
	r.Register("buffered_output_length", builtinBufferedOutputLength)
	r.Register("output_delimiters", builtinOutputDelimiters)

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
	r.Register("argon2", builtinArgon2)
	r.Register("argon2_verify", builtinArgon2Verify)
	r.Register("curl", builtinCurl)
	r.Register("url_encode", builtinUrlEncode)
	r.Register("url_decode", builtinUrlDecode)
	r.Register("pcre_cache_stats", builtinPcreCacheStats)
	r.Register("pcre_match", builtinPcreMatch)
	r.Register("pcre_replace", builtinPcreReplace)

	// Register file IO extension builtins
	r.Register("file_open", builtinFileOpen)
	r.Register("file_close", builtinFileClose)
	r.Register("file_name", builtinFileName)
	r.Register("file_openmode", builtinFileOpenmode)
	r.Register("file_read", builtinFileRead)
	r.Register("file_readline", builtinFileReadline)
	r.Register("file_readlines", builtinFileReadlines)
	r.Register("file_write", builtinFileWrite)
	r.Register("file_writeline", builtinFileWriteline)
	r.Register("file_flush", builtinFileFlush)
	r.Register("file_seek", builtinFileSeek)
	r.Register("file_tell", builtinFileTell)
	r.Register("file_eof", builtinFileEOF)
	r.Register("file_size", builtinFileSize)
	r.Register("file_mode", builtinFileMode)
	r.Register("file_last_access", builtinFileLastAccess)
	r.Register("file_last_change", builtinFileLastChange)
	r.Register("file_last_modify", builtinFileLastModify)
	r.Register("file_stat", builtinFileStat)
	r.Register("file_type", builtinFileType)
	r.Register("file_remove", builtinFileRemove)
	r.Register("file_rename", builtinFileRename)
	r.Register("file_mkdir", builtinFileMkdir)
	r.Register("file_rmdir", builtinFileRmdir)
	r.Register("file_chmod", builtinFileChmod)
	r.Register("file_list", builtinFileList)
	r.Register("file_handles", builtinFileHandles)
	r.Register("file_count_lines", builtinFileCountLines)
	r.Register("file_grep", builtinFileGrep)

	// Register sqlite extension builtins
	r.Register("sqlite_open", builtinSqliteOpen)
	r.Register("sqlite_close", builtinSqliteClose)
	r.Register("sqlite_handles", builtinSqliteHandles)
	r.Register("sqlite_info", builtinSqliteInfo)
	r.Register("sqlite_query", builtinSqliteQuery)
	r.Register("sqlite_execute", builtinSqliteExecute)
	r.Register("sqlite_last_insert_row_id", builtinSqliteLastInsertRowID)
	r.Register("sqlite_limit", builtinSqliteLimit)
	r.Register("sqlite_interrupt", builtinSqliteInterrupt)

	// Register system builtins
	r.Register("background_test", builtinBackgroundTest)
	r.Register("call_function", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinCallFunction(ctx, args, r)
	})
	r.Register("function_info", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinFunctionInfo(ctx, args, r)
	})
	r.Register("db_disk_size", builtinDbDiskSize)
	r.Register("dump_database", builtinDumpDatabase)
	r.Register("getenv", builtinGetenv)
	r.Register("read_stdin", builtinReadStdin)
	r.Register("spellcheck", builtinSpellcheck)
	r.Register("set_thread_mode", builtinSetThreadMode)
	r.Register("shutdown", builtinShutdown)
	r.Register("task_local", builtinTaskLocal)
	r.Register("set_task_local", builtinSetTaskLocal)
	r.Register("task_id", builtinTaskID)
	r.Register("ticks_left", builtinTicksLeft)
	r.Register("seconds_left", builtinSecondsLeft)
	r.Register("task_perms", builtinTaskPerms)
	r.Register("queue_info", builtinQueueInfo)
	r.Register("finished_tasks", builtinFinishedTasks)
	r.Register("thread_pool", builtinThreadPool)
	r.Register("threads", builtinThreads)
	r.Register("usage", builtinUsage)
	r.Register("malloc_stats", builtinMallocStats)
	r.Register("memory_usage", builtinMemoryUsage)
	r.Register("log_cache_stats", builtinLogCacheStats)
	r.Register("exec", builtinExec)
	r.Register("server_log", builtinServerLog)
	r.Register("server_version", builtinServerVersion)
	r.Register("time", builtinTime)
	r.Register("ftime", builtinFtime)
	r.Register("ctime", builtinCtime)

	// GC builtins
	r.Register("run_gc", builtinRunGC)
	r.Register("gc_stats", builtinGCStats)

	// Task management builtins
	r.Register("queued_tasks", builtinQueuedTasks)
	r.Register("kill_task", builtinKillTask)
	r.Register("task_stack", builtinTaskStack)
	r.Register("suspend", builtinSuspend)
	r.Register("resume", builtinResume)
	r.Register("callers", builtinCallers)
	r.Register("set_task_perms", builtinSetTaskPerms)
	r.Register("caller_perms", builtinCallerPerms)
	r.Register("raise", builtinRaise)
	r.Register("yin", builtinYin)

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
	r.Register("locate_by_name", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinLocateByName(ctx, args, store)
	})
	r.Register("locations", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinLocations(ctx, args, store)
	})
	r.Register("owned_objects", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinOwnedObjects(ctx, args, store)
	})
	r.Register("next_recycled_object", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinNextRecycledObject(ctx, args, store)
	})
	r.Register("recycled_objects", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinRecycledObjects(ctx, args, store)
	})
	r.Register("recreate", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinRecreate(ctx, args, store)
	})
	r.Register("waif_stats", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinWaifStats(ctx, args, store)
	})
	r.Register("verb_cache_stats", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinVerbCacheStats(ctx, args, store)
	})
	r.Register("reset_max_object", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinResetMaxObject(ctx, args, store)
	})
	r.Register("value_bytes", builtinValueBytes)

	// Re-register set_task_perms with store access so it can update
	// ctx.IsWizard when the programmer changes (matches Toast's behavior
	// where changing progr affects all subsequent wizard checks).
	r.Register("set_task_perms", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinSetTaskPermsWithStore(ctx, args, store)
	})
}
