package server

import (
	"errors"
	"path/filepath"
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

// checkpointProbe retains the real writer/publication lifecycle and holds only
// the specified phase, so runtime admission probes exercise the production path.
type checkpointProbe struct {
	*dbformat.Checkpoint
	beforeSerialize func()
	beforePublish   func() error
}

func (p *checkpointProbe) Serialize(store *dbstore.Store, queued, suspended []task.Snapshot, connections []dbformat.ActiveConnection) error {
	if p.beforeSerialize != nil {
		p.beforeSerialize()
	}
	return p.Checkpoint.Serialize(store, queued, suspended, connections)
}

func (p *checkpointProbe) Publish() error {
	if p.beforePublish != nil {
		if err := p.beforePublish(); err != nil {
			return err
		}
	}
	return p.Checkpoint.Publish()
}

func checkpointProbeFactory(beforeSerialize func(), beforePublish func() error) checkpointFactory {
	return func(path string) (checkpointStage, error) {
		stage, err := dbformat.BeginCheckpoint(path)
		if err != nil {
			return nil, err
		}
		return &checkpointProbe{Checkpoint: stage, beforeSerialize: beforeSerialize, beforePublish: beforePublish}, nil
	}
}

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
				processor.ForceInput(player, "eval return 1;", false, nil)
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
	server := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0), dbPath: filepath.Join(t.TempDir(), "capture.db")}
	entered := make(chan struct{})
	release := make(chan struct{})
	checkpoint := make(chan error, 1)
	go func() {
		checkpoint <- server.checkpointWith(checkpointProbeFactory(func() {
			close(entered)
			<-release
		}, nil), true)
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

func TestCheckpointPublicationAllowsRuntimeCommit(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	if code := store.DirectTxn().DefineProperty(0, "committed", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	rt := engine.NewRuntimeWithOptions(store, config.Options{AdmissionLimit: 1})
	defer rt.Stop()
	server := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0), dbPath: filepath.Join(t.TempDir(), "publication.db")}
	entered := make(chan struct{})
	release := make(chan struct{})
	checkpoint := make(chan error, 1)
	go func() {
		checkpoint <- server.checkpointWith(checkpointProbeFactory(nil, func() error {
			close(entered)
			<-release
			return nil
		}), true)
	}()
	<-entered
	finished := make(chan engine.EvalOutcome, 1)
	go func() { finished <- rt.Eval(0, []string{"#0.committed = 42; return #0.committed;"}) }()
	select {
	case got := <-finished:
		if got.Result.Val.Int() != 42 {
			t.Error(got)
		}
		value, code := store.DirectTxn().PropertyValue(0, "committed")
		if code != types.E_NONE || value.Int() != 42 {
			t.Errorf("runtime write was not committed: value=%v code=%v", value, code)
		}
	case <-time.After(time.Second):
		t.Error("runtime command could not commit during checkpoint publication")
	}
	close(release)
	if err := <-checkpoint; err != nil {
		t.Fatal(err)
	}
	reloaded, err := dbformat.LoadDatabase(server.dbPath + ".new")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	value, code := restored.DirectTxn().PropertyValue(0, "committed")
	if code != types.E_NONE || value.Int() != 0 {
		t.Fatalf("checkpoint included post-capture commit: value=%v code=%v", value, code)
	}
}

func TestCheckpointPublicationFailureRunsFailureHookAfterResume(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagWizard|dbstore.FlagProgrammer)
	if code := store.DirectTxn().DefineProperty(0, "finished", dbstore.NewProperty(types.NewInt(-1), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	addTestVerb(store, 0, "checkpoint_finished", "#0.finished = args[1];")
	rt := engine.NewRuntimeWithOptions(store, config.Options{AdmissionLimit: 1})
	defer rt.Stop()
	server := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0), dbPath: filepath.Join(t.TempDir(), "failure.db")}
	publishErr := errors.New("publication failed after admission resumed")
	err := server.checkpointWith(checkpointProbeFactory(nil, func() error {
		finished := make(chan engine.EvalOutcome, 1)
		go func() { finished <- rt.Eval(0, []string{"return 42;"}) }()
		select {
		case outcome := <-finished:
			if outcome.Result.Val.Int() != 42 {
				t.Error(outcome)
			}
		case <-time.After(time.Second):
			t.Error("publication failure probe did not resume runtime admission")
		}
		return publishErr
	}), true)
	if !errors.Is(err, publishErr) {
		t.Fatalf("checkpoint error = %v, want %v", err, publishErr)
	}
	value, code := store.DirectTxn().PropertyValue(0, "finished")
	if code != types.E_NONE || value.Int() != 0 {
		t.Fatalf("failure hook value=%v code=%v", value, code)
	}
}
