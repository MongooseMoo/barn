package builtins

import (
	"barn/db"
	"barn/types"
	"strings"
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
func LoadProtectedBuiltinsFromStore(store *db.Store) {
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
			collectProtectFlags(ref.ID(), store, next)
		}
	}

	protectedBuiltins.Lock()
	protectedBuiltins.set = next
	protectedBuiltins.Unlock()
}

// collectProtectFlags walks the server_options object and its ancestors,
// recording every `protect_<name>` property whose value is truthy. Nearer
// definitions win (standard MOO inheritance), so we only record a name the
// first time we see it.
func collectProtectFlags(objID types.ObjID, store *db.Store, out map[string]bool) {
	seen := map[types.ObjID]bool{}
	queue := []types.ObjID{objID}
	decided := map[string]bool{} // name -> already resolved by a nearer object

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true

		obj := store.Get(cur)
		if obj == nil {
			continue
		}
		for propName, prop := range obj.Properties {
			if !strings.HasPrefix(propName, protectPrefix) {
				continue
			}
			name := propName[len(protectPrefix):]
			if name == "" || decided[name] {
				continue
			}
			// A "clear" slot defers to the parent's value, so don't resolve the
			// name here — let a nearer-to-root ancestor provide the value.
			if prop.Clear {
				continue
			}
			decided[name] = true
			if prop.Value != nil && prop.Value.Truthy() {
				out[name] = true
			}
		}
		queue = append(queue, obj.Parents...)
	}
}
