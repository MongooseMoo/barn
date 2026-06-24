package server

import (
	"net"
	"testing"

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

	builtins.SetConnectionManager(cm)
	t.Cleanup(func() { builtins.SetConnectionManager(nil) })

	s := &Server{
		store:     store,
		scheduler: runtime.NewScheduler(store),
	}

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

	builtins.SetConnectionManager(cm)
	t.Cleanup(func() { builtins.SetConnectionManager(nil) })

	scheduler := runtime.NewScheduler(store)
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
