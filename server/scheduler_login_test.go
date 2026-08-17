package server

import (
	"testing"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
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

	rt := engine.NewRuntime(store)
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
	store.DirectTxn().DefineProperty(system, "created", dbstore.NewProperty(types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true))
	store.DirectTxn().DefineProperty(system, "connected", dbstore.NewProperty(types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true))
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	listener := addTestObject(t, store, 10, dbstore.FlagWizard)

	addTestVerb(store, listener, "user_created", "#0.created = args[1];")
	addTestVerb(store, listener, "user_connected", "#0.connected = args[1];")

	rt := engine.NewRuntime(store)
	s := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	s.SetConnectionManager(cm)
	conn := cm.NewConnectionFromTransport(stubTransport{})
	conn.SetListener(10, 7789, true)

	s.loginPlayer(conn, 2, true)

	createdVal, _ := store.DirectTxn().PropertyValue(system, "created")
	if (createdVal.Type() != types.TYPE_OBJ && createdVal.Type() != types.TYPE_ANON) || createdVal.Obj() != 2 {
		t.Fatalf("created hook value = %v, want #2", createdVal)
	}
	connectedVal, _ := store.DirectTxn().PropertyValue(system, "connected")
	if (connectedVal.Type() != types.TYPE_OBJ && connectedVal.Type() != types.TYPE_ANON) || connectedVal.Obj() != 2 {
		t.Fatalf("connected hook value = %v, want #2", connectedVal)
	}
}

func TestUserConnectedUsesServerInitiatedCallerFrame(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "connected_frame", dbstore.NewProperty(types.NewList(nil), 2,
		dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define connected_frame: %v", errCode)
	}
	addTestVerb(store, system, "user_connected", "#0.connected_frame = {this, player, caller, args, argstr};")

	rt := engine.NewRuntime(store)
	processor := NewInputProcessor(store, rt)
	processor.callUserHook(system, "user_connected", 2)

	got, errCode := store.DirectTxn().PropertyValue(system, "connected_frame")
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
	if errCode := store.DirectTxn().DefineProperty(system, "disconnected_frames", dbstore.NewProperty(types.NewList(nil), 2,
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

	rt := engine.NewRuntime(store)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)

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

	got, errCode := store.DirectTxn().PropertyValue(system, "disconnected_frames")
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

func TestCrossListenerReconnectDisassociatesPlayerBeforeOldHook(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	oldHandler := addTestObject(t, store, 10, dbstore.FlagWizard)
	newHandler := addTestObject(t, store, 11, dbstore.FlagWizard)
	for _, name := range []string{"old_disconnect_frames", "new_connected_frames", "hook_order"} {
		if errCode := store.DirectTxn().DefineProperty(system, name, dbstore.NewProperty(types.NewList(nil), 2,
			dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("define %s: %v", name, errCode)
		}
	}
	if _, errCode := store.AddVerb(oldHandler, dbstore.NewVerb("user_client_disconnected", []string{"user_client_disconnected"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			"connection_info_succeeds = 1;",
			"try",
			"  connection_info(args[1]);",
			"except (E_INVARG)",
			"  connection_info_succeeds = 0;",
			"endtry",
			"#0.old_disconnect_frames = {@#0.old_disconnect_frames, {this, player, caller, args, argstr, connection_info_succeeds}};",
			`#0.hook_order = {@#0.hook_order, "old_client"};`,
		})); errCode != types.E_NONE {
		t.Fatalf("add old disconnect hook: %v", errCode)
	}
	if _, errCode := store.AddVerb(newHandler, dbstore.NewVerb("user_connected", []string{"user_connected"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			"info = connection_info(args[1]);",
			`#0.new_connected_frames = {@#0.new_connected_frames, {this, player, caller, args, argstr, info["source_port"]}};`,
			`#0.hook_order = {@#0.hook_order, "new_connected"};`,
		})); errCode != types.E_NONE {
		t.Fatalf("add new connected hook: %v", errCode)
	}

	rt := engine.NewRuntime(store)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)

	oldConn := cm.NewConnectionFromTransport(stubTransport{})
	oldConn.SetListener(oldHandler, 7788, false)
	processor.loginPlayer(oldConn, player, false)
	newConn := cm.NewConnectionFromTransport(stubTransport{})
	newConn.SetListener(newHandler, 7789, false)
	processor.loginPlayer(newConn, player, false)

	oldFrames, errCode := store.DirectTxn().PropertyValue(system, "old_disconnect_frames")
	if errCode != types.E_NONE {
		t.Fatalf("read old_disconnect_frames: %v", errCode)
	}
	wantOldFrames := types.NewList([]types.Value{types.NewList([]types.Value{
		types.NewObj(oldHandler),
		types.NewObj(player),
		types.NewObj(types.ObjNothing),
		types.NewList([]types.Value{types.NewObj(player)}),
		types.NewStr(""),
		types.NewInt(0),
	})})
	if !oldFrames.Equal(wantOldFrames) {
		t.Fatalf("old disconnect frames = %s, want %s", oldFrames.String(), wantOldFrames.String())
	}

	newFrames, errCode := store.DirectTxn().PropertyValue(system, "new_connected_frames")
	if errCode != types.E_NONE {
		t.Fatalf("read new_connected_frames: %v", errCode)
	}
	wantNewFrames := types.NewList([]types.Value{types.NewList([]types.Value{
		types.NewObj(newHandler),
		types.NewObj(player),
		types.NewObj(types.ObjNothing),
		types.NewList([]types.Value{types.NewObj(player)}),
		types.NewStr(""),
		types.NewInt(7789),
	})})
	if !newFrames.Equal(wantNewFrames) {
		t.Fatalf("new connected frames = %s, want %s", newFrames.String(), wantNewFrames.String())
	}

	order, errCode := store.DirectTxn().PropertyValue(system, "hook_order")
	if errCode != types.E_NONE {
		t.Fatalf("read hook_order: %v", errCode)
	}
	wantOrder := types.NewList([]types.Value{types.NewStr("old_client"), types.NewStr("new_connected")})
	if !order.Equal(wantOrder) {
		t.Fatalf("hook order = %s, want %s", order.String(), wantOrder.String())
	}
	if got := cm.GetConnection(player); got != newConn {
		t.Fatalf("active connection = %v, want replacement %v", got, newConn)
	}
}

func TestUserConnectedResumesAfterNestedSuspendWithPendingFork(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	for _, name := range []string{"forked", "resumed", "continued"} {
		if errCode := store.DirectTxn().DefineProperty(system, name, dbstore.NewProperty(types.NewInt(0), 2,
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

	rt := engine.NewRuntime(store)
	processor := NewInputProcessor(store, rt)
	processor.callUserHook(system, "user_connected", 2)
	for range 4 {
		if rt.ProcessReadyTasks() == 0 {
			break
		}
	}

	for _, name := range []string{"forked", "resumed", "continued"} {
		value, errCode := store.DirectTxn().PropertyValue(system, name)
		if errCode != types.E_NONE {
			t.Fatalf("read %s: %v", name, errCode)
		}
		if value.Type() != types.TYPE_INT || value.Int() != 1 {
			t.Fatalf("%s = %v, want 1", name, value)
		}
	}
}
