package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/listener"
)

func TestDatabaseDiskSizeIgnoresStaleCheckpointBeforeFirstDump(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "custom.data")
	writeSizedFile(t, dbPath, 17)
	writeSizedFile(t, dbPath+".new", 23)
	writeSizedFile(t, filepath.Join(dir, "Test.db"), 1000)

	s := &Server{dbPath: dbPath}
	got, err := s.databaseDiskSize()
	if err != nil {
		t.Fatalf("databaseDiskSize: %v", err)
	}
	if got != 17 {
		t.Fatalf("databaseDiskSize = %d, want input size 17", got)
	}
}

func TestDatabaseDiskSizeKeepsRelativePathAcrossWorkingDirectoryChange(t *testing.T) {
	start := t.TempDir()
	other := t.TempDir()
	writeSizedFile(t, filepath.Join(start, "relative.db"), 31)

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(start); err != nil {
		t.Fatalf("chdir to start: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	s, err := NewServer("relative.db", []listener.Spec{{Port: 1}}, 0)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatalf("chdir away: %v", err)
	}
	writeSizedFile(t, filepath.Join(other, "relative.db"), 999)

	got, err := s.databaseDiskSize()
	if err != nil {
		t.Fatalf("databaseDiskSize: %v", err)
	}
	if got != 31 {
		t.Fatalf("databaseDiskSize = %d, want configured database size 31", got)
	}
}

func TestDatabaseDiskSizeFailsWhenBackingFilesAreUnavailable(t *testing.T) {
	s := &Server{dbPath: filepath.Join(t.TempDir(), "missing.db")}
	writeSizedFile(t, s.dbPath+".new", 23)
	if _, err := s.databaseDiskSize(); err == nil {
		t.Fatal("databaseDiskSize used a stale checkpoint without an input database")
	}
}

func TestDatabaseDiskSizeUsesCheckpointThenFallsBackToInput(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	s := &Server{
		store:       store,
		runtime:     engine.NewRuntime(store),
		connManager: NewConnectionManager(0),
		dbPath:      filepath.Join(t.TempDir(), "input.db"),
	}
	writeSizedFile(t, s.dbPath, 17)
	if err := s.checkpoint(); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := os.Stat(s.dbPath + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.databaseDiskSize(); err != nil || got != checkpoint.Size() {
		t.Fatalf("databaseDiskSize = %d, %v; want checkpoint size %d", got, err, checkpoint.Size())
	}
	if err := os.Remove(s.dbPath + ".new"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.databaseDiskSize(); err != nil || got != 17 {
		t.Fatalf("databaseDiskSize = %d, %v; want input fallback size 17", got, err)
	}
}

func TestDatabaseDiskSizeAfterFailedOrdinaryDump(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	s := &Server{
		store: store, runtime: engine.NewRuntime(store),
		connManager: NewConnectionManager(0),
		dbPath:      filepath.Join(t.TempDir(), "input.db"),
	}
	writeSizedFile(t, s.dbPath, 17)
	writeSizedFile(t, s.dbPath+".new", 23)
	if err := os.Mkdir(s.dbPath+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.checkpoint(); err == nil {
		t.Fatal("checkpoint succeeded with a directory blocking its temporary file")
	}
	if got, err := s.databaseDiskSize(); err != nil || got != 23 {
		t.Fatalf("databaseDiskSize = %d, %v; want existing output size 23 after failed dump", got, err)
	}
}

func TestDatabaseDiskSizePanicDumpDoesNotSelectStaleOrdinaryOutput(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Server{
		store: store, runtime: engine.NewRuntime(store),
		connManager: NewConnectionManager(0),
		dbPath:      filepath.Join(t.TempDir(), "input.db"),
		ctx:         ctx, cancel: cancel,
	}
	writeSizedFile(t, s.dbPath, 17)
	writeSizedFile(t, s.dbPath+".new", 23)
	if err := s.Panic("test emergency dump"); !errors.Is(err, ErrPanicShutdown) {
		t.Fatalf("Panic = %v, want ErrPanicShutdown", err)
	}
	if _, err := os.Stat(s.dbPath + ".new.PANIC"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.databaseDiskSize(); err != nil || got != 17 {
		t.Fatalf("databaseDiskSize = %d, %v; want input size 17 after panic dump", got, err)
	}
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
