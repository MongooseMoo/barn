package builtins

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

// Protected built-in function support.
//
// ToastStunt lets the database "protect" any built-in function. When a
// protected builtin is called from a verb whose `this` is not #0, the server
// does NOT run the C builtin directly. Instead it calls `#0:bf_<name>(@args)`
// and uses that verb's result (functions.cc, the f->_protected branch). If the
// `bf_<name>` verb does not exist, a wizard-permission caller falls through to
// the real builtin and a non-wizard caller gets E_PERM.
//
// Whether a builtin is protected is driven entirely by the database: the
// property `$server_options.protect_<name>` (truthy) marks `<name>` protected.
// Toast caches these flags whenever load_server_options() runs; each Registry
// owns its own immutable snapshot.
const protectPrefix = "protect_"

// Session.runtime.protected holds an IMMUTABLE snapshot of the protected set.
// The set is read on every builtin call but written only when that registry's
// database refreshes it, so the classic single-writer/many-reader pattern
// applies.
//
// Memory-model contract (see go-perf-atomics-vs-locks): the map a pointer
// references is NEVER mutated after Store; a refresh builds a brand-new map and
// atomically swaps the pointer. atomic.Pointer.Load establishes a
// happens-before edge with the Store that published the map, so a reader sees a
// fully-initialized map and never a half-built one. Readers therefore need no
// lock — they Load the pointer and index a read-only map. This preserves the
// exact observable semantics of the old RWMutex version (which builtins are
// protected, and that a refresh takes effect atomically) while removing the
// per-call RLock atomics from the hot path.

// protectedSet is one immutable snapshot of the protected builtins. byName is
// the source of truth (what the database said); byID is the same set projected
// onto the registry's builtin-ID layout at snapshot time so the per-call check
// in dispatch is a bounds-checked slice index instead of a string hash — the
// analogue of Toast's `f->_protected` flag on the bf_table entry. A builtin
// registered after the snapshot falls outside byID and is answered by byName.
type protectedSet struct {
	byName map[string]bool
	byID   []bool
}

// IsProtectedBuiltin reports whether the named builtin is currently protected
// for this registry. Lock-free: a single atomic load + read-only map index.
func (r *Session) IsProtectedBuiltin(name string) bool {
	if r == nil || r.runtime == nil {
		return false
	}
	set := r.runtime.protected.Load()
	return set != nil && set.byName[name]
}

// isProtectedEntry is IsProtectedBuiltin for the dispatch hot path: no string
// hashing when the entry's ID is covered by the snapshot.
func (r *Session) isProtectedEntry(e *builtinEntry) bool {
	if r.runtime == nil {
		return false
	}
	set := r.runtime.protected.Load()
	if set == nil {
		return false
	}
	if e.id < len(set.byID) {
		return set.byID[e.id]
	}
	return set.byName[e.name]
}

// LoadProtectedBuiltinsFromStore rescans $server_options for protect_<name>
// flags and replaces the protected-builtin set. Called from
// LoadServerOptionsFromStore so it stays in sync with Toast's cache refresh.
func (r *Session) LoadProtectedBuiltinsFromStore(store *dbstore.Store) {
	if store == nil {
		r.applyProtectedBuiltins(nil)
		return
	}
	flags := collectProtectedBuiltins(
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, err := store.DirectTxn().FindProperty(objID, name)
			if err != types.E_NONE {
				return dbstore.PropertyView{}, false
			}
			return prop, true
		},
		func(objID types.ObjID, prefix string) (map[string]bool, bool) {
			flags, errCode := store.DirectTxn().TruthyPropertiesWithPrefixInAncestry(objID, prefix)
			if errCode != types.E_NONE {
				return nil, false
			}
			return flags, true
		},
	)
	r.applyProtectedBuiltins(flags)
}

// LoadProtectedBuiltinsForTask refreshes protected-builtin flags through the
// active task view so same-task $server_options writes are visible when
// load_server_options() runs.
func (r *Session) LoadProtectedBuiltinsForTask(ctx *Execution) {
	if ctx == nil {
		r.applyProtectedBuiltins(nil)
		return
	}
	flags := collectProtectedBuiltins(
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, err := findPropertyForRead(ctx, objID, name)
			if err != types.E_NONE {
				return dbstore.PropertyView{}, false
			}
			return prop, true
		},
		func(objID types.ObjID, prefix string) (map[string]bool, bool) {
			flags, errCode := ctx.StoreTxn.TruthyPropertiesWithPrefixInAncestry(objID, prefix)
			if errCode != types.E_NONE {
				return nil, false
			}
			return flags, true
		},
	)
	if ctx.StoreTxn.HasWrites() {
		pending := pendingServerOptions(ctx.TaskContext)
		if pending == nil {
			snapshot := defaultServerOptionsSnapshot()
			enqueuePendingEffect(ctx, kernel.PendingEffect{
				Kind:          kernel.PendingEffectServerOptions,
				ServerOptions: snapshot,
			})
			pending = pendingServerOptions(ctx.TaskContext)
		}
		pending.ProtectedBuiltins = flags
		return
	}
	r.applyProtectedBuiltins(flags)
}

type protectedFlagReader func(types.ObjID, string) (map[string]bool, bool)

func collectProtectedBuiltins(findProperty propertyReader, findFlags protectedFlagReader) map[string]bool {
	next := map[string]bool{}
	if findProperty == nil || findFlags == nil {
		return next
	}

	if serverOptsProp, ok := findProperty(0, "server_options"); ok {
		if ref := serverOptsProp.Value; isObjectRef(ref) {
			if flags, ok := findFlags(ref.ID(), protectPrefix); ok {
				next = flags
			}
		}
	}

	return next
}

func (r *Session) applyProtectedBuiltins(next map[string]bool) {
	if next == nil {
		next = map[string]bool{}
	}
	// Publish the freshly-built (and henceforth immutable) set with a single
	// atomic swap; readers loading after this see the new set in full.
	r.runtime.protected.Store(&protectedSet{
		byName: next,
		byID:   r.registry.protectedByID(next),
	})
}
