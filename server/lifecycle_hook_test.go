package server

import (
	"testing"

	dbstore "barn/db/store"
	runtime "barn/scheduler"
	"barn/types"
)

func TestCallServerStartedRunsHookBeforeReturning(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, dbstore.NewProperty("started", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define property: %v", errCode)
	}
	addTestVerb(store, system, "server_started", "#0.started = 1;")

	s := &Server{
		store:     store,
		scheduler: runtime.NewScheduler(store),
	}

	if err := s.callServerStarted(); err != nil {
		t.Fatalf("call server_started: %v", err)
	}

	value, errCode := store.PropertyValue(system, "started")
	if errCode != types.E_NONE {
		t.Fatalf("read property: %v", errCode)
	}
	started, ok := value.(types.IntValue)
	if !ok || started.Val != 1 {
		t.Fatalf("started = %v, want 1 before callServerStarted returns", value)
	}
}
