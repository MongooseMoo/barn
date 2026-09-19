package server

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func TestAdmissionCapOneForcedInputCannotBlockItsOwnLane(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
	var processor *InputProcessor
	flooded := make(chan struct{})
	registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), append(vm.Descriptors(), builtins.Descriptor{
		Name: "flood_input_test", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0}, Visibility: builtins.Hidden, Effect: builtins.Transactional, Capability: config.Core,
		Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
			for i := 0; i < 1024; i++ {
				processor.ForceInput(player, "eval return 1;", false)
			}
			close(flooded)
			return types.Ok(types.NewInt(0))
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	options := config.DefaultOptions()
	options.AdmissionLimit = 1
	rt := engine.NewRuntimeWithRegistry(store, options, registry)
	defer rt.Stop()
	processor = NewInputProcessor(store, rt)
	defer processor.Stop()
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	conn := cm.NewConnectionFromTransport(newRecordingTransport("forced-input"))
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
		t.Fatal(err)
	}
	processor.Start()
	done := make(chan struct{})
	processor.EnqueueInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "eval flood_input_test(); return 7;", Done: done})
	select {
	case <-flooded:
	case <-time.After(3 * time.Second):
		t.Fatal("forced input blocked on its own lane")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forcing activation did not finish")
	}
}

func TestInputStopCompletesAcceptedAndLaterEvents(t *testing.T) {
	rt := engine.NewRuntime(dbstore.NewStore())
	defer rt.Stop()
	processor := NewInputProcessor(dbstore.NewStore(), rt)
	queued := make(chan struct{})
	processor.EnqueueInput(command.InputEvent{Done: queued})
	processor.Stop()
	later := make(chan struct{})
	processor.EnqueueInput(command.InputEvent{Done: later})
	for _, done := range []chan struct{}{queued, later} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("stopped input retained a transport waiter")
		}
	}
}

func TestCheckpointPreventsExecutionBetweenTaskAndStoreCapture(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	rt := engine.NewRuntimeWithOptions(store, config.Options{AdmissionLimit: 1})
	defer rt.Stop()
	server := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0)}
	entered := make(chan struct{})
	release := make(chan struct{})
	checkpoint := make(chan error, 1)
	go func() {
		checkpoint <- server.checkpointWith(func(_ string, _ *dbstore.Store, _ []task.Snapshot, _ []task.Snapshot, _ []dbformat.ActiveConnection) error {
			close(entered)
			<-release
			return nil
		}, true)
	}()
	<-entered
	finished := make(chan engine.EvalOutcome, 1)
	go func() { finished <- rt.Eval(0, []string{"return 42;"}) }()
	deadline := time.Now().Add(time.Second)
	for rt.AdmissionStats().Queued == 0 && time.Now().Before(deadline) {
		select {
		case <-finished:
			close(release)
			<-checkpoint
			t.Fatal("VM ran after task capture but before store capture")
		default:
		}
		time.Sleep(time.Millisecond)
	}
	queued := rt.AdmissionStats().Queued
	close(release)
	if err := <-checkpoint; err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("checkpoint probe never reached admission: queued=%d", queued)
	}
	select {
	case got := <-finished:
		if got.Result.Val.Int() != 42 {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("checkpoint did not resume admission")
	}
}
