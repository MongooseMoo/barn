package builtins

import (
	"sync/atomic"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
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
// Toast caches these flags whenever load_server_options() runs; we mirror that
// with protectedBuiltins below.
const protectPrefix = "protect_"

// protectedBuiltins holds an IMMUTABLE snapshot of the protected-builtin set
// behind an atomic.Pointer. The set is read on every builtin call but written
// only when the database refreshes it (boot + load_server_options()), so the
// classic single-writer/many-reader pattern applies.
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
var protectedBuiltins atomic.Pointer[map[string]bool]

func init() {
	empty := map[string]bool{}
	protectedBuiltins.Store(&empty)
}

// IsProtectedBuiltin reports whether the named builtin is currently protected
// per the loaded $server_options. Lock-free: a single atomic load + read-only
// map index.
func IsProtectedBuiltin(name string) bool {
	return (*protectedBuiltins.Load())[name]
}

// LoadProtectedBuiltinsFromStore rescans $server_options for protect_<name>
// flags and replaces the protected-builtin set. Called from
// LoadServerOptionsFromStore so it stays in sync with Toast's cache refresh.
func LoadProtectedBuiltinsFromStore(store *dbstore.Store) {
	if store == nil {
		applyProtectedBuiltins(nil)
		return
	}
	flags := collectProtectedBuiltins(
		func(objID types.ObjID, name string) (dbstore.PropertyView, bool) {
			prop, err := store.FindProperty(objID, name)
			if err != types.E_NONE {
				return dbstore.PropertyView{}, false
			}
			return prop, true
		},
		func(objID types.ObjID, prefix string) (map[string]bool, bool) {
			flags, errCode := store.TruthyPropertiesWithPrefixInAncestry(objID, prefix)
			if errCode != types.E_NONE {
				return nil, false
			}
			return flags, true
		},
	)
	applyProtectedBuiltins(flags)
}

// LoadProtectedBuiltinsForTask refreshes protected-builtin flags through the
// active task view so same-task $server_options writes are visible when
// load_server_options() runs.
func LoadProtectedBuiltinsForTask(ctx *kernel.TaskContext) {
	if ctx == nil {
		applyProtectedBuiltins(nil)
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
			if ctx.StoreTxn != nil {
				flags, errCode := ctx.StoreTxn.TruthyPropertiesWithPrefixInAncestry(objID, prefix)
				if errCode == types.E_NONE {
					return flags, true
				}
				return nil, false
			}
			if ctx.Store == nil {
				return nil, false
			}
			flags, errCode := ctx.Store.TruthyPropertiesWithPrefixInAncestry(objID, prefix)
			if errCode != types.E_NONE {
				return nil, false
			}
			return flags, true
		},
	)
	if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
		pending := pendingServerOptions(ctx)
		if pending == nil {
			snapshot := defaultServerOptionsSnapshot()
			enqueuePendingEffect(ctx, kernel.PendingEffect{
				Kind:          kernel.PendingEffectServerOptions,
				ServerOptions: snapshot,
			})
			pending = pendingServerOptions(ctx)
		}
		pending.ProtectedBuiltins = flags
		return
	}
	applyProtectedBuiltins(flags)
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

func applyProtectedBuiltins(next map[string]bool) {
	if next == nil {
		next = map[string]bool{}
	}
	// Publish the freshly-built (and henceforth immutable) map with a single
	// atomic swap; readers loading after this see the new set in full.
	protectedBuiltins.Store(&next)
}
