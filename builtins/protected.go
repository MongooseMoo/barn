package builtins

import (
	"sync"

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
		if ctx.PendingServerOptions == nil {
			snapshot := defaultServerOptionsSnapshot()
			ctx.PendingServerOptions = &snapshot
		}
		ctx.PendingServerOptions.ProtectedBuiltins = flags
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
		if ref, ok := serverOptsProp.Value.(types.ObjValue); ok {
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
	protectedBuiltins.Lock()
	protectedBuiltins.set = next
	protectedBuiltins.Unlock()
}
