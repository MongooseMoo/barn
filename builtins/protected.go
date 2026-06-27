package builtins

import (
	"sync/atomic"

	dbstore "barn/db/store"
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
	next := map[string]bool{}
	if store == nil {
		protectedBuiltins.Store(&next)
		return
	}

	serverOptsProp, err := store.FindProperty(0, "server_options")
	if err == types.E_NONE {
		if ref := serverOptsProp.Value; isObjectRef(ref) {
			flags, errCode := store.TruthyPropertiesWithPrefixInAncestry(ref.ID(), protectPrefix)
			if errCode == types.E_NONE {
				next = flags
			}
		}
	}

	// Publish the freshly-built (and henceforth immutable) map with a single
	// atomic swap; readers loading after this see the new set in full.
	protectedBuiltins.Store(&next)
}
