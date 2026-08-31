package server

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

func TestUnquoteInBandInput(t *testing.T) {
	if got := unquoteInBandInput(`#$"hello quoted world`); got != "hello quoted world" {
		t.Fatalf("unquoted input = %q", got)
	}
	if got := unquoteInBandInput("ordinary"); got != "ordinary" {
		t.Fatalf("ordinary input = %q", got)
	}
}

func TestParseProgramTargetResolvesSystemObjectProperty(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	target := addTestObject(t, store, 3, 0)
	addTestVerb(store, target, "progme")
	if errCode := store.DirectTxn().DefineProperty(0, "target", dbstore.NewProperty(types.NewObj(target), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty() = %v", errCode)
	}
	runtime := engine.NewRuntime(store)
	t.Cleanup(runtime.Stop)
	processor := NewInputProcessor(store, runtime)
	gotTarget, gotVerb, ok := processor.parseProgramTarget(2, 0, "$target:progme")
	if !ok || gotTarget != target || gotVerb != "progme" {
		t.Fatalf("parseProgramTarget() = (%v, %q, %v)", gotTarget, gotVerb, ok)
	}
}
