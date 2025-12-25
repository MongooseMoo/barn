package db

import (
	"path/filepath"
	"testing"
)

func TestLoadDatabase(t *testing.T) {
	// Use the toastcore.db from cow_py
	dbPath := filepath.Join("..", "..", "cow_py", "toastcore.db")

	db, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	// Basic sanity checks
	if db == nil {
		t.Fatal("Database is nil")
	}

	if db.Version != 17 {
		t.Errorf("Expected version 17, got %d", db.Version)
	}

	if len(db.Objects) == 0 {
		t.Error("No objects loaded")
	}

	// Check that #0 (system object) exists
	systemObj := db.Objects[0]
	if systemObj == nil {
		t.Fatal("System object (#0) not found")
	}

	t.Logf("Loaded database version %d with %d objects", db.Version, len(db.Objects))
	t.Logf("System object: %s", systemObj.Name)
	t.Logf("Players: %d", len(db.Players))
}
