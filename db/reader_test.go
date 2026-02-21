package db

import (
	"barn/types"
	"bufio"
	"os"
	"path/filepath"
	"strings"
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

func TestReadSuspendedTasksMultipleActivations(t *testing.T) {
	// This tests the bug where a suspended task with 2 activations
	// (2 period terminators) was being parsed incorrectly.
	// The parser was stopping at the first period, not realizing
	// the task had more activations.

	// Task 1: 1 activation (top_activ_stack=0)
	// Task 2: 2 activations (top_activ_stack=1)
	// Total: 3 periods
	taskData := `2 suspended tasks
1000 1001 0
0
10
0
0 -1 0 50
language version 17
return 1;
.
1 variables
NUM
0
0
0 rt_stack slots in use
0
-111
1
1
1
1
1
1 -7 -8 1 -9 1 1 -10 1
No
More
Parse
Infos
test_verb
test_verb
6
0 0 0
1000 1002 0
0
10
0
1 0 0 50
language version 17
return 2;
.
1 variables
NUM
0
0
0 rt_stack slots in use
0
-111
1
2
1
2
1
2 -7 -8 2 -9 2 2 -10 1
No
More
Parse
Infos
outer_verb
outer_verb
6
0 0 0
language version 17
return 3;
.
1 variables
NUM
0
0
0 rt_stack slots in use
0
-111
1
2
1
2
1
2 -7 -8 2 -9 2 2 -10 1
No
More
Parse
Infos
inner_verb
inner_verb
6
0 0 0
`
	// After reading 2 suspended tasks, we should be able to read the next section
	taskData += "1 interrupted tasks\n"

	r := bufio.NewReader(strings.NewReader(taskData))
	db := &Database{
		Version: 17,
		Objects: make(map[types.ObjID]*Object),
	}

	err := db.readSuspendedTasks(r)
	if err != nil {
		t.Fatalf("readSuspendedTasks failed: %v", err)
	}

	// Now try to read what should be "1 interrupted tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read next line after suspended tasks: %v", err)
	}

	expected := "1 interrupted tasks\n"
	if line != expected {
		t.Errorf("After reading suspended tasks, expected %q but got %q", expected, line)
	}
}

func TestLoadMongooseSnapshot(t *testing.T) {
	// Test loading the mongoose7_snapshot.db
	dbPath := "mongoose7_snapshot.db"

	db, err := LoadDatabase(filepath.Join("..", dbPath))
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

	t.Logf("Loaded database version %d with %d objects", db.Version, len(db.Objects))
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

func TestResolvedPropOrderMatchesPropertyMap(t *testing.T) {
	dbPath := filepath.Join("..", "..", "cow_py", "toastcore.db")

	db, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	for objID, obj := range db.Objects {
		if obj == nil {
			continue
		}
		for i, name := range obj.PropOrder {
			if strings.HasPrefix(name, "_inherited_") {
				t.Fatalf("object #%d prop order index %d still unresolved: %q", objID, i, name)
			}
			if _, ok := obj.Properties[name]; !ok {
				t.Fatalf("object #%d prop order index %d missing property %q", objID, i, name)
			}
		}
	}
}

func TestRoundTripPreservesInheritedOverrideProperty(t *testing.T) {
	dbPath := filepath.Join("..", "..", "cow_py", "toastcore.db")

	loaded, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	store := loaded.NewStoreFromDatabase()
	beforeObj := store.Get(101)
	if beforeObj == nil {
		t.Fatal("Object #101 not found")
	}
	beforeProp, ok := beforeObj.Properties["index_cache"]
	if !ok {
		t.Fatal(`Object #101 missing property "index_cache"`)
	}
	if beforeProp.Clear {
		t.Fatal(`Expected #101.index_cache to be a local override (clear=false) before round trip`)
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "roundtrip-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()

	writer := NewWriter(tmpFile, store)
	if err := writer.WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase failed: %v", err)
	}

	reloaded, err := LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to reload round-tripped database: %v", err)
	}
	afterObj := reloaded.NewStoreFromDatabase().Get(101)
	if afterObj == nil {
		t.Fatal("Reloaded object #101 not found")
	}
	afterProp, ok := afterObj.Properties["index_cache"]
	if !ok {
		t.Fatal(`Reloaded object #101 missing property "index_cache"`)
	}

	if afterProp.Clear {
		t.Fatal(`Round trip corrupted #101.index_cache into clear=true`)
	}
	if afterProp.Owner != beforeProp.Owner || afterProp.Perms != beforeProp.Perms {
		t.Fatalf("Round trip changed owner/perms for #101.index_cache: before owner=%d perms=%d, after owner=%d perms=%d",
			beforeProp.Owner, beforeProp.Perms, afterProp.Owner, afterProp.Perms)
	}
	if (beforeProp.Value == nil) != (afterProp.Value == nil) {
		t.Fatalf("Round trip changed nil-ness of #101.index_cache value: before nil=%v after nil=%v",
			beforeProp.Value == nil, afterProp.Value == nil)
	}
	if beforeProp.Value != nil && !beforeProp.Value.Equal(afterProp.Value) {
		t.Fatalf("Round trip changed #101.index_cache value: before=%v after=%v",
			beforeProp.Value, afterProp.Value)
	}
}
