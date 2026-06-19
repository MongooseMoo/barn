package server

import (
	"testing"

	dbstore "barn/db/store"
	"barn/types"
)

func addTestObject(t *testing.T, store *dbstore.Store, id types.ObjID, flags dbstore.ObjectFlags) *dbstore.Object {
	t.Helper()
	obj := dbstore.NewObject(id, 2)
	obj.Name = "test"
	obj.Flags = flags
	if err := store.Add(obj); err != nil {
		t.Fatalf("add object #%d: %v", id, err)
	}
	return obj
}

func addTestVerb(obj *dbstore.Object, name string, code ...string) {
	verb := &dbstore.Verb{
		Name:  name,
		Names: []string{name},
		Owner: 2,
		Perms: dbstore.VerbRead | dbstore.VerbExecute,
		ArgSpec: dbstore.VerbArgs{
			This: "this",
			Prep: "none",
			That: "this",
		},
		Code: code,
	}
	obj.Verbs[name] = verb
	obj.VerbList = append(obj.VerbList, verb)
}

func TestDoLoginCommandDispatchesOnListenerWithArgstr(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	addTestObject(t, store, 3, dbstore.FlagUser)
	listener := addTestObject(t, store, 10, dbstore.FlagWizard)

	addTestVerb(system, "do_login_command", "return #3;")
	addTestVerb(listener, "do_login_command",
		`if (length(args) == 2 && args[2] == "two words" && argstr == "connect \"two words\"")`,
		"  return #2;",
		"else",
		"  return #3;",
		"endif",
	)

	s := NewScheduler(store)
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
	createdProp := dbstore.NewProperty("created", types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, false)
	system.Properties["created"] = &createdProp
	connectedProp := dbstore.NewProperty("connected", types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, false)
	system.Properties["connected"] = &connectedProp
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	listener := addTestObject(t, store, 10, dbstore.FlagWizard)

	addTestVerb(listener, "user_created", "#0.created = args[1];")
	addTestVerb(listener, "user_connected", "#0.connected = args[1];")

	s := NewScheduler(store)
	cm := NewConnectionManager(nil, 7777)
	s.SetConnectionManager(cm)
	conn := cm.NewConnectionFromTransport(stubTransport{})
	conn.SetListener(10, 7789, true)

	s.loginPlayer(conn, 2, true)

	created, ok := system.Properties["created"].View().Value.(types.ObjValue)
	if !ok || created.ID() != 2 {
		t.Fatalf("created hook value = %v, want #2", system.Properties["created"].View().Value)
	}
	connected, ok := system.Properties["connected"].View().Value.(types.ObjValue)
	if !ok || connected.ID() != 2 {
		t.Fatalf("connected hook value = %v, want #2", system.Properties["connected"].View().Value)
	}
}
