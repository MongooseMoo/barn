package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestCallersRedactsAnonymousThisFromUnrelatedViewer(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anon, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 1, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}

	taskValue := task.NewState(1, 2, 1000, 1)
	taskValue.PushFrame(types.ActivationFrame{
		This:       anon,
		ThisValue:  types.NewAnon(anon),
		Programmer: 1,
		Verb:       "entry",
		VerbLoc:    0,
		Player:     2,
	})
	taskValue.PushFrame(types.ActivationFrame{
		This:       0,
		ThisValue:  types.NewObj(0),
		Programmer: 2,
		Verb:       "probe",
		VerbLoc:    0,
		Player:     2,
	})

	ctx := newTestExecution()
	ctx.Store = store
	ctx.Task = taskValue
	ctx.Programmer = 2
	ctx.Player = 2
	ctx.Verb = "probe"

	result := admitTestBuiltin("callers", ctx, nil)
	if result.IsError() {
		t.Fatalf("callers failed: %v", result.Error)
	}
	thisValue := result.Val.Get(1).Get(1)
	if thisValue.Type() != types.TYPE_ANON {
		t.Fatalf("redacted this type = %v, want ANON", thisValue.Type())
	}
	if store.DirectTxn().Valid(thisValue.ID()) {
		t.Fatalf("redacted this = %s is still valid", thisValue.String())
	}
}
