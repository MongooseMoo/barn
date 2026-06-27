package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"barn/builtins"
	"barn/config"
	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/kernel"
	runtime "barn/scheduler"
	"barn/task"
	"barn/types"
	"barn/vm"
)

// Server represents the MOO server
type Server struct {
	store              *dbstore.Store
	scheduler          *runtime.Scheduler
	input              *InputProcessor
	connManager        *ConnectionManager
	dbPath             string
	listenerSpecs      []builtins.ListenerSpec
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
}

var ErrPanicShutdown = errors.New("panic shutdown")

// NewServer creates a new MOO server with default runtime options.
func NewServer(dbPath string, listenerSpecs []builtins.ListenerSpec, checkpointIntervalSec int) (*Server, error) {
	return NewServerWithOptions(dbPath, listenerSpecs, checkpointIntervalSec, config.DefaultOptions())
}

// NewServerWithOptions creates a new MOO server with the supplied runtime options.
func NewServerWithOptions(dbPath string, listenerSpecs []builtins.ListenerSpec, checkpointIntervalSec int, options config.Options) (*Server, error) {
	if len(listenerSpecs) == 0 {
		return nil, fmt.Errorf("no listeners configured")
	}
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid runtime options: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		dbPath:             dbPath,
		listenerSpecs:      append([]builtins.ListenerSpec(nil), listenerSpecs...),
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

	s.store = database.NewStoreFromDatabase()
	s.scheduler = runtime.NewSchedulerWithOptions(s.store, s.options)
	s.input = NewInputProcessor(s.store, s.scheduler)
	s.connManager = NewConnectionManager(int(s.listenerSpecs[0].Port))

	s.input.SetConnectionManager(s.connManager)
	s.scheduler.SetPendingFinalizationSink(s.store.AppendPendingFinalizations)
	s.scheduler.SetTaskLineSender(func(player types.ObjID, line string) {
		if conn := s.connManager.GetConnection(player); conn != nil {
			_ = conn.Send(line)
		}
	})
	s.scheduler.SetTracebackSender(func(player types.ObjID, err types.ErrorCode, stack []task.ActivationFrame) {
		lines := task.FormatTraceback(stack, err)
		conn := s.connManager.GetConnection(player)
		if conn == nil {
			log.Printf("Traceback for player %v (connection not found):", player)
			for _, line := range lines {
				log.Printf("  %s", line)
			}
			return
		}
		for _, line := range lines {
			_ = conn.Send(line)
		}
	})
	s.scheduler.SetTaskOutputFlusher(func(player types.ObjID, outputSuffix string) {
		if conn := s.connManager.GetConnection(player); conn != nil {
			conn.Flush()
			if outputSuffix != "" {
				_ = conn.Send(outputSuffix)
			}
		}
	})

	// Wire the host capabilities the server provides onto the scheduler's
	// builtin registry (the registry owns them; there is no global state).
	reg := s.scheduler.Registry()

	// Wire notify() builtin to connection manager
	reg.SetConnectionManager(s.connManager)

	// Wire force_input() builtin to scheduler
	reg.SetInputForcer(s.input)
	reg.SetTaskYielder(s.scheduler)

	// Wire dump_database() builtin to request a server-loop checkpoint.
	reg.SetDumpFunc(func() error { return s.requestCheckpoint() })
	reg.SetShutdownFunc(func(ctx *kernel.TaskContext, message string, unclean bool) error {
		if ctx != nil {
			if callerVM, ok := ctx.CallerVM.(*vm.VM); ok {
				s.store.AppendPendingFinalizations(vm.CollectPendingFinalizationValues(s.store, callerVM))
			}
		}
		shutdownMessage := "Server shutdown"
		if ctx != nil {
			caller := fmt.Sprintf("#%d", ctx.Programmer)
			if name, errCode := s.store.ObjectName(ctx.Programmer); errCode == types.E_NONE && name != "" {
				caller = name
			}
			shutdownMessage = "shutdown() called by " + caller
			if message != "" {
				shutdownMessage += ": " + message
			}
		} else if message != "" {
			shutdownMessage = message
		}
		if unclean {
			s.Panic(shutdownMessage)
			return nil
		}
		s.Shutdown(shutdownMessage)
		return nil
	})

	// Prime the server-options and protected-builtin caches from the database
	// before any verb runs, matching Toast's boot-time load_server_options() /
	// load_server_protect_function_flags(). The MOO may refresh these later by
	// calling load_server_options().
	builtins.LoadServerOptionsFromStore(s.store)
	builtins.LoadProtectedBuiltinsFromStore(s.store)

	s.scheduler.LoadQueuedTasks(database.QueuedTasks)

	log.Printf("Loaded database version %d with %d objects", database.Version, len(database.Objects))
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

	// Start scheduler
	s.input.Start()

	// Bind listener sockets before server_started so MOO code can inspect
	// listeners(), but do not accept connections until the hook returns.
	if err := s.connManager.BindListeners(s.listenerSpecs); err != nil {
		s.cancel()
		s.connManager.CloseListeners()
		s.input.Stop()
		s.scheduler.Stop()
		s.backgroundWG.Wait()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return fmt.Errorf("listen failed: %w", err)
	}

	// Call #0:server_started()
	if err := s.callServerStarted(); err != nil {
		log.Printf("Warning: #0:server_started() failed: %v", err)
	}

	// Start listening for connections
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
				log.Printf("Checkpoint failed: %v", err)
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
	log.Println("Starting checkpoint...")

	// Call #0:checkpoint_started()
	if err := s.callCheckpointStarted(); err != nil {
		log.Printf("Warning: #0:checkpoint_started() failed: %v", err)
	}

	start := time.Now()

	queuedTasks, suspendedTasks := s.scheduler.TaskSnapshots()
	if err := dbformat.WriteCheckpoint(s.dbPath, s.store.Snapshot(), queuedTasks, suspendedTasks); err != nil {
		s.callCheckpointFinished(false)
		return err
	}

	// Call #0:checkpoint_finished(success)
	if err := s.callCheckpointFinished(true); err != nil {
		log.Printf("Warning: #0:checkpoint_finished() failed: %v", err)
	}

	log.Printf("Checkpoint complete in %v", time.Since(start))
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
	s.mu.Unlock()

	log.Println("Initiating shutdown...")
	s.cancel()
}

// shutdown performs the actual shutdown sequence
func (s *Server) shutdown() error {
	log.Println("Shutting down server...")

	s.mu.Lock()
	message := s.shutdownMessage
	s.mu.Unlock()
	if message == "" {
		message = "Server shutdown"
	}

	// Call #0:shutdown_started()
	if err := s.callShutdownStarted(message); err != nil {
		log.Printf("Warning: #0:shutdown_started() failed: %v", err)
	}

	s.connManager.CloseListeners()
	s.connManager.CloseConnections(message)

	s.input.Stop()

	// Final checkpoint (unless checkpointing was explicitly disabled)
	if s.checkpointInterval > 0 {
		log.Println("Performing final checkpoint...")
		if err := s.checkpoint(); err != nil {
			log.Printf("Warning: final checkpoint failed: %v", err)
		}
	} else {
		log.Println("Final checkpoint skipped (checkpointing disabled)")
	}

	s.scheduler.Stop()
	s.backgroundWG.Wait()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	log.Println("Server shutdown complete")
	return nil
}

// Panic performs emergency shutdown
func (s *Server) Panic(message string) error {
	log.Printf("PANIC: %s", message)

	// Attempt emergency database dump
	log.Println("Attempting emergency database dump...")
	if err := s.checkpoint(); err != nil {
		log.Printf("Emergency dump failed: %v", err)
	}

	err := fmt.Errorf("%w: %s", ErrPanicShutdown, message)
	s.mu.Lock()
	s.terminalErr = err
	s.mu.Unlock()
	s.cancel()
	return err
}

// callServerStarted calls #0:server_started()
func (s *Server) callServerStarted() error {
	if !s.store.HasLocalVerb(0, "server_started") {
		return nil
	}
	_, err := s.scheduler.RunServerVerbTask(0, "server_started", nil, 0)
	return err
}

// callCheckpointStarted calls #0:checkpoint_started()
func (s *Server) callCheckpointStarted() error {
	if !s.store.HasLocalVerb(0, "checkpoint_started") {
		return nil
	}
	_, err := s.scheduler.RunServerVerbTask(0, "checkpoint_started", nil, 0)
	return err
}

// callCheckpointFinished calls #0:checkpoint_finished(success)
func (s *Server) callCheckpointFinished(success bool) error {
	if !s.store.HasLocalVerb(0, "checkpoint_finished") {
		return nil
	}
	_, err := s.scheduler.RunServerVerbTask(0, "checkpoint_finished", []types.Value{types.NewInt(boolToInt(success))}, 0)
	return err
}

// callShutdownStarted calls #0:shutdown_started(message)
func (s *Server) callShutdownStarted(message string) error {
	if !s.store.HasLocalVerb(0, "shutdown_started") {
		return nil
	}
	_, err := s.scheduler.RunServerVerbTask(0, "shutdown_started", []types.Value{types.NewStr(message)}, 0)
	return err
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
