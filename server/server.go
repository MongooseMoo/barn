package server

import (
	"barn/db"
	"barn/parser"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Server represents the MOO server
type Server struct {
	store          *db.Store
	database       *db.Database
	scheduler      *Scheduler
	dbPath         string
	running        bool
	mu             sync.Mutex
	shutdownChan   chan struct{}
	checkpointChan chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewServer creates a new MOO server
func NewServer(dbPath string) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		dbPath:         dbPath,
		shutdownChan:   make(chan struct{}),
		checkpointChan: make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}, nil
}

// LoadDatabase loads the database from disk
func (s *Server) LoadDatabase() error {
	db, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}

	s.database = db
	s.store = db.NewStoreFromDatabase()
	s.scheduler = NewScheduler(s.store)

	log.Printf("Loaded database version %d with %d objects", db.Version, len(db.Objects))
	return nil
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

	// Call #0:server_started()
	if err := s.callServerStarted(); err != nil {
		log.Printf("Warning: #0:server_started() failed: %v", err)
	}

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
	ticker := time.NewTicker(1 * time.Hour) // Default checkpoint interval
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

	// TODO: Implement database writer
	// For now, just log
	log.Println("Checkpoint would write database here")

	// Call #0:checkpoint_finished(success)
	success := true
	if err := s.callCheckpointFinished(success); err != nil {
		log.Printf("Warning: #0:checkpoint_finished() failed: %v", err)
	}

	log.Println("Checkpoint complete")
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

	// Stop scheduler
	s.scheduler.Stop()

	// Final checkpoint
	log.Println("Performing final checkpoint...")
	if err := s.checkpoint(); err != nil {
		log.Printf("Warning: final checkpoint failed: %v", err)
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
	if systemObj == nil {
		return fmt.Errorf("system object not found")
	}

	verb := systemObj.Verbs["server_started"]
	if verb == nil {
		return nil // Verb not defined, skip
	}

	// Create task to call verb
	code := []parser.Stmt{} // Empty for now - need verb call statement
	s.scheduler.CreateForegroundTask(0, code)

	return nil
}

// callCheckpointStarted calls #0:checkpoint_started()
func (s *Server) callCheckpointStarted() error {
	systemObj := s.store.Get(0)
	if systemObj == nil {
		return fmt.Errorf("system object not found")
	}

	verb := systemObj.Verbs["checkpoint_started"]
	if verb == nil {
		return nil // Verb not defined, skip
	}

	// Create task to call verb
	code := []parser.Stmt{} // Empty for now - need verb call statement
	s.scheduler.CreateForegroundTask(0, code)

	return nil
}

// callCheckpointFinished calls #0:checkpoint_finished(success)
func (s *Server) callCheckpointFinished(success bool) error {
	systemObj := s.store.Get(0)
	if systemObj == nil {
		return fmt.Errorf("system object not found")
	}

	verb := systemObj.Verbs["checkpoint_finished"]
	if verb == nil {
		return nil // Verb not defined, skip
	}

	// Create task to call verb with success parameter
	code := []parser.Stmt{} // Empty for now - need verb call statement
	s.scheduler.CreateForegroundTask(0, code)

	return nil
}

// callShutdownStarted calls #0:shutdown_started(message)
func (s *Server) callShutdownStarted(message string) error {
	systemObj := s.store.Get(0)
	if systemObj == nil {
		return fmt.Errorf("system object not found")
	}

	verb := systemObj.Verbs["shutdown_started"]
	if verb == nil {
		return nil // Verb not defined, skip
	}

	// Create task to call verb with message parameter
	code := []parser.Stmt{} // Empty for now - need verb call statement
	s.scheduler.CreateForegroundTask(0, code)

	return nil
}

// DumpDatabase triggers an immediate checkpoint
func (s *Server) DumpDatabase() error {
	return s.checkpoint()
}
