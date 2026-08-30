package builtins

import (
	"sync"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Execution carries the language-visible task state together with the runtime
// services required for one builtin call. The VM constructs it explicitly for
// production dispatch; tests of pure builtins can use Session.NewExecution
// with a nil Task and callbacks.
type Execution struct {
	*kernel.TaskContext
	Task                 *task.Task
	Registry             *Registry
	Session              *Session
	PushEval             func(*bytecode.Program) types.Result
	PushMoveLifecycle    func(MoveLifecycleRequest) types.Result
	CollectAnonymousRefs func(map[types.ObjID]struct{})
	PendingFinalizations func() []types.Value
}

// NewExecution binds task state and an optional concrete task to this session.
func (s *Session) NewExecution(ctx *kernel.TaskContext, task *task.Task) *Execution {
	execution := &Execution{TaskContext: ctx, Task: task, Registry: s.registry, Session: s}
	execution.ensureStoreTxn()
	return execution
}

func (ctx *Execution) ensureStoreTxn() {
	if ctx != nil && ctx.TaskContext != nil && ctx.StoreTxn == nil && ctx.Store != nil {
		ctx.StoreTxn = ctx.Store.DirectTxn()
	}
}

// BuiltinFunc is a function type for builtin functions.
type BuiltinFunc func(ctx *Execution, args []types.Value) types.Result

// VerbCallerFunc is a callback for calling verbs on objects
// Returns the result of calling the verb, or E_VERBNF if verb not found
type VerbCallerFunc func(objID types.ObjID, verbName string, args []types.Value, ctx *Execution) types.Result

// builtinEntry is the per-builtin dispatch record. It is stored once in the
// id-indexed entries slice so CallByID resolves a builtin with a single bounds
// check + slice index instead of two map lookups. It carries the raw
// (un-wrapped) function plus its argument signature so the dispatch path can
// validate args inline, without routing through a per-call validation closure.
type builtinEntry struct {
	name     string
	fn       BuiltinFunc       // builtin plus replay-safety marker; validation stays inline
	sig      functionSignature // valid only when hasSig is true
	hasSig   bool
	lineSync bool
}

// Registry holds all registered builtin functions
type Registry struct {
	// entries is indexed by builtin ID; CallByID/CallByName/NeedsLineSyncByID
	// resolve through it with one slice index, no hashing.
	entries []*builtinEntry
	// funcs maps name -> the validation-wrapping closure. Kept for Get/Has and
	// call_function(), which call the returned fn directly and so must retain
	// the same arg-validation behavior the closure provided.
	funcs    map[string]BuiltinFunc
	nameToID map[string]int

	compilerMu     sync.Mutex
	sourceCompiler *compiler.Compiler
}

// NewRegistry creates a new builtin function registry
func NewRegistry() *Registry {
	r := &Registry{
		funcs:    make(map[string]BuiltinFunc),
		nameToID: make(map[string]int),
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

	// Register object builtins
	r.Register("create", builtinCreate)
	r.Register("recycle", builtinRecycle)
	r.Register("valid", builtinValid)
	r.Register("max_object", builtinMaxObject)
	r.Register("parent", builtinParent)
	r.Register("parents", builtinParents)
	r.Register("children", builtinChildren)
	r.Register("ancestors", builtinAncestors)
	r.Register("descendants", builtinDescendants)
	r.Register("isa", builtinIsa)
	r.Register("chparent", builtinChparent)
	r.Register("chparents", builtinChparents)
	r.Register("move", builtinMove)
	r.Register("is_player", builtinIsPlayer)
	r.Register("set_player_flag", builtinSetPlayerFlag)
	r.Register("players", builtinPlayers)
	r.Register("occupants", builtinOccupants)
	r.Register("renumber", builtinRenumber)
	r.Register("new_waif", builtinNewWaif)
	r.Register("object_bytes", builtinObjectBytes)

	// Register property builtins
	r.Register("properties", builtinProperties)
	r.Register("property_info", builtinPropertyInfo)
	r.Register("set_property_info", builtinSetPropertyInfo)
	r.Register("add_property", builtinAddProperty)
	r.Register("delete_property", builtinDeleteProperty)
	r.Register("clear_property", builtinClearProperty)
	r.Register("is_clear_property", builtinIsClearProperty)

	// Register verb builtins
	r.Register("respond_to", builtinRespondTo)
	r.Register("verbs", builtinVerbs)
	r.Register("verb_info", builtinVerbInfo)
	r.Register("verb_args", builtinVerbArgs)
	r.Register("verb_code", builtinVerbCode)
	r.Register("add_verb", builtinAddVerb)
	r.Register("delete_verb", builtinDeleteVerb)
	r.Register("set_verb_info", builtinSetVerbInfo)
	r.Register("set_verb_args", builtinSetVerbArgs)
	r.Register("set_verb_code", builtinSetVerbCode)
	r.Register("disassemble", builtinDisassemble)

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

	// Register crypto/encoding builtins
	r.Register("encode_base64", builtinEncodeBase64)
	r.Register("decode_base64", builtinDecodeBase64)
	r.Register("encode_binary", builtinEncodeBinary)
	r.Register("decode_binary", builtinDecodeBinary)
	r.Register("crypt", builtinCrypt)

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
	r.Register("call_function", builtinCallFunction)
	r.Register("function_info", builtinFunctionInfo)
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
	r.Register("load_server_options", builtinLoadServerOptions)
	r.Register("locate_by_name", builtinLocateByName)
	r.Register("locations", builtinLocations)
	r.Register("owned_objects", builtinOwnedObjects)
	r.Register("next_recycled_object", builtinNextRecycledObject)
	r.Register("recycled_objects", builtinRecycledObjects)
	r.Register("recreate", builtinRecreate)
	r.Register("waif_stats", builtinWaifStats)
	r.Register("verb_cache_stats", builtinVerbCacheStats)
	r.Register("reset_max_object", builtinResetMaxObject)
	r.Register("value_bytes", builtinValueBytes)

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

	// eval() is registered by the bytecode VM registry.
	// to avoid circular dependencies (eval needs parser which needs eval)

	return r
}

// Register adds a builtin function to the registry
func (r *Registry) Register(name string, fn BuiltinFunc) {
	r.compilerMu.Lock()
	defer r.compilerMu.Unlock()

	invoke := fn
	if builtinHasIrreversibleSideEffect(name) {
		invoke = func(ctx *Execution, args []types.Value) types.Result {
			if ctx != nil && ctx.TaskContext != nil {
				ctx.IrreversibleSideEffect = true
			}
			return fn(ctx, args)
		}
	}
	entry := &builtinEntry{
		name:     name,
		fn:       invoke,
		lineSync: needsCallStackLineSync(name),
	}

	// The validation-wrapping closure is preserved ONLY in the funcs map, for
	// Get()/Has()/call_function(), which invoke the returned fn directly. The
	// hot CallByID/CallByName path uses entry.fn + inline validation instead, so
	// it never pays for the closure indirection.
	stored := invoke
	if sig, ok := lookupFunctionSignature(name); ok {
		entry.sig = sig
		entry.hasSig = true
		inner := invoke
		stored = func(ctx *Execution, args []types.Value) types.Result {
			if err := validateKnownFunctionArgs(name, sig, args); err != types.E_NONE {
				return types.Err(err)
			}
			return inner(ctx, args)
		}
	}

	id := len(r.entries)
	r.entries = append(r.entries, entry)
	r.funcs[name] = stored
	r.nameToID[name] = id
	r.sourceCompiler = nil
}

// Compiler returns the source compiler for the registry's current builtin-ID
// layout. Register invalidates it so a later compilation cannot reuse bytecode
// produced before the layout changed.
func (r *Registry) Compiler() *compiler.Compiler {
	r.compilerMu.Lock()
	defer r.compilerMu.Unlock()
	if r.sourceCompiler != nil {
		return r.sourceCompiler
	}
	builtinIDs := make(map[string]int, len(r.nameToID))
	for name, id := range r.nameToID {
		builtinIDs[name] = id
	}
	r.sourceCompiler = compiler.New(builtinIDs)
	return r.sourceCompiler
}

// builtinHasIrreversibleSideEffect reports builtins whose real implementation
// changes state outside StoreTxn and cannot safely be repeated by whole-task
// conflict retry. Database mutations use LiveStoreMutated instead; notify,
// switch_player, boot_player, and load_server_options are buffered until commit.
func builtinHasIrreversibleSideEffect(name string) bool {
	switch name {
	case "reseed_random",
		"listen", "unlisten", "set_connection_option", "open_network_connection", "read_http", "flush_input", "force_input", "curl",
		"file_open", "file_close", "file_read", "file_readline", "file_readlines", "file_write", "file_writeline", "file_flush", "file_seek", "file_remove", "file_rename", "file_mkdir", "file_rmdir", "file_chmod",
		"sqlite_open", "sqlite_close", "sqlite_query", "sqlite_execute", "sqlite_limit", "sqlite_interrupt",
		"dump_database", "read_stdin", "shutdown", "exec", "server_log", "run_gc", "reset_max_object",
		"kill_task", "resume":
		return true
	default:
		return false
	}
}

func needsCallStackLineSync(name string) bool {
	return name == "callers" || name == "task_stack"
}

// NeedsLineSyncByID reports whether a builtin must see VM frame line numbers
// flushed into the task activation stack before it runs.
func (r *Registry) NeedsLineSyncByID(id int) bool {
	if id < 0 || id >= len(r.entries) {
		return false
	}
	return r.entries[id].lineSync
}

// GetID returns the ID for a builtin function name
func (r *Registry) GetID(name string) (int, bool) {
	id, ok := r.nameToID[name]
	return id, ok
}

// CallByIDWithExecution calls a builtin with explicitly supplied runtime services.
func (s *Session) CallByIDWithExecution(id int, ctx *Execution, args []types.Value) types.Result {
	r := s.registry
	if id < 0 || id >= len(r.entries) {
		return types.Err(types.E_VERBNF)
	}
	if ctx == nil || ctx.Registry != r || ctx.Session != s {
		return types.Err(types.E_INVARG)
	}
	ctx.ensureStoreTxn()
	return s.dispatch(r.entries[id], ctx, args)
}

// CallByNameWithExecution calls a named builtin with explicit runtime services.
func (s *Session) CallByNameWithExecution(name string, ctx *Execution, args []types.Value) (types.Result, bool) {
	r := s.registry
	id, ok := r.nameToID[name]
	if !ok {
		return types.Result{}, false
	}
	if ctx == nil || ctx.Registry != r || ctx.Session != s {
		return types.Err(types.E_INVARG), true
	}
	ctx.ensureStoreTxn()
	return s.dispatch(r.entries[id], ctx, args), true
}

// dispatch runs a builtin, first giving ToastStunt's protected-builtin
// redirection a chance to intercept the call, then validating arguments inline.
//
// Order is load-bearing and matches the pre-refactor behavior: the protected
// redirect is evaluated BEFORE argument validation, so a redirected call passes
// the raw args to #0:bf_<name> unvalidated. Only when the call falls through to
// the real builtin do we run the same arg-count/type checks the registration
// closure used to perform (identical E_ARGS/E_TYPE codes).
func (s *Session) dispatch(e *builtinEntry, ctx *Execution, args []types.Value) types.Result {
	if redirect, ok := s.maybeProtectedRedirect(e.name, ctx, args); ok {
		return redirect
	}
	if e.hasSig {
		if err := validateKnownFunctionArgs(e.name, e.sig, args); err != types.E_NONE {
			return types.Err(err)
		}
	}
	return e.fn(ctx, args)
}

// maybeProtectedRedirect implements ToastStunt's protected-builtin dispatch.
// When the builtin is protected and is being called from a verb whose `this`
// is not #0, the call is redirected to `#0:bf_<name>(@args)`:
//   - if that verb exists, its result (return or raise) becomes the call result;
//   - if it does not exist, a wizard caller falls through to the real builtin
//     (ok=false) and a non-wizard caller gets E_PERM.
//
// Returns (result, true) when the call was handled by the redirect path, or
// (_, false) when the caller should run the real builtin normally.
func (s *Session) maybeProtectedRedirect(name string, ctx *Execution, args []types.Value) (types.Result, bool) {
	if ctx == nil || name == "" {
		return types.Result{}, false
	}
	// caller() == #0 (the bf_ wrapper, or any #0 verb) runs the real builtin.
	if ctx.ThisObj == types.ObjID(0) {
		return types.Result{}, false
	}
	if !s.IsProtectedBuiltin(name) {
		return types.Result{}, false
	}
	store := ctx.Store
	if store == nil {
		return types.Result{}, false
	}
	bfName := "bf_" + name
	_, _, err := findVerbForRead(ctx, types.ObjID(0), bfName)
	if err == nil {
		// #0:bf_<name> exists: run it and use its outcome (return or raise).
		verbArgs := append([]types.Value(nil), args...)
		return s.CallVerb(types.ObjID(0), bfName, verbArgs, ctx), true
	}
	// No wrapper verb: wizards fall through to the real builtin, others denied.
	if !ctx.IsWizard {
		return types.Err(types.E_PERM), true
	}
	return types.Result{}, false
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

// CallVerb calls a verb on an object using the registered verb caller
// Returns E_VERBNF if no verb caller is set or if the verb is not found
func (s *Session) CallVerb(objID types.ObjID, verbName string, args []types.Value, ctx *Execution) types.Result {
	if s.host.VerbCaller == nil {
		return types.Err(types.E_VERBNF)
	}
	return s.host.VerbCaller(objID, verbName, args, ctx)
}
