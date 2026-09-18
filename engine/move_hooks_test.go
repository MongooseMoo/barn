package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func addMoveHookTestVerb(t *testing.T, store *dbstore.Store, object types.ObjID, name string, code ...string) {
	t.Helper()
	if _, errCode := store.AddVerb(object, dbstore.NewVerb(
		name,
		[]string{name},
		2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		code,
	)); errCode != types.E_NONE {
		t.Fatalf("add %s on #%d: %s", name, object, errCode)
	}
}

func TestMoveHooksResumeOnOwningVM(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	const (
		old         types.ObjID = 10
		destination types.ObjID = 11
		what        types.ObjID = 12
		driver      types.ObjID = 13
	)
	for _, object := range []types.ObjID{old, destination, what, driver} {
		addServerVerbTestObject(t, store, object, 0)
	}
	if errCode := store.DirectTxn().MoveObject(what, old, 0); errCode != types.E_NONE {
		t.Fatalf("initial move: %s", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(what, "move_trace", dbstore.NewProperty(
		types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true,
	)); errCode != types.E_NONE {
		t.Fatalf("define move_trace: %s", errCode)
	}
	addMoveHookTestVerb(t, store, driver, "run_move", "move(args[1], args[2]);")
	addMoveHookTestVerb(t, store, old, "exitfunc",
		`args[1].move_trace = {@args[1].move_trace, "exit-before"};`,
		"suspend(0);",
		`args[1].move_trace = {@args[1].move_trace, "exit-after"};`,
	)
	addMoveHookTestVerb(t, store, destination, "enterfunc",
		`args[1].move_trace = {@args[1].move_trace, "enter-before"};`,
		"suspend(0);",
		`args[1].move_trace = {@args[1].move_trace, "enter-after"};`,
	)

	runtime := NewRuntime(store)
	if line := runtime.EvalCommandOutput(2, "#13:run_move(#12, #11); return 1;"); line != "{1, 1}" {
		t.Fatalf("eval result = %s, want {1, 1}", line)
	}
	got, errCode := store.DirectTxn().PropertyValue(what, "move_trace")
	if errCode != types.E_NONE {
		t.Fatalf("read move_trace: %s", errCode)
	}
	want := types.NewList([]types.Value{
		types.NewStr("exit-before"),
		types.NewStr("exit-after"),
		types.NewStr("enter-before"),
		types.NewStr("enter-after"),
	})
	if !got.Equal(want) {
		t.Fatalf("move_trace = %s, want %s", got.String(), want.String())
	}
}
