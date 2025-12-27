package db

import (
	"barn/types"
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

func TestParentParsing(t *testing.T) {
	// Use the toastcore.db from cow_py
	dbPath := filepath.Join("..", "..", "cow_py", "toastcore.db")

	db, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	// Test #39 (player_db) has parent #37 (Generic Database)
	obj39 := db.Objects[39]
	if obj39 == nil {
		t.Fatal("Object #39 (player_db) not found")
	}
	if len(obj39.Parents) == 0 {
		t.Error("Object #39 has no parents")
	} else if obj39.Parents[0] != 37 {
		t.Errorf("Object #39 should have parent #37, got #%d", obj39.Parents[0])
	}

	// Test #37 (Generic Database) has parent #1
	obj37 := db.Objects[37]
	if obj37 == nil {
		t.Fatal("Object #37 (Generic Database) not found")
	}
	if len(obj37.Parents) == 0 {
		t.Error("Object #37 has no parents")
	} else if obj37.Parents[0] != 1 {
		t.Errorf("Object #37 should have parent #1, got #%d", obj37.Parents[0])
	}

	// Test #1 (root class) has no parent
	obj1 := db.Objects[1]
	if obj1 == nil {
		t.Fatal("Object #1 (root class) not found")
	}
	if len(obj1.Parents) != 0 {
		t.Errorf("Object #1 (root) should have no parents, got %d parents: %v", len(obj1.Parents), obj1.Parents)
	}
}

func TestVerbCount(t *testing.T) {
	// Use the toastcore.db from cow_py
	dbPath := filepath.Join("..", "..", "cow_py", "toastcore.db")

	db, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	// Test #37 (Generic Database) has verbs including find_exact
	obj37 := db.Objects[37]
	if obj37 == nil {
		t.Fatal("Object #37 (Generic Database) not found")
	}

	if len(obj37.VerbList) == 0 {
		t.Error("Object #37 should have verbs")
	}

	// Check if find_exact verb exists
	hasFinexact := false
	for _, verb := range obj37.VerbList {
		if verb.Names[0] == "find_exact" {
			hasFinexact = true
			break
		}
	}
	if !hasFinexact {
		t.Error("Object #37 should have find_exact verb")
	}
}

func TestVerbInheritance(t *testing.T) {
	// Use the toastcore.db from cow_py
	dbPath := filepath.Join("..", "..", "cow_py", "toastcore.db")

	db, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	store := db.NewStoreFromDatabase()

	// Test that #39 (player_db) can find find_exact verb from parent #37
	verb, _, err := store.FindVerb(types.ObjID(39), "find_exact")
	if err != nil {
		t.Errorf("Error finding verb: %v", err)
	}
	if verb == nil {
		t.Error("Failed to find inherited verb 'find_exact' on #39 (should inherit from #37)")
	} else if verb.Names[0] != "find_exact" {
		t.Errorf("Expected verb name 'find_exact', got '%s'", verb.Names[0])
	}

	// Test that the verb is actually from #37
	obj37 := db.Objects[37]
	foundOnParent := false
	for _, v := range obj37.VerbList {
		if v.Names[0] == "find_exact" {
			foundOnParent = true
			break
		}
	}
	if !foundOnParent {
		t.Error("find_exact verb not found on parent #37")
	}
}
