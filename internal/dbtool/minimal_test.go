package dbtool

import (
	"slices"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// The in-memory database is the classic Minimal.db object graph and nothing
// else: four objects, no verbs, no properties.
func TestNewMinimalStoreIsMinimalDBShape(t *testing.T) {
	store, err := NewMinimalStore()
	if err != nil {
		t.Fatalf("NewMinimalStore: %v", err)
	}
	wizardFlags := dbstore.FlagUser | dbstore.FlagProgrammer | dbstore.FlagWizard
	want := []struct {
		id       types.ObjID
		name     string
		flags    dbstore.ObjectFlags
		location types.ObjID
		parents  []types.ObjID
		children []types.ObjID
		contents []types.ObjID
	}{
		{0, "System Object", dbstore.FlagRead, types.ObjNothing, []types.ObjID{1}, []types.ObjID{}, []types.ObjID{}},
		{1, "Root Class", dbstore.FlagRead, types.ObjNothing, []types.ObjID{}, []types.ObjID{0, 2, 3}, []types.ObjID{}},
		{2, "The First Room", 0, types.ObjNothing, []types.ObjID{1}, []types.ObjID{}, []types.ObjID{3}},
		{3, "Wizard", wizardFlags, 2, []types.ObjID{1}, []types.ObjID{}, []types.ObjID{}},
	}

	txn := store.DirectTxn()
	if got := txn.MaxObject(); got != 3 {
		t.Fatalf("max object = #%d, want #3", got)
	}
	if got := len(store.All()); got != len(want) {
		t.Fatalf("store holds %d objects, want %d", got, len(want))
	}
	if got := store.Players(); !slices.Equal(got, []types.ObjID{3}) {
		t.Fatalf("players = %v, want [3]", got)
	}
	for _, w := range want {
		obj, ok := store.Get(w.id)
		if !ok {
			t.Fatalf("#%d is missing", w.id)
		}
		if obj.Name != w.name {
			t.Errorf("#%d name = %q, want %q", w.id, obj.Name, w.name)
		}
		if obj.Owner != 3 {
			t.Errorf("#%d owner = #%d, want #3", w.id, obj.Owner)
		}
		if obj.Flags != w.flags {
			t.Errorf("#%d flags = %#x, want %#x", w.id, obj.Flags, w.flags)
		}
		if obj.Location != w.location {
			t.Errorf("#%d location = #%d, want #%d", w.id, obj.Location, w.location)
		}
		if obj.VerbCount != 0 || obj.PropertyCount != 0 {
			t.Errorf("#%d has %d verbs and %d properties, want none", w.id, obj.VerbCount, obj.PropertyCount)
		}
		if got, _ := txn.Parents(w.id); !slices.Equal(got, w.parents) {
			t.Errorf("#%d parents = %v, want %v", w.id, got, w.parents)
		}
		if got, _ := txn.Children(w.id); !slices.Equal(got, w.children) {
			t.Errorf("#%d children = %v, want %v", w.id, got, w.children)
		}
		if got, _ := txn.Contents(w.id); !slices.Equal(got, w.contents) {
			t.Errorf("#%d contents = %v, want %v", w.id, got, w.contents)
		}
	}
}

// Each call builds a fresh database, so one session's writes never reach the
// next.
func TestNewMinimalStoreIsFreshPerCall(t *testing.T) {
	first, err := NewMinimalStore()
	if err != nil {
		t.Fatalf("NewMinimalStore: %v", err)
	}
	if _, _, err := runEval(t, first, `add_property(#0, "marker", 1, {player, "r"}); return 1;`); err != nil {
		t.Fatalf("eval on the first store: %v", err)
	}
	second, err := NewMinimalStore()
	if err != nil {
		t.Fatalf("NewMinimalStore: %v", err)
	}
	out, errOut, err := runEval(t, second, `return properties(#0);`)
	if err != nil {
		t.Fatalf("eval on the second store: %v\nstderr: %s", err, errOut)
	}
	if out != "=> {}\n" {
		t.Fatalf("second store's #0 properties printed %q, want \"=> {}\\n\"", out)
	}
}

// The minimal database is one a MOO program can run in: it evaluates as the
// wizard, who stands in the first room.
func TestNewMinimalStoreEvaluatesAsWizard(t *testing.T) {
	store, err := NewMinimalStore()
	if err != nil {
		t.Fatalf("NewMinimalStore: %v", err)
	}
	out, errOut, err := runEval(t, store, `{player, player.wizard, player.programmer, is_player(player), player.location, parent(#0), children(#1), #2.contents}`)
	if err != nil {
		t.Fatalf("EvalExpression: %v\nstderr: %s", err, errOut)
	}
	if want := "=> {#3, 1, 1, 1, #2, #1, {#0, #2, #3}, {#3}}\n"; out != want {
		t.Fatalf("minimal database printed %q, want %q", out, want)
	}
}
