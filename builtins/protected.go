package builtins

import (
	dbstore "barn/db/store"
	"barn/types"
	"sync"
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

var protectedBuiltins = struct {
	sync.RWMutex
	set map[string]bool
}{set: map[string]bool{}}

// IsProtectedBuiltin reports whether the named builtin is currently protected
// per the loaded $server_options.
func IsProtectedBuiltin(name string) bool {
	protectedBuiltins.RLock()
	defer protectedBuiltins.RUnlock()
	return protectedBuiltins.set[name]
}

// LoadProtectedBuiltinsFromStore rescans $server_options for protect_<name>
// flags and replaces the protected-builtin set. Called from
// LoadServerOptionsFromStore so it stays in sync with Toast's cache refresh.
func LoadProtectedBuiltinsFromStore(store *dbstore.Store) {
	next := map[string]bool{}
	if store == nil {
		protectedBuiltins.Lock()
		protectedBuiltins.set = next
		protectedBuiltins.Unlock()
		return
	}

	serverOptsProp, err := store.FindProperty(0, "server_options")
	if err == types.E_NONE {
		if ref, ok := serverOptsProp.Value.(types.ObjValue); ok {
			flags, errCode := store.TruthyPropertiesWithPrefixInAncestry(ref.ID(), protectPrefix)
			if errCode == types.E_NONE {
				next = flags
			}
		}
	}

	protectedBuiltins.Lock()
	protectedBuiltins.set = next
	protectedBuiltins.Unlock()
}
