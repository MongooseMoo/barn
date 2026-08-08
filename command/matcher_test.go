package command

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestParsePlayerCommandResolvesObjects(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 2, "player", types.ObjNothing, []types.ObjID{4})
	addCommandTestObject(t, store, 3, "room", types.ObjNothing, []types.ObjID{5})
	addCommandTestObject(t, store, 4, "key", 2, nil)
	addCommandTestObject(t, store, 5, "box", 3, nil)

	cmd := ParsePlayerCommand(store, 2, 3, "put key in box")

	if cmd.Dobjstr != "key" {
		t.Fatalf("Dobjstr = %q, want key", cmd.Dobjstr)
	}
	if cmd.Dobj != 4 {
		t.Fatalf("Dobj = #%d, want #4", cmd.Dobj)
	}
	if cmd.Iobjstr != "box" {
		t.Fatalf("Iobjstr = %q, want box", cmd.Iobjstr)
	}
	if cmd.Iobj != 5 {
		t.Fatalf("Iobj = #%d, want #5", cmd.Iobj)
	}
}

func TestFindHuhVerbUsesLocationByDefault(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 2, "player", types.ObjNothing, nil)
	addCommandTestObject(t, store, 3, "room", types.ObjNothing, nil)
	addCommandTestVerb(t, store, 2, "huh")
	addCommandTestVerb(t, store, 3, "huh")

	match := FindHuhVerb(store, 2, 3, false)

	if match == nil {
		t.Fatalf("FindHuhVerb returned nil")
	}
	if match.This != 3 {
		t.Fatalf("huh This = #%d, want location #3", match.This)
	}
	if match.VerbLoc != 3 {
		t.Fatalf("huh VerbLoc = #%d, want #3", match.VerbLoc)
	}
}

func TestFindHuhVerbUsesPlayerWhenEnabled(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 2, "player", types.ObjNothing, nil)
	addCommandTestObject(t, store, 3, "room", types.ObjNothing, nil)
	addCommandTestVerb(t, store, 2, "huh")
	addCommandTestVerb(t, store, 3, "huh")

	match := FindHuhVerb(store, 2, 3, true)

	if match == nil {
		t.Fatalf("FindHuhVerb returned nil")
	}
	if match.This != 2 {
		t.Fatalf("huh This = #%d, want player #2", match.This)
	}
	if match.VerbLoc != 2 {
		t.Fatalf("huh VerbLoc = #%d, want #2", match.VerbLoc)
	}
}

func addCommandTestObject(t *testing.T, store *dbstore.Store, id types.ObjID, name string, location types.ObjID, contents []types.ObjID) {
	t.Helper()
	builder := dbstore.NewObjectBuilder(id)
	builder.SetName(name)
	builder.SetOwner(2)
	builder.SetLocation(location)
	builder.SetContents(contents)
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("add object #%d: %v", id, err)
	}
}

func addCommandTestVerb(t *testing.T, store *dbstore.Store, objID types.ObjID, name string) {
	t.Helper()
	_, err := store.AddVerb(objID, dbstore.NewVerb(
		name,
		[]string{name},
		2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{`return 1;`},
	))
	if err != types.E_NONE {
		t.Fatalf("add verb #%d:%s: %v", objID, name, err)
	}
}
