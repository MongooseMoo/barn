package server

import (
	"barn/builtins"
	"barn/config"
	"barn/db"
	"barn/types"
	"barn/vm"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Server represents the MOO server
type Server struct {
	store              *db.Store
	database           *db.Database
	scheduler          *Scheduler
	connManager        *ConnectionManager
	dbPath             string
	listenerSpecs      []builtins.ListenerSpec
	checkpointInterval time.Duration
	options            config.Options
	running            bool
	mu                 sync.Mutex
	shutdownChan       chan struct{}
	checkpointChan     chan struct{}
	ctx                context.Context
	cancel             context.CancelFunc
	startupReady       chan struct{}
}

// NewServer creates a new MOO server
func NewServer(dbPath string, listenerSpecs []builtins.ListenerSpec, checkpointIntervalSec int) (*Server, error) {
	return NewServerWithOptions(dbPath, listenerSpecs, checkpointIntervalSec, config.DefaultOptions())
}

// NewServerWithOptions creates a new MOO server with explicit runtime options.
func NewServerWithOptions(dbPath string, listenerSpecs []builtins.ListenerSpec, checkpointIntervalSec int, options config.Options) (*Server, error) {
	if len(listenerSpecs) == 0 {
		return nil, fmt.Errorf("no listeners configured")
	}
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		dbPath:             dbPath,
		listenerSpecs:      append([]builtins.ListenerSpec(nil), listenerSpecs...),
		checkpointInterval: time.Duration(checkpointIntervalSec) * time.Second,
		options:            options,
		shutdownChan:       make(chan struct{}),
		checkpointChan:     make(chan struct{}),
		ctx:                ctx,
		cancel:             cancel,
		startupReady:       make(chan struct{}),
	}, nil
}

// LoadDatabase loads the database from disk
func (s *Server) LoadDatabase() error {
	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}

	s.database = database
	s.store = database.NewStoreFromDatabase()
	s.scheduler = NewSchedulerWithOptions(s.store, s.options)
	for _, queued := range database.QueuedTasks {
		if err := s.scheduler.RestoreQueuedTask(queued); err != nil {
			log.Printf("restore queued task %d: %v", queued.ID, err)
		}
	}
	s.connManager = NewConnectionManager(s, int(s.listenerSpecs[0].Port))

	// Wire scheduler to connection manager for output flushing
	s.scheduler.SetConnectionManager(s.connManager)
	s.scheduler.SetPendingFinalizationSink(func(values []types.Value) {
		s.appendPendingFinalizations(values)
	})

	// Wire notify() builtin to connection manager
	builtins.SetConnectionManager(s.connManager)

	// Wire force_input() builtin to scheduler
	builtins.SetInputForcer(s.scheduler)
	builtins.SetTaskYielder(s.scheduler)

	// Wire dump_database() builtin to server checkpoint
	builtins.SetDumpFunc(func() error { return s.checkpoint() })
	builtins.SetShutdownFunc(func(ctx *types.TaskContext) error {
		if ctx != nil {
			if callerVM, ok := ctx.CallerVM.(*vm.VM); ok {
				s.appendPendingFinalizations(vm.CollectPendingFinalizationValues(s.store, callerVM))
			}
		}
		s.Shutdown()
		return nil
	})

	log.Printf("Loaded database version %d with %d objects", database.Version, len(database.Objects))
	return nil
}

// GetStore returns the object store
func (s *Server) GetStore() *db.Store {
	return s.store
}

// GetEvaluator returns the evaluator from the scheduler
func (s *Server) GetEvaluator() *vm.Evaluator {
	return s.scheduler.GetEvaluator()
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
	s.scheduler.Start()

	// Start listening before #0:server_started() so startup code that checks
	// listeners() sees the primary listener, matching Toast's prod detection.
	if err := s.connManager.StartListeners(s.listenerSpecs); err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	s.syncPrimaryListenerPortProperty()

	// Call #0:server_started()
	if err := s.callServerStarted(); err != nil {
		log.Printf("Warning: #0:server_started() failed: %v", err)
	}
	if err := s.waitForStartupQuiescence(5 * time.Second); err != nil {
		log.Printf("Warning: startup tasks still active: %v", err)
	}
	close(s.startupReady)

	// Set up signal handling
	go s.handleSignals()

	// Set up periodic checkpoints
	go s.checkpointLoop()

	// Main loop
	return s.mainLoop()
}

// mainLoop is the main server loop
func (s *Server) mainLoop() error {
	for {
		select {
		case <-s.ctx.Done():
			return s.shutdown()
		case <-s.checkpointChan:
			if err := s.checkpoint(); err != nil {
				log.Printf("Checkpoint failed: %v", err)
			}
		}
	}
}

func (s *Server) waitUntilStartupReady() bool {
	select {
	case <-s.startupReady:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *Server) waitForStartupQuiescence(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !s.scheduler.HasImmediateTasks(time.Now()) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", timeout)
		}
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// handleSignals handles OS signals
func (s *Server) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Received shutdown signal")
		s.Shutdown()
	case <-s.ctx.Done():
		return
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
			s.checkpointChan <- struct{}{}
		case <-s.ctx.Done():
			return
		}
	}
}

// checkpoint saves the database to disk
func (s *Server) checkpoint() error {
	log.Println("Starting checkpoint...")

	// Call #0:checkpoint_started()
	if err := s.callCheckpointStarted(); err != nil {
		log.Printf("Warning: #0:checkpoint_started() failed: %v", err)
	}

	start := time.Now()

	// Write to temp file
	tempPath := s.dbPath + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		s.callCheckpointFinished(false)
		return fmt.Errorf("create temp file: %w", err)
	}

	writer := db.NewWriter(tempFile, s.store)
	writer.SetPendingFinalizations(s.database.PendingFinalizations)
	writer.SetTaskSource(s.scheduler) // Provide tasks for serialization
	if err := writer.WriteDatabase(); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		s.callCheckpointFinished(false)
		return fmt.Errorf("write database: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		s.callCheckpointFinished(false)
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename temp -> main database
	if err := os.Rename(tempPath, s.dbPath); err != nil {
		// On Windows, need to remove dest first
		os.Remove(s.dbPath)
		if err := os.Rename(tempPath, s.dbPath); err != nil {
			s.callCheckpointFinished(false)
			return fmt.Errorf("rename temp to main: %w", err)
		}
	}

	if err := copyFile(s.dbPath, s.dbPath+".new"); err != nil {
		s.callCheckpointFinished(false)
		return fmt.Errorf("write sibling checkpoint: %w", err)
	}

	// Call #0:checkpoint_finished(success)
	if err := s.callCheckpointFinished(true); err != nil {
		log.Printf("Warning: #0:checkpoint_finished() failed: %v", err)
	}

	log.Printf("Checkpoint complete in %v", time.Since(start))
	return nil
}

// Shutdown initiates graceful shutdown
func (s *Server) Shutdown() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	log.Println("Initiating shutdown...")
	s.cancel()
}

// shutdown performs the actual shutdown sequence
func (s *Server) shutdown() error {
	log.Println("Shutting down server...")

	// Call #0:shutdown_started()
	if err := s.callShutdownStarted("Server shutdown"); err != nil {
		log.Printf("Warning: #0:shutdown_started() failed: %v", err)
	}

	// Stop accepting new connections and close active transports while the
	// scheduler can still process disconnect hooks.
	s.connManager.Shutdown()

	// Stop scheduler
	s.scheduler.Stop()

	// Final checkpoint (unless checkpointing was explicitly disabled)
	if s.checkpointInterval > 0 {
		log.Println("Performing final checkpoint...")
		if err := s.checkpoint(); err != nil {
			log.Printf("Warning: final checkpoint failed: %v", err)
		}
	} else {
		log.Println("Final checkpoint skipped (checkpointing disabled)")
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	log.Println("Server shutdown complete")
	return nil
}

// Panic performs emergency shutdown
func (s *Server) Panic(message string) {
	log.Printf("PANIC: %s", message)

	// Attempt emergency database dump
	log.Println("Attempting emergency database dump...")
	if err := s.checkpoint(); err != nil {
		log.Printf("Emergency dump failed: %v", err)
	}

	os.Exit(1)
}

// callServerStarted calls #0:server_started()
func (s *Server) callServerStarted() error {
	systemObj := s.store.Get(0)
	if systemObj == nil || systemObj.Verbs["server_started"] == nil {
		return nil
	}
	_, err := s.scheduler.CreateServerVerbTask(0, "server_started", nil, 0)
	return err
}

func (s *Server) syncPrimaryListenerPortProperty() {
	systemObj := s.store.Get(0)
	if systemObj == nil {
		return
	}
	networkProp, ok := systemObj.LookupProperty("network")
	if !ok {
		return
	}
	networkObj, ok := networkProp.Value.(types.ObjValue)
	if !ok {
		return
	}
	network := s.store.Get(networkObj.ID())
	if network == nil {
		return
	}
	portProp, ok := network.LookupProperty("port")
	if !ok {
		return
	}
	portProp.Value = types.NewInt(int64(s.connManager.GetListenPort()))
}

// callCheckpointStarted calls #0:checkpoint_started()
func (s *Server) callCheckpointStarted() error {
	systemObj := s.store.Get(0)
	if systemObj == nil || systemObj.Verbs["checkpoint_started"] == nil {
		return nil
	}
	_, err := s.scheduler.CreateServerVerbTask(0, "checkpoint_started", nil, 0)
	return err
}

// callCheckpointFinished calls #0:checkpoint_finished(success)
func (s *Server) callCheckpointFinished(success bool) error {
	systemObj := s.store.Get(0)
	if systemObj == nil || systemObj.Verbs["checkpoint_finished"] == nil {
		return nil
	}
	_, err := s.scheduler.CreateServerVerbTask(0, "checkpoint_finished", []types.Value{types.NewInt(boolToInt(success))}, 0)
	return err
}

// callShutdownStarted calls #0:shutdown_started(message)
func (s *Server) callShutdownStarted(message string) error {
	systemObj := s.store.Get(0)
	if systemObj == nil || systemObj.Verbs["shutdown_started"] == nil {
		return nil
	}
	_, err := s.scheduler.CreateServerVerbTask(0, "shutdown_started", []types.Value{types.NewStr(message)}, 0)
	return err
}

// DumpDatabase triggers an immediate checkpoint
func (s *Server) DumpDatabase() error {
	return s.checkpoint()
}

func (s *Server) appendPendingFinalizations(values []types.Value) {
	if len(values) == 0 || s.database == nil {
		return
	}

	seen := make(map[string]struct{}, len(s.database.PendingFinalizations)+len(values))
	for _, value := range s.database.PendingFinalizations {
		seen[value.String()] = struct{}{}
	}
	for _, value := range values {
		key := value.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		s.database.PendingFinalizations = append(s.database.PendingFinalizations, value)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
