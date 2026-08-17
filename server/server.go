package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Server represents the MOO server
type Server struct {
	store              *dbstore.Store
	runtime            *engine.Runtime
	input              *InputProcessor
	connManager        *ConnectionManager
	checkpointedConns  []dbformat.ActiveConnection
	dbPath             string
	listenerSpecs      []listener.Spec
	checkpointInterval time.Duration
	options            config.Options
	running            bool
	mu                 sync.Mutex
	shutdownMessage    string
	terminalErr        error
	backgroundWG       sync.WaitGroup
	checkpointChan     chan struct{}
	ctx                context.Context
	cancel             context.CancelFunc
	lifecycle          LifecycleObserver
}

// LifecycleObserver reports application lifecycle boundaries to passive
// operator probes. Implementations must be concurrency safe and non-blocking.
type LifecycleObserver interface {
	Ready()
	Draining()
	Stopped()
	Failed()
}

// SetLifecycleObserver installs a per-server lifecycle observer.
func (s *Server) SetLifecycleObserver(observer LifecycleObserver) { s.lifecycle = observer }

var ErrPanicShutdown = errors.New("panic shutdown")

// NewServer creates a new MOO server with default runtime options.
func NewServer(dbPath string, listenerSpecs []listener.Spec, checkpointIntervalSec int) (*Server, error) {
	return NewServerWithOptions(dbPath, listenerSpecs, checkpointIntervalSec, config.DefaultOptions())
}

// NewServerWithOptions creates a new MOO server with the supplied runtime options.
func NewServerWithOptions(dbPath string, listenerSpecs []listener.Spec, checkpointIntervalSec int, options config.Options) (*Server, error) {
	if len(listenerSpecs) == 0 {
		return nil, fmt.Errorf("no listeners configured")
	}
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid runtime options: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		dbPath:             dbPath,
		listenerSpecs:      append([]listener.Spec(nil), listenerSpecs...),
		checkpointInterval: time.Duration(checkpointIntervalSec) * time.Second,
		options:            options,
		checkpointChan:     make(chan struct{}, 1),
		ctx:                ctx,
		cancel:             cancel,
	}, nil
}

// LoadDatabase loads the database from disk
func (s *Server) LoadDatabase() error {
	database, err := dbformat.LoadDatabase(s.dbPath)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}

	s.store, err = database.NewStoreFromDatabase()
	if err != nil {
		return fmt.Errorf("construct store from database: %w", err)
	}
	s.runtime = engine.NewRuntimeWithOptions(s.store, s.options)
	s.input = NewInputProcessor(s.store, s.runtime)
	s.connManager = NewConnectionManager(int(s.listenerSpecs[0].Port))
	s.checkpointedConns = append([]dbformat.ActiveConnection(nil), database.ActiveConnections...)

	// Counters are incremented where the events happen; these two are read on
	// demand because "how many right now" is a question about live state.
	metrics.PublishGauge("barn.tasks_live", s.runtime.LiveTaskCount)
	metrics.PublishGauge("barn.connections_live", func() int64 {
		return int64(len(s.connManager.ConnectedPlayers(true)))
	})

	s.input.SetConnectionManager(s.connManager)
	s.runtime.SetPendingFinalizationSink(s.store.AppendPendingFinalizations)
	s.runtime.AdoptPendingFinalizations(s.store.TakePendingFinalizations())
	s.runtime.SetTaskLineSender(func(player types.ObjID, line string) {
		if conn := s.connManager.GetConnection(player); conn != nil {
			_ = conn.Send(line)
		}
	})
	s.runtime.SetTracebackSender(func(player types.ObjID, err types.ErrorCode, stack []task.ActivationFrame) {
		lines := task.FormatTraceback(stack, err)
		conn := s.connManager.GetConnection(player)
		if conn == nil {
			slog.Error("traceback undeliverable (no connection)",
				slog.Int64("player", int64(player)),
				slog.String("error", types.NewErr(err).String()),
				slog.String("traceback", strings.Join(lines, "\n")))
			return
		}
		for _, line := range lines {
			_ = conn.Send(line)
		}
	})
	s.runtime.SetTaskOutputFlusher(func(player types.ObjID, outputSuffix string) {
		if conn := s.connManager.GetConnection(player); conn != nil {
			conn.Flush()
			if outputSuffix != "" {
				_ = conn.Send(outputSuffix)
			}
		}
	})

	// Wire the host capabilities the server provides onto the runtime's
	// builtin session (the session owns them; there is no global state).
	session := s.runtime.Session()
	host := session.Host()

	// Wire notify() builtin to connection manager
	host.ConnManager = s.connManager

	// Wire the force_input() builtin to the runtime.
	host.InputForcer = s.input
	host.TaskYielder = s.runtime
	host.ProcessStdin = builtins.NewProcessStdin(os.Stdin)

	// dump_database() does not report success until the requested checkpoint is
	// durable and available for managed restart adoption.
	host.Checkpoint = func() error { return s.checkpoint() }
	host.Shutdown = func(execution *builtins.Execution, message string, unclean bool) error {
		var ctx *kernel.TaskContext
		var callerRoots []types.Value
		if execution != nil {
			ctx = execution.TaskContext
			if !ctx.DeferredGC && execution.PendingFinalizations != nil {
				callerRoots = execution.PendingFinalizations()
			}
		}
		shutdownMessage := "Server shutdown"
		if ctx != nil {
			caller := fmt.Sprintf("#%d", ctx.Programmer)
			if name, errCode := s.store.DirectTxn().ObjectName(ctx.Programmer); errCode == types.E_NONE && name != "" {
				caller = name
			}
			shutdownMessage = "shutdown() called by " + caller
			if message != "" {
				shutdownMessage += ": " + message
			}
		} else if message != "" {
			shutdownMessage = message
		}
		panicMessage := message
		if panicMessage == "" {
			panicMessage = shutdownMessage
		}
		ready := s.runtime.BeginShutdownWithRoots(callerRoots)
		if ctx != nil && (ctx.DeferredGC || execution.Task != nil) {
			s.backgroundWG.Add(1)
			go func() {
				defer s.backgroundWG.Done()
				<-ready
				if unclean {
					_ = s.Panic(panicMessage)
					return
				}
				s.Shutdown(shutdownMessage)
			}()
			return nil
		}
		<-ready
		if unclean {
			_ = s.Panic(panicMessage)
			return nil
		}
		s.Shutdown(shutdownMessage)
		return nil
	}
	session.ConfigureHost(host)
	if err := host.Validate(); err != nil {
		return fmt.Errorf("configure builtin host: %w", err)
	}

	// Prime the server-options and protected-builtin caches from the database
	// before any verb runs, matching Toast's boot-time load_server_options() /
	// load_server_protect_function_flags(). The MOO may refresh these later by
	// calling load_server_options().
	s.runtime.Session().LoadServerOptionsFromStore(s.store)
	s.runtime.Session().LoadProtectedBuiltinsFromStore(s.store)

	s.runtime.LoadQueuedTasks(database.QueuedTasks)
	s.runtime.LoadSuspendedTasks(database.SuspendedTasks)

	slog.Info("database loaded",
		slog.Int("version", database.Version),
		slog.Int("objects", len(database.Objects)))
	return nil
}

// GetStore returns the object store
func (s *Server) GetStore() *dbstore.Store {
	return s.store
}

// Start starts the server
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	// Start the execution runtime.
	s.input.Start()

	// Bind listener sockets before server_started so MOO code can inspect
	// listeners(), but do not accept connections until the hook returns.
	if err := s.connManager.BindListeners(s.listenerSpecs); err != nil {
		if s.lifecycle != nil {
			s.lifecycle.Failed()
		}
		s.cancel()
		s.connManager.CloseListeners()
		s.input.Stop()
		s.runtime.Stop()
		s.backgroundWG.Wait()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return fmt.Errorf("listen failed: %w", err)
	}

	s.callCheckpointedConnectionHooks()

	// Call #0:server_started()
	if err := s.callServerStarted(); err != nil {
		slog.Warn("#0:server_started() failed", slog.Any("err", err))
	}

	// Start listening for connections
	if s.lifecycle != nil {
		s.lifecycle.Ready()
	}
	s.connManager.StartAccepting()

	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		s.checkpointLoop()
	}()

	// Main loop
	return s.mainLoop()
}

// mainLoop is the main server loop
func (s *Server) mainLoop() error {
	for {
		select {
		case <-s.ctx.Done():
			s.mu.Lock()
			terminalErr := s.terminalErr
			s.mu.Unlock()
			if terminalErr != nil {
				return terminalErr
			}
			return s.shutdown()
		case <-s.checkpointChan:
			if err := s.checkpoint(); err != nil {
				slog.Error("checkpoint failed", slog.Any("err", err))
			}
		}
	}
}

// checkpointLoop runs periodic checkpoints
func (s *Server) checkpointLoop() {
	if s.checkpointInterval <= 0 {
		return // Checkpointing disabled
	}
	ticker := time.NewTicker(s.checkpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.requestCheckpoint(); err != nil {
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Server) requestCheckpoint() error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}

	select {
	case s.checkpointChan <- struct{}{}:
	default:
	}
	return nil
}

// checkpoint saves the database to disk
func (s *Server) checkpoint() error {
	return s.checkpointWith(dbformat.WriteCheckpoint)
}

type checkpointWriter func(
	string,
	*dbstore.Store,
	[]task.Snapshot,
	[]task.Snapshot,
	[]dbformat.ActiveConnection,
) error

func (s *Server) checkpointWith(writeCheckpoint checkpointWriter) error {
	slog.Info("checkpoint started")

	// Call #0:checkpoint_started()
	if err := s.callCheckpointStarted(); err != nil {
		slog.Warn("#0:checkpoint_started() failed", slog.Any("err", err))
	}

	start := time.Now()

	queuedTasks, suspendedTasks := s.runtime.TaskSnapshots()
	activeConnections := s.connManager.CheckpointConnections()
	if err := writeCheckpoint(s.dbPath, s.store, queuedTasks, suspendedTasks, activeConnections); err != nil {
		s.callCheckpointFinished(false)
		return err
	}

	// Call #0:checkpoint_finished(success)
	if err := s.callCheckpointFinished(true); err != nil {
		slog.Warn("#0:checkpoint_finished() failed", slog.Any("err", err))
	}

	elapsed := time.Since(start)
	metrics.Checkpoints.Add(1)
	metrics.CheckpointLastMs.Set(elapsed.Milliseconds())

	slog.Info("checkpoint complete", slog.Int64("duration_ms", elapsed.Milliseconds()))
	return nil
}

// Shutdown initiates graceful shutdown.
func (s *Server) Shutdown(message string) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.shutdownMessage = message
	if s.lifecycle != nil {
		s.lifecycle.Draining()
	}
	s.mu.Unlock()

	slog.Info("initiating shutdown", slog.String("message", message))
	s.cancel()
}

// shutdown performs the actual shutdown sequence
func (s *Server) shutdown() error {
	slog.Info("shutting down")

	s.mu.Lock()
	message := s.shutdownMessage
	s.mu.Unlock()
	if message == "" {
		message = "Server shutdown"
	}

	// Call #0:shutdown_started()
	if err := s.callShutdownStarted(message); err != nil {
		slog.Warn("#0:shutdown_started() failed", slog.Any("err", err))
	}

	s.connManager.CloseListeners()
	s.connManager.CloseConnections(message)

	s.input.Stop()

	// Final checkpoint (unless checkpointing was explicitly disabled)
	if s.checkpointInterval > 0 {
		if err := s.checkpoint(); err != nil {
			slog.Error("final checkpoint failed", slog.Any("err", err))
		}
	} else {
		slog.Info("final checkpoint skipped (checkpointing disabled)")
	}

	s.runtime.Stop()
	s.backgroundWG.Wait()
	if err := s.runtime.Session().Close(); err != nil {
		slog.Warn("closing builtin session", slog.Any("err", err))
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	if s.lifecycle != nil {
		s.lifecycle.Stopped()
	}

	slog.Info("shutdown complete")
	return nil
}

// Panic performs emergency shutdown
func (s *Server) Panic(message string) error {
	if s.lifecycle != nil {
		s.lifecycle.Draining()
	}
	// The Go stack is the only record of where the server actually tripped;
	// the message alone says that it died, not why.
	slog.Error("PANIC: "+message,
		slog.String("panic", message),
		slog.String("go_stack", string(debug.Stack())))

	// Attempt emergency database dump
	if err := s.checkpointWith(dbformat.WritePanicCheckpoint); err != nil {
		slog.Error("emergency dump failed", slog.Any("err", err))
	} else {
		slog.Info("emergency dump written", slog.String("path", s.dbPath+".new.PANIC"))
	}

	err := fmt.Errorf("%w: %s", ErrPanicShutdown, message)
	s.mu.Lock()
	s.terminalErr = err
	s.mu.Unlock()
	if s.lifecycle != nil {
		s.lifecycle.Failed()
	}
	s.cancel()
	return err
}

// callCheckpointedConnectionHooks reports connections present at checkpoint
// time as disconnected before the server_started hook runs.
func (s *Server) callCheckpointedConnectionHooks() {
	for _, connection := range s.checkpointedConns {
		s.input.callUserHook(connection.Listener, "user_disconnected", connection.Player)
	}
	s.checkpointedConns = nil
}

// callServerStarted calls #0:server_started()
func (s *Server) callServerStarted() error {
	if !s.store.HasLocalVerb(0, "server_started") {
		return nil
	}
	_, err := s.runtime.RunServerVerbTask(0, "server_started", nil, 0)
	return err
}

// callCheckpointStarted calls #0:checkpoint_started()
func (s *Server) callCheckpointStarted() error {
	if !s.store.HasLocalVerb(0, "checkpoint_started") {
		return nil
	}
	_, err := s.runtime.RunServerVerbTask(0, "checkpoint_started", nil, 0)
	return err
}

// callCheckpointFinished calls #0:checkpoint_finished(success)
func (s *Server) callCheckpointFinished(success bool) error {
	if !s.store.HasLocalVerb(0, "checkpoint_finished") {
		return nil
	}
	_, err := s.runtime.RunServerVerbTask(0, "checkpoint_finished", []types.Value{types.NewInt(boolToInt(success))}, 0)
	return err
}

// callShutdownStarted calls #0:shutdown_started(message)
func (s *Server) callShutdownStarted(message string) error {
	if !s.store.HasLocalVerb(0, "shutdown_started") {
		return nil
	}
	_, err := s.runtime.RunServerVerbTask(0, "shutdown_started", []types.Value{types.NewStr(message)}, 0)
	return err
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
