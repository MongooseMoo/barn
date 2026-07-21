package server

import (
	"testing"

	"barn/command"
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
	store.DefineProperty(system, "created", dbstore.NewProperty(types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true))
	store.DefineProperty(system, "connected", dbstore.NewProperty(types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true))
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
	if (createdVal.Type() != types.TYPE_OBJ && createdVal.Type() != types.TYPE_ANON) || createdVal.Obj() != 2 {
		t.Fatalf("created hook value = %v, want #2", createdVal)
	}
	connectedVal, _ := store.PropertyValue(system, "connected")
	if (connectedVal.Type() != types.TYPE_OBJ && connectedVal.Type() != types.TYPE_ANON) || connectedVal.Obj() != 2 {
		t.Fatalf("connected hook value = %v, want #2", connectedVal)
	}
}

func TestUserConnectedUsesServerInitiatedCallerFrame(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, "connected_frame", dbstore.NewProperty(types.NewList(nil), 2,
		dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define connected_frame: %v", errCode)
	}
	addTestVerb(store, system, "user_connected", "#0.connected_frame = {this, player, caller, args, argstr};")

	rt := runtime.NewScheduler(store)
	processor := NewInputProcessor(store, rt)
	processor.callUserHook(system, "user_connected", 2)

	got, errCode := store.PropertyValue(system, "connected_frame")
	if errCode != types.E_NONE {
		t.Fatalf("read connected_frame: %v", errCode)
	}
	want := types.NewList([]types.Value{
		types.NewObj(system),
		types.NewObj(2),
		types.NewObj(types.ObjNothing),
		types.NewList([]types.Value{types.NewObj(2)}),
		types.NewStr(""),
	})
	if !got.Equal(want) {
		t.Fatalf("user_connected frame = %s, want %s", got.String(), want.String())
	}
}

func TestUserClientDisconnectedCannotResolveUnrelatedConnection(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	disconnectedPlayer := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	otherPlayer := addTestObject(t, store, 3, dbstore.FlagUser)
	if errCode := store.DefineProperty(system, "disconnected_frames", dbstore.NewProperty(types.NewList(nil), 2,
		dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define disconnected_frames: %v", errCode)
	}
	if _, errCode := store.AddVerb(system, dbstore.NewVerb("user_client_disconnected", []string{"user_client_disconnected"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			"connection_info_succeeds = 1;",
			"try",
			"  connection_info(args[1]);",
			"except (E_INVARG)",
			"  connection_info_succeeds = 0;",
			"endtry",
			"#0.disconnected_frames = {@#0.disconnected_frames, {this, player, caller, args, argstr, connection_info_succeeds}};",
		})); errCode != types.E_NONE {
		t.Fatalf("add user_client_disconnected: %v", errCode)
	}

	rt := runtime.NewScheduler(store)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	rt.Registry().SetConnectionManager(cm)

	disconnectedConn := cm.NewConnectionFromTransport(stubTransport{})
	if err := cm.SwitchPlayer(types.ObjID(-disconnectedConn.ID), disconnectedPlayer); err != nil {
		t.Fatalf("connect disconnected player: %v", err)
	}
	otherConn := cm.NewConnectionFromTransport(stubTransport{})
	if err := cm.SwitchPlayer(types.ObjID(-otherConn.ID), otherPlayer); err != nil {
		t.Fatalf("connect other player: %v", err)
	}

	processor.processDisconnect(command.InputEvent{ConnID: disconnectedConn.ID})
	if conn := cm.GetConnection(disconnectedPlayer); conn != nil {
		t.Fatalf("disconnected player still resolves to connection %v", conn)
	}

	got, errCode := store.PropertyValue(system, "disconnected_frames")
	if errCode != types.E_NONE {
		t.Fatalf("read disconnected_frames: %v", errCode)
	}
	want := types.NewList([]types.Value{types.NewList([]types.Value{
		types.NewObj(system),
		types.NewObj(disconnectedPlayer),
		types.NewObj(types.ObjNothing),
		types.NewList([]types.Value{types.NewObj(disconnectedPlayer)}),
		types.NewStr(""),
		types.NewInt(0),
	})})
	if !got.Equal(want) {
		t.Fatalf("user_client_disconnected frames = %s, want %s", got.String(), want.String())
	}
}

func TestUserConnectedResumesAfterNestedSuspendWithPendingFork(t *testing.T) {
	resetTaskManager()
	t.Cleanup(resetTaskManager)

	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	for _, name := range []string{"forked", "resumed", "continued"} {
		if errCode := store.DefineProperty(system, name, dbstore.NewProperty(types.NewInt(0), 2,
			dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("define %s: %v", name, errCode)
		}
	}

	addTestVerb(store, system, "yield_once",
		"suspend(0);",
		"#0.resumed = 1;",
	)
	addTestVerb(store, system, "user_connected",
		"fork (0)",
		"  #0.forked = 1;",
		"endfork",
		"#0:yield_once();",
		"#0.continued = 1;",
	)

	rt := runtime.NewScheduler(store)
	processor := NewInputProcessor(store, rt)
	processor.callUserHook(system, "user_connected", 2)
	for range 4 {
		if rt.ProcessReadyTasks() == 0 {
			break
		}
	}

	for _, name := range []string{"forked", "resumed", "continued"} {
		value, errCode := store.PropertyValue(system, name)
		if errCode != types.E_NONE {
			t.Fatalf("read %s: %v", name, errCode)
		}
		if value.Type() != types.TYPE_INT || value.Int() != 1 {
			t.Fatalf("%s = %v, want 1", name, value)
		}
	}
}
