package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestBuiltinPropertyPseudoSetMatchesToast(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	for _, name := range []string{"parent", "children", "player"} {
		if builtins.IsBuiltinProperty(name) {
			t.Fatalf("IsBuiltinProperty(%q) = true, want false", name)
		}
		if value, ok := getBuiltinProperty(store, nil, 0, name); ok {
			t.Fatalf("getBuiltinProperty(%q) = (%v, true), want false", name, value)
		}
		if handled, errCode := setBuiltinProperty(store, nil, 0, name, types.NewInt(1), nil); handled || errCode != types.E_NONE {
			t.Fatalf("setBuiltinProperty(%q) = (%v, %v), want (false, E_NONE)", name, handled, errCode)
		}
	}

	if !builtins.IsBuiltinProperty("last_move") {
		t.Fatalf("IsBuiltinProperty(last_move) = false, want true")
	}
	value, ok := getBuiltinProperty(store, nil, 0, "last_move")
	if !ok {
		t.Fatalf("getBuiltinProperty(last_move) ok = false, want true")
	}
	if value.Type() != types.TYPE_MAP {
		t.Fatalf("last_move = %T, want MapValue", value)
	}
	handled, errCode := setBuiltinProperty(store, nil, 0, "last_move", types.NewList(nil), nil)
	if !handled || errCode != types.E_PERM {
		t.Fatalf("setBuiltinProperty(last_move) = (%v, %v), want (true, E_PERM)", handled, errCode)
	}
}

func TestBuiltinPropertyWritePermissions(t *testing.T) {
	tests := []struct {
		name  string
		value types.Value
	}{
		{"name", types.NewStr("renamed")},
		{"owner", types.NewObj(2)},
		{"programmer", types.NewInt(1)},
		{"wizard", types.NewInt(1)},
		{"r", types.NewInt(1)},
		{"w", types.NewInt(1)},
		{"f", types.NewInt(1)},
		{"a", types.NewInt(1)},
		{"location", types.NewObj(1)},
		{"contents", types.NewList(nil)},
		{"last_move", types.NewMap(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := builtinPropertyTestStore(t, false)
			ctx := &kernel.TaskContext{Programmer: 2, Store: store}
			handled, errCode := setBuiltinProperty(store, nil, 1, tt.name, tt.value, ctx)
			if !handled || errCode != types.E_PERM {
				t.Fatalf("setBuiltinProperty(%q) = (%v, %v), want (true, E_PERM)", tt.name, handled, errCode)
			}
		})
	}
}

func TestBuiltinPropertyOwnerAndWizardRules(t *testing.T) {
	for _, name := range []string{"name", "r", "w", "f", "a"} {
		t.Run("owner_can_set_"+name, func(t *testing.T) {
			store := builtinPropertyTestStore(t, false)
			value := types.NewInt(1)
			if name == "name" {
				value = types.NewStr("renamed")
			}
			if _, errCode := setBuiltinProperty(store, nil, 1, name, value, &kernel.TaskContext{Programmer: 1, Store: store}); errCode != types.E_NONE {
				t.Fatalf("owner setting %q returned %v", name, errCode)
			}
		})
	}

	for _, name := range []string{"owner", "programmer", "wizard"} {
		t.Run("owner_cannot_set_"+name, func(t *testing.T) {
			store := builtinPropertyTestStore(t, false)
			value := types.NewInt(1)
			if name == "owner" {
				value = types.NewObj(2)
			}
			if _, errCode := setBuiltinProperty(store, nil, 1, name, value, &kernel.TaskContext{Programmer: 1, Store: store}); errCode != types.E_PERM {
				t.Fatalf("owner setting %q returned %v, want E_PERM", name, errCode)
			}
		})
	}

	store := builtinPropertyTestStore(t, true)
	if _, errCode := setBuiltinProperty(store, nil, 1, "name", types.NewStr("renamed"), &kernel.TaskContext{Programmer: 1, Store: store}); errCode != types.E_PERM {
		t.Fatalf("owner renaming player returned %v, want E_PERM", errCode)
	}
	wizard := &kernel.TaskContext{Programmer: 0, IsWizard: true, Store: store}
	if _, errCode := setBuiltinProperty(store, nil, 1, "wizard", types.NewInt(1), wizard); errCode != types.E_NONE {
		t.Fatalf("wizard setting wizard returned %v", errCode)
	}
}

func TestBuiltinPropertyErrorOrdering(t *testing.T) {
	store := builtinPropertyTestStore(t, false)
	nonWizard := &kernel.TaskContext{Programmer: 2, Store: store}
	if _, errCode := setBuiltinProperty(store, nil, 1, "owner", types.NewInt(1), nonWizard); errCode != types.E_TYPE {
		t.Fatalf("invalid owner value returned %v, want E_TYPE before E_PERM", errCode)
	}
	if _, errCode := setBuiltinProperty(store, nil, 1, "name", types.NewInt(1), nonWizard); errCode != types.E_TYPE {
		t.Fatalf("invalid name value returned %v, want E_TYPE before E_PERM", errCode)
	}
	if _, errCode := setBuiltinProperty(store, nil, 1, "wizard", types.NewStr("truthy"), nonWizard); errCode != types.E_PERM {
		t.Fatalf("non-wizard wizard assignment returned %v, want E_PERM before value handling", errCode)
	}
}

func TestBuiltinPropertyProtectionOption(t *testing.T) {
	store := builtinPropertyTestStore(t, false)
	options := dbstore.NewObject(3, 0)
	options.SetProperty("protect_r", dbstore.NewProperty(types.NewInt(1), 0, dbstore.PropRead, false, true))
	if err := store.Add(options); err != nil {
		t.Fatalf("Add server options: %v", err)
	}
	if errCode := store.DefineProperty(0, "server_options", dbstore.NewProperty(types.NewObj(3), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty(server_options): %v", errCode)
	}
	builtins.LoadProtectedBuiltinsFromStore(store)
	t.Cleanup(func() { builtins.LoadProtectedBuiltinsFromStore(nil) })

	ctx := &kernel.TaskContext{Programmer: 1, Store: store}
	if _, errCode := setBuiltinProperty(store, nil, 1, "r", types.NewInt(1), ctx); errCode != types.E_PERM {
		t.Fatalf("owner setting protected r returned %v, want E_PERM", errCode)
	}
}

func builtinPropertyTestStore(t *testing.T, user bool) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 1, 2} {
		obj := dbstore.NewObject(id, id)
		if id == 1 {
			obj.SetOwner(1)
			if user {
				obj.SetFlags(dbstore.FlagUser)
			}
		}
		if err := store.Add(obj); err != nil {
			t.Fatalf("Add(%d): %v", id, err)
		}
	}
	return store
}
