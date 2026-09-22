package server

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
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

func TestRuntimeTickAdmitsBackgroundWorkWithQueuedInput(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()
	program, diagnostics := rt.Registry().Compiler().CompileMOO([]string{"return 1;"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	id := rt.CreateBackgroundTask(types.ObjNothing, program, 0)
	done := make(chan struct{})
	processor.inputQueue <- command.InputEvent{ConnID: 42, Player: -42, Done: done}
	runtimeDone := processor.processRuntimeTick()
	if runtimeDone == nil {
		t.Fatal("queued input suppressed background admission")
	}
	select {
	case <-runtimeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("background dispatch did not finish")
	}
	if state := rt.GetTask(id).GetState(); state != task.TaskCompleted {
		t.Fatalf("background task state = %v", state)
	}
	<-done
}

func TestRuntimeWakeRunsEarlierArrivalAndTimedSuspension(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()
	processor.Start()
	queue := func(id int64, code string, delay time.Duration) *task.Task {
		program, diagnostics := rt.Registry().Compiler().CompileMOO([]string{code})
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		tk := task.NewTaskFull(id, types.ObjNothing, program, 1000, 5)
		tk.StartTime = time.Now().Add(delay)
		tk.Done = make(chan struct{})
		rt.QueueTask(tk)
		return tk
	}
	later := queue(90101, "return 1;", time.Hour)
	defer later.Kill()
	// Let the loop install the distant timer before an earlier arrival.
	time.Sleep(20 * time.Millisecond)
	immediate := queue(90102, "suspend(0.03); return 7;", 0)
	select {
	case <-immediate.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("arrival or timed resumption lost its wakeup")
	}
	if immediate.Result.Val.Int() != 7 || immediate.GetState() != task.TaskCompleted {
		t.Fatalf("timed task result = %v, state = %v", immediate.Result, immediate.GetState())
	}
	if later.GetState() != task.TaskQueued {
		t.Fatal("future task ran early")
	}
}

func TestRuntimeWakeExternalCompletionAndLateCompletionAfterStop(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	program, diagnostics := rt.Registry().Compiler().CompileMOO([]string{"suspend(); return 9;"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	tk := task.NewTaskFull(90201, types.ObjNothing, program, 1000, 5)
	tk.Done = make(chan struct{})
	rt.QueueTask(tk)
	processor.Start()
	defer processor.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for tk.GetState() != task.TaskSuspended && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !tk.CompleteExec(types.NewInt(0)) {
		t.Fatal("helper completion failed")
	}
	select {
	case <-tk.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("external completion did not wake the dispatcher")
	}
	if tk.Result.Val.Int() != 9 {
		t.Fatalf("result = %v", tk.Result)
	}
	processor.Stop()
	// A subprocess callback may outlive its listener. No closed-channel panic
	// or blocked sender is permitted, even when many notifications coalesce.
	for range 1000 {
		tk.SuspendIndefinite()
		if !tk.CompleteExec(types.NewInt(0)) {
			t.Fatal("late completion failed")
		}
	}
}
