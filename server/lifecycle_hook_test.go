package server

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"barn/builtins"
	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/kernel"
	runtime "barn/scheduler"
	"barn/types"
	"barn/vm"
)

func TestShutdownHostCallbackUsesSchedulerBoundaryWhenNotRunningAndOnPanic(t *testing.T) {
	tests := []struct {
		name    string
		unclean bool
		want    int
	}{
		{name: "not running", want: 1},
		{name: "panic", unclean: true, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("..", "Test_fresh2.db"))
			if err != nil {
				t.Fatalf("read source database: %v", err)
			}
			dbPath := filepath.Join(t.TempDir(), "callback.db")
			if err := os.WriteFile(dbPath, source, 0o600); err != nil {
				t.Fatalf("write test database: %v", err)
			}
			s, err := NewServer(dbPath, []builtins.ListenerSpec{{Protocol: builtins.ListenerProtocolTCP, Port: 7777}}, 0)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			if err := s.LoadDatabase(); err != nil {
				t.Fatalf("LoadDatabase: %v", err)
			}
			t.Cleanup(s.scheduler.Stop)

			anonID, errCode := s.store.CreateObject([]types.ObjID{0}, 2, true)
			if errCode != types.E_NONE {
				t.Fatalf("create caller anonymous object: %v", errCode)
			}
			callerVM := vm.NewVM(s.store, s.scheduler.Registry())
			callerVM.PendingFinalizations = []types.Value{types.NewAnon(anonID)}
			var handedOff []types.Value
			s.scheduler.SetPendingFinalizationSink(func(values []types.Value) {
				handedOff = append(handedOff, values...)
			})

			ctx := kernel.NewTaskContext()
			ctx.IsWizard = true
			ctx.Programmer = 2
			ctx.Registry = s.scheduler.Registry()
			ctx.CallerVM = callerVM
			shutdown, ok := s.scheduler.Registry().Get("shutdown")
			if !ok {
				t.Fatal("shutdown builtin is not registered")
			}
			result := shutdown(ctx, []types.Value{types.NewStr("test"), types.NewInt(boolInt(tc.unclean))})
			if !result.IsNormal() {
				t.Fatalf("shutdown result = %#v, want normal", result)
			}

			if got := len(handedOff); got != tc.want {
				t.Fatalf("scheduler handoff roots = %v, want %d via canonical boundary", handedOff, tc.want)
			}
		})
	}
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func TestCallServerStartedRunsHookBeforeReturning(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, "started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
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
	if value.Type() != types.TYPE_INT || value.Int() != 1 {
		t.Fatalf("started = %v, want 1 before callServerStarted returns", value)
	}
}

func TestCheckpointedConnectionsDisconnectBeforeServerStarted(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, "events", dbstore.NewProperty(types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define events: %v", errCode)
	}
	addTestVerb(store, system, "user_disconnected",
		`#0.events = {@#0.events, {"disconnected", player, args[1], this}};`,
	)
	addTestVerb(store, system, "server_started",
		`#0.events = {@#0.events, {"started", player, this}};`,
	)

	scheduler := runtime.NewScheduler(store)
	s := &Server{
		store:     store,
		scheduler: scheduler,
		input:     NewInputProcessor(store, scheduler),
		checkpointedConns: []dbformat.ActiveConnection{{
			Player:   -7,
			Listener: 0,
		}},
	}

	s.callCheckpointedConnectionHooks()
	if err := s.callServerStarted(); err != nil {
		t.Fatalf("call server_started: %v", err)
	}

	got, errCode := store.PropertyValue(system, "events")
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
	if errCode := store.DefineProperty(system, "listener_count", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
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
	if errCode := store.DefineProperty(system, "listener_count", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define listener_count property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, "shutdown_message", dbstore.NewProperty(types.NewStr(""), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
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
	if value.Type() != types.TYPE_INT || value.Int() != 1 {
		t.Fatalf("listener_count = %v, want 1", value)
	}

	value, errCode = store.PropertyValue(system, "shutdown_message")
	if errCode != types.E_NONE {
		t.Fatalf("read shutdown_message: %v", errCode)
	}
	if value.Type() != types.TYPE_STR || value.Str() != "Maintenance" {
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
	if errCode := store.DefineProperty(system, "checkpoint_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, "checkpoint_finished", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
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
	if started.Type() != types.TYPE_INT || started.Int() != 1 {
		t.Fatalf("checkpoint_started = %v, want 1", started)
	}

	finished, errCode := store.PropertyValue(system, "checkpoint_finished")
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
	if errCode := store.DefineProperty(system, "checkpoint_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, "checkpoint_finished", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_finished property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, "shutdown_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define shutdown_started property: %v", errCode)
	}
	addTestVerb(store, system, "checkpoint_started", "pending = create(#0, #2, 1); #0.checkpoint_started = 1;")
	addTestVerb(store, system, "checkpoint_finished", "#0.checkpoint_finished = args[1];")
	addTestVerb(store, system, "shutdown_started", "#0.shutdown_started = 1;")

	scheduler := runtime.NewScheduler(store)
	scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)
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
	if started.Type() != types.TYPE_INT || started.Int() != 1 {
		t.Fatalf("checkpoint_started = %v, want 1", started)
	}

	finished, errCode := store.PropertyValue(system, "checkpoint_finished")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_finished: %v", errCode)
	}
	if finished.Type() != types.TYPE_INT || finished.Int() != 1 {
		t.Fatalf("checkpoint_finished = %v, want 1", finished)
	}

	shutdownStarted, errCode := store.PropertyValue(system, "shutdown_started")
	if errCode != types.E_NONE {
		t.Fatalf("read shutdown_started: %v", errCode)
	}
	if shutdownStarted.Type() != types.TYPE_INT || shutdownStarted.Int() != 0 {
		t.Fatalf("shutdown_started = %v, want 0", shutdownStarted)
	}

	reloaded, err := dbformat.LoadDatabase(s.dbPath + ".new")
	if err != nil {
		t.Fatalf("load emergency checkpoint: %v", err)
	}
	if got := len(reloaded.PendingFinalizations); got != 1 {
		t.Fatalf("emergency checkpoint pending roots = %v, want checkpoint_started local preserved after Panic publishes shutdown", reloaded.PendingFinalizations)
	}
}

func TestPanicCheckpointKeepsSuspendedWaifAndAnonymousRootsTaskOwned(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, "held", dbstore.NewProperty(types.None, 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define held property: %v", errCode)
	}
	addTestVerb(store, system, "hold", "w = new_waif(); a = create(#0, #2, 1); w.held = a; suspend();")

	scheduler := runtime.NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)
	if _, err := scheduler.RunServerVerbTask(system, "hold", nil, 2); err != nil {
		t.Fatalf("run suspended root holder: %v", err)
	}
	queued, suspended := scheduler.TaskSnapshots()
	if len(queued) != 0 || len(suspended) != 1 {
		t.Fatalf("task snapshots before panic: queued=%d suspended=%d, want 0 and 1", len(queued), len(suspended))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{
		store:          store,
		scheduler:      scheduler,
		input:          NewInputProcessor(store, scheduler),
		connManager:    NewConnectionManager(7777),
		dbPath:         filepath.Join(t.TempDir(), "panic-live-roots.db"),
		checkpointChan: make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}

	if err := s.Panic("live roots"); !errors.Is(err, ErrPanicShutdown) {
		t.Fatalf("Panic error = %v, want ErrPanicShutdown", err)
	}
	reloaded, err := dbformat.LoadDatabase(s.dbPath + ".new")
	if err != nil {
		t.Fatalf("load emergency checkpoint: %v", err)
	}
	if got := len(reloaded.PendingFinalizations); got != 0 {
		t.Fatalf("emergency checkpoint pending roots = %v, want none", reloaded.PendingFinalizations)
	}
	if got := len(reloaded.SuspendedTasks); got != 1 {
		t.Fatalf("emergency checkpoint suspended tasks = %d, want one", got)
	}
	if got := len(reloaded.AnonymousObjs); got != 1 {
		t.Fatalf("emergency checkpoint anonymous objects = %d, want one task-owned object", got)
	}
}

func TestShutdownPublishesFinalizationHandoffBeforeCancel(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	addTestVerb(store, 0, "after_shutdown", "pending = create(#0, #2, 1);")
	scheduler := runtime.NewScheduler(store)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Server{
		store:       store,
		scheduler:   scheduler,
		running:     true,
		ctx:         ctx,
		cancel:      cancel,
		connManager: NewConnectionManager(7777),
	}
	scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)

	s.Shutdown("test")
	if _, err := scheduler.RunServerVerbTask(0, "after_shutdown", nil, 0); err != nil {
		t.Fatalf("run task after Shutdown: %v", err)
	}
	if got := len(store.Snapshot().PendingFinalizations); got != 1 {
		t.Fatalf("pending roots after Shutdown = %d, want 1", got)
	}
}

func TestRequestedCheckpointRunsOnServerLoop(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(system, "checkpoint_started", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define checkpoint_started property: %v", errCode)
	}
	if errCode := store.DefineProperty(system, "checkpoint_finished", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
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
	if started.Type() != types.TYPE_INT || started.Int() != 0 {
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

	started, errCode = store.PropertyValue(system, "checkpoint_started")
	if errCode != types.E_NONE {
		t.Fatalf("read checkpoint_started after loop: %v", errCode)
	}
	if started.Type() != types.TYPE_INT || started.Int() != 1 {
		t.Fatalf("checkpoint_started after loop = %v, want one coalesced checkpoint", started)
	}
}
