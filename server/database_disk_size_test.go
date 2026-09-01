package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/internal/listener"
)

func TestDatabaseDiskSizeTracksConfiguredFiles(t *testing.T) {
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
	if got != 40 {
		t.Fatalf("databaseDiskSize = %d, want input + checkpoint size 40", got)
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
	if _, err := s.databaseDiskSize(); err == nil {
		t.Fatal("databaseDiskSize succeeded without a database or checkpoint")
	}
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
