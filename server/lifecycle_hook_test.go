package server

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	listenercfg "github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/types"
)

type recordingLifecycle struct {
	states []string
}

func (l *recordingLifecycle) Ready()    { l.states = append(l.states, "ready") }
func (l *recordingLifecycle) Draining() { l.states = append(l.states, "draining") }
func (l *recordingLifecycle) Stopped()  { l.states = append(l.states, "stopped") }
func (l *recordingLifecycle) Failed()   { l.states = append(l.states, "failed") }

func TestCallServerStartedRunsHookBeforeReturning(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define property: %v", errCode)
	}
	addTestVerb(store, system, "server_started", "#0.started = 1;")

	s := &Server{
		store:   store,
		runtime: engine.NewRuntime(store),
	}

	if err := s.callServerStarted(); err != nil {
		t.Fatalf("call server_started: %v", err)
	}

	value, errCode := store.DirectTxn().PropertyValue(system, "started")
	if errCode != types.E_NONE {
		t.Fatalf("read property: %v", errCode)
	}
	if value.Type() != types.TYPE_INT || value.Int() != 1 {
		t.Fatalf("started = %v, want 1 before callServerStarted returns", value)
	}
}

func TestCheckpointedConnectionsDisconnectBeforeServerStarted(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "events", dbstore.NewProperty(types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define events: %v", errCode)
	}
	addTestVerb(store, system, "user_disconnected",
		`#0.events = {@#0.events, {"disconnected", player, args[1], this}};`,
	)
	addTestVerb(store, system, "server_started",
		`#0.events = {@#0.events, {"started", player, this}};`,
	)

	rt := engine.NewRuntime(store)
	s := &Server{
		store:   store,
		runtime: rt,
		input:   NewInputProcessor(store, rt),
		checkpointedConns: []dbformat.ActiveConnection{{
			Player:   -7,
			Listener: 0,
		}},
	}

	s.callCheckpointedConnectionHooks()
	if err := s.callServerStarted(); err != nil {
		t.Fatalf("call server_started: %v", err)
	}

	got, errCode := store.DirectTxn().PropertyValue(system, "events")
	if errCode != types.E_NONE {
		t.Fatalf("read events: %v", errCode)
	}
	want := types.NewList([]types.Value{
		types.NewList([]types.Value{
			types.NewStr("disconnected"),
			types.NewObj(-7),
			types.NewObj(-7),
			types.NewObj(0),
		}),
		types.NewList([]types.Value{
			types.NewStr("started"),
			types.NewObj(0),
			types.NewObj(0),
		}),
	})
	if !got.Equal(want) {
		t.Fatalf("startup events = %s, want %s", got.String(), want.String())
	}
}

func TestServerStartedCanSeeBoundListenersBeforeAccepting(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "listener_count", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define property: %v", errCode)
	}
	addTestVerb(store, system, "server_started", "#0.listener_count = length(listeners());")

	cm := NewConnectionManager(0)
	if err := cm.BindListeners([]listenercfg.Spec{{
		Protocol:  listenercfg.ProtocolTCP,
		Port:      0,
		Interface: "127.0.0.1",
	}}); err != nil {
		t.Fatalf("bind listeners: %v", err)
	}
	defer cm.CloseListeners()

	s := &Server{
		store:   store,
		runtime: engine.NewRuntime(store),
	}
	setTestConnectionManager(s.runtime.Session(), cm)

	if err := s.callServerStarted(); err != nil {
		t.Fatalf("call server_started: %v", err)
	}

	value, errCode := store.DirectTxn().PropertyValue(system, "listener_count")
	if errCode != types.E_NONE {
		t.Fatalf("read property: %v", errCode)
	}
	if value.Type() != types.TYPE_INT || value.Int() != 1 {
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
	rt := engine.NewRuntime(store)
	input := NewInputProcessor(store, rt)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lifecycle := &recordingLifecycle{}
	s := &Server{
		store:          store,
		runtime:        rt,
		input:          input,
		connManager:    NewConnectionManager(int(port)),
		listenerSpecs:  []listenercfg.Spec{{Protocol: listenercfg.ProtocolTCP, Port: port, Interface: "127.0.0.1"}},
		checkpointChan: make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
		lifecycle:      lifecycle,
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
	if got := lifecycle.states; len(got) != 1 || got[0] != "failed" {
		t.Fatalf("lifecycle states = %v, want [failed]", got)
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
	if errCode := store.DirectTxn().DefineProperty(system, "listener_count", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define listener_count property: %v", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(system, "shutdown_message", dbstore.NewProperty(types.NewStr(""), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define shutdown_message property: %v", errCode)
	}
	addTestVerb(store, system, "shutdown_started",
		"#0.listener_count = length(listeners());",
		"#0.shutdown_message = args[1];",
	)

	cm := NewConnectionManager(7777)
	listener := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}
	if _, err := cm.registerListener(listener, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   0,
	}, true, nil); err != nil {
		t.Fatalf("register listener: %v", err)
	}
	defer cm.CloseListeners()

	rt := engine.NewRuntime(store)
	setTestConnectionManager(rt.Session(), cm)
	lifecycle := &recordingLifecycle{}
	s := &Server{
		store:           store,
		runtime:         rt,
		input:           NewInputProcessor(store, rt),
		connManager:     cm,
		shutdownMessage: "Maintenance",
		lifecycle:       lifecycle,
	}

	if err := s.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	value, errCode := store.DirectTxn().PropertyValue(system, "listener_count")
	if errCode != types.E_NONE {
		t.Fatalf("read listener_count: %v", errCode)
	}
	if value.Type() != types.TYPE_INT || value.Int() != 1 {
		t.Fatalf("listener_count = %v, want 1", value)
	}

	value, errCode = store.DirectTxn().PropertyValue(system, "shutdown_message")
	if errCode != types.E_NONE {
		t.Fatalf("read shutdown_message: %v", errCode)
	}
	if value.Type() != types.TYPE_STR || value.Str() != "Maintenance" {
		t.Fatalf("shutdown_message = %v, want Maintenance", value)
	}

	if !listener.closed {
		t.Fatalf("listener was not closed")
	}
	if got := lifecycle.states; len(got) != 1 || got[0] != "stopped" {
		t.Fatalf("lifecycle states = %v, want [stopped]", got)
	}
	if infos := cm.ListenerInfos(); len(infos) != 0 {
		t.Fatalf("listeners after shutdown = %+v, want none", infos)
	}
}

func TestShutdownClosesActiveConnectionsWithMessage(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	cm := NewConnectionManager(7777)
	transport := newRecordingTransport("client")
	cm.NewConnectionFromTransport(transport)

	s := &Server{
		store:           store,
		runtime:         rt,
		input:           NewInputProcessor(store, rt),
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
	rt := engine.NewRuntime(store)
	cm := NewConnectionManager(7777)
	input := NewInputProcessor(store, rt)
	input.SetConnectionManager(cm)
	input.Start()

	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	cm.connectionWG.Add(1)
	go cm.handleConnection(conn)

	s := &Server{
		store:           store,
		runtime:         rt,
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
	rt := engine.NewRuntime(store)
	s := &Server{
		store:           store,
		runtime:         rt,
		input:           NewInputProcessor(store, rt),
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

func TestShutdownFinalCheckpointRunsHooksBeforeRuntimeStops(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "checkpoint_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(system, "checkpoint_finished", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "#0.checkpoint_started = 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = args[1];")

	rt := engine.NewRuntime(store)
	s := &Server{
		store:              store,
		runtime:            rt,
		input:              NewInputProcessor(store, rt),
		connManager:        NewConnectionManager(7777),
		dbPath:             filepath.Join(t.TempDir(), "shutdown.db"),
		checkpointInterval: time.Second,
		shutdownMessage:    "Maintenance",
	}

	if err := s.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	started, errCode := store.DirectTxn().PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started: %v", errCode)
	}
	if started.Type() != types.TYPE_INT || started.Int() != 1 {
		t.Fatalf("checkpoint_started = %v, want 1", started)
	}

	finished, errCode := store.DirectTxn().PropertyValue(system, "checkpoint_finished")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_finished: %v", errCode)
	}
	if finished.Type() != types.TYPE_INT || finished.Int() != 1 {
		t.Fatalf("checkpoint_finished = %v, want 1", finished)
	}
}

func TestPanicReturnsTerminalErrorWithoutGracefulShutdown(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "checkpoint_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(system, "checkpoint_finished", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(system, "shutdown_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define shutdown_started property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "#0.checkpoint_started = 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = args[1];")
	addTestVerb(store, system, "shutdown_started", "#0.shutdown_started = 1;")

	rt := engine.NewRuntime(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dbPath := filepath.Join(t.TempDir(), "panic.db")
	s := &Server{
		store:          store,
		runtime:        rt,
		input:          NewInputProcessor(store, rt),
		connManager:    NewConnectionManager(7777),
		dbPath:         dbPath,
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

	started, errCode := store.DirectTxn().PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started: %v", errCode)
	}
	if started.Type() != types.TYPE_INT || started.Int() != 1 {
		t.Fatalf("checkpoint_started = %v, want 1", started)
	}

	finished, errCode := store.DirectTxn().PropertyValue(system, "checkpoint_finished")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_finished: %v", errCode)
	}
	if finished.Type() != types.TYPE_INT || finished.Int() != 1 {
		t.Fatalf("checkpoint_finished = %v, want 1", finished)
	}

	shutdownStarted, errCode := store.DirectTxn().PropertyValue(system, "shutdown_started")
	if errCode != types.E_NONE {
		t.Fatalf("read shutdown_started: %v", errCode)
	}
	if shutdownStarted.Type() != types.TYPE_INT || shutdownStarted.Int() != 0 {
		t.Fatalf("shutdown_started = %v, want 0", shutdownStarted)
	}

	if _, err := os.Stat(dbPath + ".new.PANIC"); err != nil {
		t.Fatalf("stat emergency checkpoint: %v", err)
	}
	if _, err := os.Stat(dbPath + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary checkpoint exists after panic: %v", err)
	}
}

func TestRequestedCheckpointRunsOnServerLoop(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(system, "checkpoint_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(system, "checkpoint_finished", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "#0.checkpoint_started = #0.checkpoint_started + 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = #0.checkpoint_finished + args[1];")

	rt := engine.NewRuntime(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{
		store:          store,
		runtime:        rt,
		input:          NewInputProcessor(store, rt),
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

	started, errCode := store.DirectTxn().PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started before loop: %v", errCode)
	}
	if started.Type() != types.TYPE_INT || started.Int() != 0 {
		t.Fatalf("checkpoint_started before loop = %v, want 0", started)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.mainLoop()
	}()

	deadline := time.After(time.Second)
	for {
		finished, errCode := store.DirectTxn().PropertyValue(system, "checkpoint_finished")
		if errCode != types.E_NONE {
			t.Fatalf("read checkpoint_finished: %v", errCode)
		}
		if finished.Type() == types.TYPE_INT && finished.Int() == 1 {
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

	started, errCode = store.DirectTxn().PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started after loop: %v", errCode)
	}
	if started.Type() != types.TYPE_INT || started.Int() != 1 {
		t.Fatalf("checkpoint_started after loop = %v, want one coalesced checkpoint", started)
	}
}
