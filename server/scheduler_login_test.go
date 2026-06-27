package server

import (
	"testing"

	dbstore "barn/db/store"
	runtime "barn/scheduler"
	"barn/types"
)

func addTestObject(t *testing.T, store *dbstore.Store, id types.ObjID, flags dbstore.ObjectFlags) types.ObjID {
	t.Helper()
	b := dbstore.NewObjectBuilder(id)
	b.SetOwner(2)
	b.SetName("test")
	b.SetFlags(flags)
	if err := store.Add(b.Build()); err != nil {
		t.Fatalf("add object #%d: %v", id, err)
	}
	return id
}

func addTestVerb(store *dbstore.Store, objID types.ObjID, name string, code ...string) {
	store.AddVerb(objID, dbstore.NewVerb(name, []string{name}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		code))
}

func TestDoLoginCommandDispatchesOnListenerWithArgstr(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	addTestObject(t, store, 3, dbstore.FlagUser)
	listener := addTestObject(t, store, 10, dbstore.FlagWizard)

	addTestVerb(store, system, "do_login_command", "return #3;")
	addTestVerb(store, listener, "do_login_command",
		`if (length(args) == 2 && args[2] == "two words" && argstr == "connect \"two words\"")`,
		"  return #2;",
		"else",
		"  return #3;",
		"endif",
	)

	rt := runtime.NewScheduler(store)
	s := NewInputProcessor(store, rt)
	conn := NewConnection(2, stubTransport{})
	conn.SetListener(10, 7789, true)

	player, err := s.callDoLoginCommand(conn, `connect "two words"`)
	if err != nil {
		t.Fatalf("callDoLoginCommand: %v", err)
	}
	if player != 2 {
		t.Fatalf("login returned #%d, want #2 from listener handler", player)
	}
}

func TestLoginPlayerRunsListenerCreatedAndConnectedHooks(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	store.DefineProperty(system, dbstore.NewProperty("created", types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true))
	store.DefineProperty(system, dbstore.NewProperty("connected", types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true))
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	listener := addTestObject(t, store, 10, dbstore.FlagWizard)

	addTestVerb(store, listener, "user_created", "#0.created = args[1];")
	addTestVerb(store, listener, "user_connected", "#0.connected = args[1];")

	rt := runtime.NewScheduler(store)
	s := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	s.SetConnectionManager(cm)
	conn := cm.NewConnectionFromTransport(stubTransport{})
	conn.SetListener(10, 7789, true)

	s.loginPlayer(conn, 2, true)

	createdVal, _ := store.PropertyValue(system, "created")
	created, ok := createdVal.(types.ObjValue)
	if !ok || created.ID() != 2 {
		t.Fatalf("created hook value = %v, want #2", createdVal)
	}
	connectedVal, _ := store.PropertyValue(system, "connected")
	connected, ok := connectedVal.(types.ObjValue)
	if !ok || connected.ID() != 2 {
		t.Fatalf("connected hook value = %v, want #2", connectedVal)
	}
}
