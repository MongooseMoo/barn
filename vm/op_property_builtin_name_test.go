package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Toast matches built-in property names with strcasecmp
// (src/db_properties.cc db_find_property), which folds ASCII letters only.
// "wİzard" (LATIN CAPITAL LETTER I WITH DOT ABOVE) lowers to "wizard"
// under Unicode case mapping, but on Toast it is an ordinary property name.
func TestBuiltinPropertyNamesFoldASCIIOnly(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	for _, name := range []string{"WiZaRd", "LOCATION", "Last_Move", "R"} {
		if !bytecode.IsBuiltinPropertyName(name) {
			t.Fatalf("IsBuiltinPropertyName(%q) = false, want true", name)
		}
	}

	name := "wİzard"
	if bytecode.IsBuiltinPropertyName(name) {
		t.Fatalf("IsBuiltinPropertyName(%q) = true, want false", name)
	}
	if value, ok := getBuiltinProperty(store.DirectTxn(), 0, name); ok {
		t.Fatalf("getBuiltinProperty(%q) = (%v, true), want false", name, value)
	}
	if handled, errCode := setBuiltinProperty(newTestSession(builtins.NewRegistry()), store.DirectTxn(), 0, name, types.NewInt(1), nil); handled || errCode != types.E_NONE {
		t.Fatalf("setBuiltinProperty(%q) = (%v, %v), want (false, E_NONE)", name, handled, errCode)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer} {
		if set, _ := store.DirectTxn().HasObjectFlag(0, flag); set {
			t.Fatalf("flag %v set by a write to %q", flag, name)
		}
	}
}
