package builtins

import (
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/config"
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
	Task     *task.Task
	Registry *Registry
	Session  *Session
	PushEval func(*bytecode.Program) types.Result
	// PushProtectedVerb runs an executable #0 wrapper on the calling VM.
	PushProtectedVerb    func(string, []types.Value) types.Result
	PushMoveLifecycle    func(MoveLifecycleRequest) types.Result
	PushRecycleLifecycle func(RecycleLifecycleRequest) types.Result
	CollectAnonymousRefs func(map[types.ObjID]struct{})
	PendingFinalizations func() []types.Value
}

// NewExecution binds task state and an optional concrete task to this session.
func (s *Session) NewExecution(ctx *kernel.TaskContext, task *task.Task) *Execution {
	execution := &Execution{TaskContext: ctx, Task: task, Registry: s.registry, Session: s}
	execution.ensureStoreTxn()
	return execution
}

// Rebind points a reused Execution at the current task state. Callers that
// issue many builtin calls from one VM keep a single Execution (and its
// service closures) alive instead of allocating one per call.
func (ctx *Execution) Rebind(taskCtx *kernel.TaskContext, task *task.Task) {
	ctx.TaskContext = taskCtx
	ctx.Task = task
	ctx.ensureStoreTxn()
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
	name       string
	id         int         // index in Registry.entries; keys protectedSet.byID
	fn         BuiltinFunc // builtin plus replay-safety marker; validation stays inline
	sig        Signature
	visibility Visibility
	lineSync   bool
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

	sourceCompiler *compiler.Compiler
}

// NewRegistry constructs Barn's default base registry. VM clients contribute
// their descriptors using NewRegistryFromDescriptors before creating sessions.
func NewRegistry() *Registry {
	r, err := NewRegistryFromDescriptors(config.DefaultCapabilities(), append(BaseDescriptors(), AppendedDescriptors()...))
	if err != nil {
		panic(err)
	}
	return r
}

// install is used only while constructing a validated immutable registry.
func (r *Registry) install(d Descriptor) {
	name, fn := d.Name, d.Implementation

	invoke := fn
	if d.Effect == Irreversible || d.Effect == CommitGateReentrant {
		guarded := d.Effect == Irreversible
		invoke = func(ctx *Execution, args []types.Value) types.Result {
			if ctx != nil && ctx.TaskContext != nil {
				// The first irreversible effect of an attempt is the runtime's last
				// chance to re-run the task instead of letting a later commit
				// conflict surface as an uncatchable error. If it asks to stop,
				// return without performing the effect; the VM unwinds and runTask
				// re-runs the task.
				if guarded && !beginIrreversible(ctx) {
					return abortedAttempt()
				}
				ctx.IrreversibleSideEffect = true
			}
			return fn(ctx, args)
		}
	}
	entry := &builtinEntry{
		name:       name,
		fn:         invoke,
		lineSync:   d.LineSync,
		visibility: d.Visibility,
	}

	// The validation-wrapping closure is preserved ONLY in the funcs map, for
	// Get()/Has()/call_function(), which invoke the returned fn directly. The
	// hot CallByID/CallByName path uses entry.fn + inline validation instead, so
	// it never pays for the closure indirection.
	sig := cloneSignature(d.Signature)
	entry.sig = sig
	stored := func(ctx *Execution, args []types.Value) types.Result {
		if err := validateFunctionArgs(sig, args); err != types.E_NONE {
			return functionArgError(sig, args, err)
		}
		return invoke(ctx, args)
	}

	id := len(r.entries)
	entry.id = id
	r.entries = append(r.entries, entry)
	r.funcs[name] = stored
	r.nameToID[name] = id
}

// protectedByID projects a by-name protected set onto the current builtin-ID
// immutable layout (see protectedSet).
func (r *Registry) protectedByID(flags map[string]bool) []bool {
	if len(flags) == 0 {
		return nil
	}
	byID := make([]bool, len(r.entries))
	for name := range flags {
		if !flags[name] {
			continue
		}
		if id, ok := r.nameToID[name]; ok {
			byID[id] = true
		}
	}
	return byID
}

// Compiler returns the source compiler bound to this immutable builtin layout.
func (r *Registry) Compiler() *compiler.Compiler {
	return r.sourceCompiler
}

// NeedsLineSyncByID reports whether a builtin must see VM frame line numbers
// flushed into the task activation stack before it runs.
func (r *Registry) NeedsLineSyncByID(id int) bool {
	if id < 0 || id >= len(r.entries) || r.entries[id] == nil {
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
	if id < 0 || id >= len(r.entries) || r.entries[id] == nil {
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
	// Cheap pre-check (nil ctx, #0 caller, unprotected entry) before the
	// redirect helper so the common case never builds and copies its 88-byte
	// Result. The helper repeats these checks; they are the same predicate.
	if ctx != nil && ctx.ThisObj != types.ObjID(0) && s.isProtectedEntryFor(ctx, e) {
		if redirect, ok := s.maybeProtectedRedirect(e.name, ctx, args); ok {
			return redirect
		}
	}
	if err := validateFunctionArgs(e.sig, args); err != types.E_NONE {
		return functionArgError(e.sig, args, err)
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
	if !s.isProtectedNameFor(ctx, name) {
		return types.Result{}, false
	}
	store := ctx.Store
	if store == nil {
		return types.Result{}, false
	}
	bfName := "bf_" + name
	_, _, err := findCallableVerbForRead(ctx, types.ObjID(0), bfName)
	if err == nil {
		// #0:bf_<name> exists: run it and use its outcome (return or raise).
		verbArgs := append([]types.Value(nil), args...)
		if ctx.PushProtectedVerb != nil {
			return ctx.PushProtectedVerb(bfName, verbArgs), true
		}
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
