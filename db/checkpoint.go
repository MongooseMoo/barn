package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DumpReason indicates why a database dump is being performed
type DumpReason int

const (
	DumpShutdown   DumpReason = iota // Server is shutting down
	DumpCheckpoint                   // Periodic checkpoint
	DumpPanic                        // Emergency dump (panic recovery)
)

func (r DumpReason) String() string {
	switch r {
	case DumpShutdown:
		return "shutdown"
	case DumpCheckpoint:
		return "checkpoint"
	case DumpPanic:
		return "panic"
	default:
		return "unknown"
	}
}

// CheckpointManager handles periodic database checkpointing
type CheckpointManager struct {
	mu         sync.Mutex
	dbPath     string // Path to main database file
	store      *Store
	generation int  // Checkpoint generation number (0, 1)
	lastSave   time.Time
	interval   time.Duration
	stopChan   chan struct{}
	doneChan   chan struct{}
}

// NewCheckpointManager creates a new checkpoint manager
func NewCheckpointManager(dbPath string, store *Store, interval time.Duration) *CheckpointManager {
	return &CheckpointManager{
		dbPath:     dbPath,
		store:      store,
		generation: 0,
		interval:   interval,
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
	}
}

// Start begins periodic checkpointing in a background goroutine
func (cm *CheckpointManager) Start() {
	if cm.interval <= 0 {
		return // Checkpointing disabled
	}
	go cm.checkpointLoop()
}

// Stop stops the checkpoint loop and waits for it to complete
func (cm *CheckpointManager) Stop() {
	if cm.interval <= 0 {
		return
	}
	close(cm.stopChan)
	<-cm.doneChan
}

// checkpointLoop runs periodic checkpoints
func (cm *CheckpointManager) checkpointLoop() {
	defer close(cm.doneChan)
	ticker := time.NewTicker(cm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case <-ticker.C:
			if err := cm.Checkpoint(DumpCheckpoint); err != nil {
				// Log error but continue
				fmt.Fprintf(os.Stderr, "Checkpoint error: %v\n", err)
			}
		}
	}
}

// Checkpoint performs a database checkpoint
// The process is:
// 1. Write to a temporary file (db.#N# where N is 0 or 1)
// 2. Remove the previous checkpoint file
// 3. Rename temp file to main database file
func (cm *CheckpointManager) Checkpoint(reason DumpReason) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	start := time.Now()

	// Generate temp filename based on reason
	var tempPath string
	if reason == DumpPanic {
		tempPath = cm.dbPath + ".PANIC"
	} else {
		tempPath = fmt.Sprintf("%s.#%d#", cm.dbPath, cm.generation)
	}

	// Write to temp file
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	writer := NewWriter(tempFile, cm.store)
	if err := writer.WriteDatabase(); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("write database: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	// Remove previous checkpoint (other generation)
	if reason != DumpPanic {
		prevGen := 1 - cm.generation
		prevPath := fmt.Sprintf("%s.#%d#", cm.dbPath, prevGen)
		os.Remove(prevPath) // Ignore error if file doesn't exist
	}

	// Atomic rename temp -> main database
	if err := atomicRename(tempPath, cm.dbPath); err != nil {
		return fmt.Errorf("rename temp to main: %w", err)
	}

	// Update state
	cm.lastSave = time.Now()
	if reason != DumpPanic {
		cm.generation = 1 - cm.generation // Toggle between 0 and 1
	}

	duration := time.Since(start)
	fmt.Printf("Checkpoint (%s) completed in %v\n", reason, duration)

	return nil
}

// LastSave returns the time of the last successful save
func (cm *CheckpointManager) LastSave() time.Time {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.lastSave
}

// atomicRename performs an atomic rename operation
// On Unix this is atomic, on Windows we need to handle existing file
func atomicRename(src, dst string) error {
	// On Windows, os.Rename fails if dst exists
	// First try direct rename (works on Unix and when dst doesn't exist)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// If that failed, try removing dst first (Windows)
	if os.Remove(dst) == nil {
		return os.Rename(src, dst)
	}

	// If dst removal failed, try backup approach
	backup := dst + ".bak"
	if os.Rename(dst, backup) == nil {
		if err := os.Rename(src, dst); err == nil {
			os.Remove(backup) // Clean up backup
			return nil
		}
		// Restore from backup if rename failed
		os.Rename(backup, dst)
	}

	return err
}

// DumpToFile writes the database to a specific file path
// This is useful for explicit dumps (e.g., -dump flag)
func (cm *CheckpointManager) DumpToFile(path string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	writer := NewWriter(f, cm.store)
	if err := writer.WriteDatabase(); err != nil {
		return fmt.Errorf("write database: %w", err)
	}

	return nil
}
