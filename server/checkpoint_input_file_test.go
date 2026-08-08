package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	runtime "github.com/MongooseMoo/barn/scheduler"
)

// TestCheckpointDoesNotModifyInputDB verifies that a checkpoint writes only to
// <dbPath>.new and never overwrites the original input database file.
func TestCheckpointDoesNotModifyInputDB(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)

	dbPath := filepath.Join(t.TempDir(), "input.db")
	sentinel := []byte("sentinel-do-not-modify")
	if err := os.WriteFile(dbPath, sentinel, 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	s := &Server{
		store:              store,
		scheduler:          runtime.NewScheduler(store),
		input:              NewInputProcessor(store, runtime.NewScheduler(store)),
		connManager:        NewConnectionManager(0),
		dbPath:             dbPath,
		checkpointInterval: time.Second,
	}

	if err := s.checkpoint(); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read input db: %v", err)
	}
	if !bytes.Equal(got, sentinel) {
		t.Fatal("input DB was modified by checkpoint; only <db>.new should be written")
	}

	if _, err := os.Stat(dbPath + ".new"); err != nil {
		t.Fatalf("<db>.new not created: %v", err)
	}
}
