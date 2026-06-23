package vm

import (
	"testing"

	"barn/builtins"
	dbstore "barn/db/store"
	"barn/types"
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
	if _, ok := value.(types.MapValue); !ok {
		t.Fatalf("last_move = %T, want MapValue", value)
	}
	handled, errCode := setBuiltinProperty(store, nil, 0, "last_move", types.NewList(nil), nil)
	if !handled || errCode != types.E_PERM {
		t.Fatalf("setBuiltinProperty(last_move) = (%v, %v), want (true, E_PERM)", handled, errCode)
	}
}
