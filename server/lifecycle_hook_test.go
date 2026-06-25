package server

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"barn/builtins"
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

func TestServerStartedCanSeeBoundListenersBeforeAccepting(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, dbstore.NewProperty("listener_count", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define property: %v", errCode)
	}
	addTestVerb(store, system, "server_started", "#0.listener_count = length(listeners());")

	cm := NewConnectionManager(0)
	if err := cm.BindListeners([]builtins.ListenerSpec{{
		Protocol:  builtins.ListenerProtocolTCP,
		Port:      0,
		Interface: "127.0.0.1",
	}}); err != nil {
		t.Fatalf("bind listeners: %v", err)
	}
	defer cm.CloseListeners()

	s := &Server{
		store:     store,
		scheduler: runtime.NewScheduler(store),
	}
	s.scheduler.Registry().SetConnectionManager(cm)

	if err := s.callServerStarted(); err != nil {
		t.Fatalf("call server_started: %v", err)
	}

	value, errCode := store.PropertyValue(system, "listener_count")
	if errCode != types.E_NONE {
		t.Fatalf("read property: %v", errCode)
	}
	listenerCount, ok := value.(types.IntValue)
	if !ok || listenerCount.Val != 1 {
		t.Fatalf("listener_count = %v, want 1", value)
	}
}

func TestStartRollsBackBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy local port: %v", err)
	}
	defer occupied.Close()
	port := int64(occupied.Addr().(*net.TCPAddr).Port)

	store := dbstore.NewStore()
	scheduler := runtime.NewScheduler(store)
	input := NewInputProcessor(store, scheduler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{
		store:          store,
		scheduler:      scheduler,
		input:          input,
		connManager:    NewConnectionManager(int(port)),
		listenerSpecs:  []builtins.ListenerSpec{{Protocol: builtins.ListenerProtocolTCP, Port: port, Interface: "127.0.0.1"}},
		checkpointChan: make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}

	if err := s.Start(); err == nil {
		t.Fatalf("Start succeeded on occupied port")
	}

	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running {
		t.Fatalf("server remained running after bind failure")
	}
	if infos := s.connManager.ListenerInfos(); len(infos) != 0 {
		t.Fatalf("listeners after bind failure = %+v, want none", infos)
	}
	select {
	case <-s.ctx.Done():
	default:
		t.Fatalf("server context was not canceled after bind failure")
	}
	select {
	case <-input.ctx.Done():
	default:
		t.Fatalf("input processor was not stopped after bind failure")
	}
}

func TestShutdownStartedRunsBeforeListenersClose(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, dbstore.NewProperty("listener_count", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define listener_count property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, dbstore.NewProperty("shutdown_message", types.NewStr(""), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define shutdown_message property: %v", errCode)
	}
	addTestVerb(store, system, "shutdown_started",
		"#0.listener_count = length(listeners());",
		"#0.shutdown_message = args[1];",
	)

	cm := NewConnectionManager(7777)
	listener := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}
	if _, err := cm.registerListener(listener, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   0,
	}, true, nil); err != nil {
		t.Fatalf("register listener: %v", err)
	}
	defer cm.CloseListeners()

	scheduler := runtime.NewScheduler(store)
	scheduler.Registry().SetConnectionManager(cm)
	s := &Server{
		store:           store,
		scheduler:       scheduler,
		input:           NewInputProcessor(store, scheduler),
		connManager:     cm,
		shutdownMessage: "Maintenance",
	}

	if err := s.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	value, errCode := store.PropertyValue(system, "listener_count")
	if errCode != types.E_NONE {
		t.Fatalf("read listener_count: %v", errCode)
	}
	listenerCount, ok := value.(types.IntValue)
	if !ok || listenerCount.Val != 1 {
		t.Fatalf("listener_count = %v, want 1", value)
	}

	value, errCode = store.PropertyValue(system, "shutdown_message")
	if errCode != types.E_NONE {
		t.Fatalf("read shutdown_message: %v", errCode)
	}
	shutdownMessage, ok := value.(types.StrValue)
	if !ok || shutdownMessage.Value() != "Maintenance" {
		t.Fatalf("shutdown_message = %v, want Maintenance", value)
	}

	if !listener.closed {
		t.Fatalf("listener was not closed")
	}
	if infos := cm.ListenerInfos(); len(infos) != 0 {
		t.Fatalf("listeners after shutdown = %+v, want none", infos)
	}
}

func TestShutdownClosesActiveConnectionsWithMessage(t *testing.T) {
	store := dbstore.NewStore()
	scheduler := runtime.NewScheduler(store)
	cm := NewConnectionManager(7777)
	transport := newRecordingTransport("client")
	cm.NewConnectionFromTransport(transport)

	s := &Server{
		store:           store,
		scheduler:       scheduler,
		input:           NewInputProcessor(store, scheduler),
		connManager:     cm,
		shutdownMessage: "Maintenance",
	}

	if err := s.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	lines := transport.writtenLines()
	if len(lines) != 1 || lines[0] != "*** Shutting down: Maintenance ***" {
		t.Fatalf("shutdown lines = %+v, want shutdown banner", lines)
	}
	if !transport.isClosed() {
		t.Fatalf("transport was not closed")
	}
}

func TestShutdownWaitsForDisconnectCleanup(t *testing.T) {
	store := dbstore.NewStore()
	scheduler := runtime.NewScheduler(store)
	cm := NewConnectionManager(7777)
	input := NewInputProcessor(store, scheduler)
	input.SetConnectionManager(cm)
	input.Start()

	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	cm.connectionWG.Add(1)
	go cm.handleConnection(conn)

	s := &Server{
		store:           store,
		scheduler:       scheduler,
		input:           input,
		connManager:     cm,
		shutdownMessage: "Maintenance",
	}

	if err := s.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if got := cm.getConnectionByConnID(conn.ID); got != nil {
		t.Fatalf("connection still registered after shutdown: %+v", got)
	}
	if conn := cm.GetConnection(types.ObjID(-conn.ID)); conn != nil {
		t.Fatalf("negative player mapping still registered after shutdown")
	}
}

func TestShutdownWaitsForBackgroundGoroutines(t *testing.T) {
	store := dbstore.NewStore()
	scheduler := runtime.NewScheduler(store)
	s := &Server{
		store:           store,
		scheduler:       scheduler,
		input:           NewInputProcessor(store, scheduler),
		connManager:     NewConnectionManager(7777),
		shutdownMessage: "Maintenance",
	}

	release := make(chan struct{})
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		<-release
	}()

	done := make(chan error, 1)
	go func() {
		done <- s.shutdown()
	}()

	select {
	case err := <-done:
		t.Fatalf("shutdown returned before background goroutine completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestShutdownFinalCheckpointRunsHooksBeforeSchedulerStops(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, dbstore.NewProperty("checkpoint_started", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, dbstore.NewProperty("checkpoint_finished", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "#0.checkpoint_started = 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = args[1];")

	scheduler := runtime.NewScheduler(store)
	s := &Server{
		store:              store,
		scheduler:          scheduler,
		input:              NewInputProcessor(store, scheduler),
		connManager:        NewConnectionManager(7777),
		dbPath:             filepath.Join(t.TempDir(), "shutdown.db"),
		checkpointInterval: time.Second,
		shutdownMessage:    "Maintenance",
	}

	if err := s.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	started, errCode := store.PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started: %v", errCode)
	}
	if val, ok := started.(types.IntValue); !ok || val.Val != 1 {
		t.Fatalf("checkpoint_started = %v, want 1", started)
	}

	finished, errCode := store.PropertyValue(system, "checkpoint_finished")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_finished: %v", errCode)
	}
	if val, ok := finished.(types.IntValue); !ok || val.Val != 1 {
		t.Fatalf("checkpoint_finished = %v, want 1", finished)
	}
}

func TestPanicReturnsTerminalErrorWithoutGracefulShutdown(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, dbstore.NewProperty("checkpoint_started", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, dbstore.NewProperty("checkpoint_finished", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, dbstore.NewProperty("shutdown_started", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define shutdown_started property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "#0.checkpoint_started = 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = args[1];")
	addTestVerb(store, system, "shutdown_started", "#0.shutdown_started = 1;")

	scheduler := runtime.NewScheduler(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{
		store:          store,
		scheduler:      scheduler,
		input:          NewInputProcessor(store, scheduler),
		connManager:    NewConnectionManager(7777),
		dbPath:         filepath.Join(t.TempDir(), "panic.db"),
		checkpointChan: make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}

	if err := s.Panic("boom"); !errors.Is(err, ErrPanicShutdown) {
		t.Fatalf("Panic error = %v, want ErrPanicShutdown", err)
	}
	if err := s.mainLoop(); !errors.Is(err, ErrPanicShutdown) {
		t.Fatalf("mainLoop error = %v, want ErrPanicShutdown", err)
	}

	started, errCode := store.PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started: %v", errCode)
	}
	if val, ok := started.(types.IntValue); !ok || val.Val != 1 {
		t.Fatalf("checkpoint_started = %v, want 1", started)
	}

	finished, errCode := store.PropertyValue(system, "checkpoint_finished")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_finished: %v", errCode)
	}
	if val, ok := finished.(types.IntValue); !ok || val.Val != 1 {
		t.Fatalf("checkpoint_finished = %v, want 1", finished)
	}

	shutdownStarted, errCode := store.PropertyValue(system, "shutdown_started")
	if errCode != types.E_NONE {
		t.Fatalf("read shutdown_started: %v", errCode)
	}
	if val, ok := shutdownStarted.(types.IntValue); !ok || val.Val != 0 {
		t.Fatalf("shutdown_started = %v, want 0", shutdownStarted)
	}
}

func TestRequestedCheckpointRunsOnServerLoop(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, dbstore.NewProperty("checkpoint_started", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, dbstore.NewProperty("checkpoint_finished", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "#0.checkpoint_started = #0.checkpoint_started + 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = #0.checkpoint_finished + args[1];")

	scheduler := runtime.NewScheduler(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{
		store:          store,
		scheduler:      scheduler,
		input:          NewInputProcessor(store, scheduler),
		connManager:    NewConnectionManager(7777),
		dbPath:         filepath.Join(t.TempDir(), "requested.db"),
		checkpointChan: make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}

	if err := s.requestCheckpoint(); err != nil {
		t.Fatalf("request checkpoint: %v", err)
	}
	if err := s.requestCheckpoint(); err != nil {
		t.Fatalf("second request checkpoint: %v", err)
	}

	started, errCode := store.PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started before loop: %v", errCode)
	}
	if val, ok := started.(types.IntValue); !ok || val.Val != 0 {
		t.Fatalf("checkpoint_started before loop = %v, want 0", started)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.mainLoop()
	}()

	deadline := time.After(time.Second)
	for {
		finished, errCode := store.PropertyValue(system, "checkpoint_finished")
		if errCode != types.E_NONE {
			t.Fatalf("read checkpoint_finished: %v", errCode)
		}
		if val, ok := finished.(types.IntValue); ok && val.Val == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("checkpoint request was not processed")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("main loop: %v", err)
	}

	started, errCode = store.PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started after loop: %v", errCode)
	}
	if val, ok := started.(types.IntValue); !ok || val.Val != 1 {
		t.Fatalf("checkpoint_started after loop = %v, want one coalesced checkpoint", started)
	}
}
