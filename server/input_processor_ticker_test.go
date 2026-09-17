package server

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func TestInputDispatchContinuesDuringBackgroundTask(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), append(vm.Descriptors(), builtins.Descriptor{
		Name: "wait_for_input_test", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0},
		Visibility: builtins.Hidden, Effect: builtins.Transactional, Capability: config.Core,
		Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
			started <- struct{}{}
			<-release
			return types.Ok(types.NewInt(0))
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	rt := engine.NewRuntimeWithRegistry(store, config.DefaultOptions(), registry)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()
	defer close(release)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
		t.Fatal(err)
	}
	if got := rt.EvalCommandOutput(player, `fork (0) wait_for_input_test(); endfork; fork (0) wait_for_input_test(); endfork; return 1;`); got != "{1, 1}" {
		t.Fatalf("queue background work: %s", got)
	}
	processor.Start()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("background task did not start")
	}
	done := make(chan struct{})
	processor.EnqueueInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "eval return 42;", Done: done})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background task blocked foreground input dispatch")
	}
	if got := transport.writtenLines(); len(got) != 1 || got[0] != "{1, 42}" {
		t.Fatalf("foreground response = %q", got)
	}
	select {
	case <-started:
		t.Fatal("overlapping scheduler passes started a second background task")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRuntimeTickRoutesDisconnectThroughConnectionLane(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()

	const connID = int64(42)
	firstDone := make(chan struct{})
	processor.dispatch(command.InputEvent{
		ConnID: connID,
		Player: types.ObjID(-connID),
		Done:   firstDone,
	})
	<-firstDone

	processor.workersMu.Lock()
	_, laneExists := processor.workers[connID]
	processor.workersMu.Unlock()
	if !laneExists {
		t.Fatal("connection lane was not created")
	}

	disconnectDone := make(chan struct{})
	processor.inputQueue <- command.InputEvent{
		ConnID:       connID,
		IsDisconnect: true,
		Done:         disconnectDone,
	}
	processor.processRuntimeTick()
	<-disconnectDone

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		processor.workersMu.Lock()
		_, laneExists = processor.workers[connID]
		processor.workersMu.Unlock()
		if !laneExists {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("ticker-processed disconnect left the connection lane registered")
}
