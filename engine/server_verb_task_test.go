package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func addServerVerbTestObject(t *testing.T, store *dbstore.Store, id types.ObjID, flags dbstore.ObjectFlags) {
	t.Helper()
	builder := dbstore.NewObjectBuilder(id)
	builder.SetOwner(2)
	builder.SetName("test")
	builder.SetFlags(flags)
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("add object #%d: %v", id, err)
	}
}

func TestRunServerVerbTaskRunsBeforeReturning(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(0, "started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define property: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb("server_started", []string{"server_started"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"#0.started = 1;"}))

	rt := NewRuntime(store)
	if _, err := rt.RunServerVerbTask(0, "server_started", nil, 0); err != nil {
		t.Fatalf("run server verb task: %v", err)
	}

	value, errCode := store.DirectTxn().PropertyValue(0, "started")
	if errCode != types.E_NONE {
		t.Fatalf("read property: %v", errCode)
	}
	if value.Type() != types.TYPE_INT || value.Int() != 1 {
		t.Fatalf("started = %v, want 1 before RunServerVerbTask returns", value)
	}
}

func TestCreateLoginHookTaskUsesServerOriginCaller(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(0, "login_frame", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define login_frame: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb("do_login_command", []string{"do_login_command"}, 0,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"#0.login_frame = {this, player, caller, args, argstr};", "return 0;"}))

	rt := NewRuntime(store)
	connectionPlayer := types.ObjID(-7)
	if _, err := rt.CreateLoginHookTask(0, "do_login_command", nil, connectionPlayer, "", nil, nil); err != nil {
		t.Fatalf("CreateLoginHookTask: %v", err)
	}

	got, errCode := store.DirectTxn().PropertyValue(0, "login_frame")
	if errCode != types.E_NONE {
		t.Fatalf("read login_frame: %v", errCode)
	}
	want := types.NewList([]types.Value{
		types.NewObj(0),
		types.NewObj(connectionPlayer),
		types.NewObj(types.ObjNothing),
		types.NewList(nil),
		types.NewStr(""),
	})
	if !got.Equal(want) {
		t.Fatalf("login frame = %s, want %s", got.String(), want.String())
	}
}
