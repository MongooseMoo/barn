package db

import (
	"path/filepath"
	"testing"
)

func startupRepairFixture(name string) string {
	return filepath.Join("..", "..", "mongoose", "toaststunt", "test", "tests", name)
}

func TestLoadDatabaseSupportsFormat5Fixtures(t *testing.T) {
	database, err := LoadDatabase(startupRepairFixture("Broken1.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	if database.Version != 5 {
		t.Fatalf("Version = %d, want 5", database.Version)
	}
	if len(database.Objects) != 4 {
		t.Fatalf("len(Objects) = %d, want 4", len(database.Objects))
	}
	if database.Objects[0] == nil {
		t.Fatal("object #0 missing")
	}
}

func TestLoadDatabaseReadsPendingFinalizations(t *testing.T) {
	database, err := LoadDatabase(startupRepairFixture("Anon6.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	if got := len(database.PendingFinalizations); got != 1 {
		t.Fatalf("len(PendingFinalizations) = %d, want 1", got)
	}
}
