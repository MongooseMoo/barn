package command

import (
	"testing"

	dbstore "barn/db/store"
	"barn/types"
)

func TestParseCommandForPlayerResolvesObjects(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 2, "player", types.ObjNothing, []types.ObjID{4})
	addCommandTestObject(t, store, 3, "room", types.ObjNothing, []types.ObjID{5})
	addCommandTestObject(t, store, 4, "key", 2, nil)
	addCommandTestObject(t, store, 5, "box", 3, nil)

	cmd := ParseCommandForPlayer(store, 2, 3, "put key in box")

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
