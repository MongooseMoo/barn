package format

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"barn/types"
)

func startupRepairFixture(name string) string {
	return filepath.Join("testdata", name)
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

	if got := len(database.PendingFinalizations); got != 0 {
		t.Fatalf("len(PendingFinalizations) = %d, want 0", got)
	}
}

func TestLoadDatabaseRepairsBrokenFixturesAndLogs(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		snippets []string
		check    func(t *testing.T, database *Database)
	}{
		{
			name:    "broken1",
			fixture: "Broken1.db",
			snippets: []string{
				"#0.parent = #103 <invalid> ... removed",
				"#0.child = #104 <invalid> ... removed",
				"#0.location = #101 <invalid> ... fixed",
				"#0.content = #102 <invalid> ... removed",
			},
			check: func(t *testing.T, database *Database) {
				obj := database.Objects[0]
				if obj == nil {
					t.Fatal("object #0 missing")
				}
				if obj.Location() != types.ObjNothing {
					t.Fatalf("object #0 location = %d, want %d", obj.Location(), types.ObjNothing)
				}
				if containsObjID(obj.Parents(), 103) || len(obj.Children()) != 0 || len(obj.Contents()) != 0 {
					t.Fatalf("object #0 refs not repaired: parents=%v children=%v contents=%v", obj.Parents(), obj.Children(), obj.Contents())
				}
			},
		},
		{
			name:    "broken2",
			fixture: "Broken2.db",
			snippets: []string{
				"#0.parents is not an object or list of objects",
				"#0.children is not a list of objects",
				"#0.location is not an object",
				"#0.contents is not a list of objects",
				"#1.parents is not an object or list of objects",
				"#1.children is not a list of objects",
				"#1.location is not an object",
				"#1.contents is not a list of objects",
			},
		},
		{
			name:    "broken3",
			fixture: "Broken3.db",
			snippets: []string{
				"Cycle in parent chain of #0",
				"Cycle in location chain of #0",
				"Cycle in parent chain of #3",
				"Cycle in location chain of #3",
			},
		},
		{
			name:    "broken4",
			fixture: "Broken4.db",
			snippets: []string{
				"#0 not in it's location's (#2) contents",
				"#0 not in it's parent's (#1) children",
				"#3 not in it's location's (#2) contents",
				"#3 not in it's parent's (#1) children",
			},
			check: func(t *testing.T, database *Database) {
				if !containsObjID(database.Objects[2].Contents(), 0) || !containsObjID(database.Objects[2].Contents(), 3) {
					t.Fatalf("object #2 contents not repaired: %v", database.Objects[2].Contents())
				}
				if !containsObjID(database.Objects[1].Children(), 0) || !containsObjID(database.Objects[1].Children(), 3) {
					t.Fatalf("object #1 children not repaired: %v", database.Objects[1].Children())
				}
			},
		},
		{
			name:    "broken5",
			fixture: "Broken5.db",
			snippets: []string{
				"#1 not in it's child's (#0) parents",
				"#2 not in it's content's (#3) location",
			},
			check: func(t *testing.T, database *Database) {
				if !containsObjID(database.Objects[0].Parents(), 1) {
					t.Fatalf("object #0 parents not repaired: %v", database.Objects[0].Parents())
				}
				if database.Objects[3].Location() != 2 {
					t.Fatalf("object #3 location = %d, want 2", database.Objects[3].Location())
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			oldWriter := log.Writer()
			oldFlags := log.Flags()
			log.SetOutput(&buf)
			log.SetFlags(0)
			defer func() {
				log.SetOutput(oldWriter)
				log.SetFlags(oldFlags)
			}()

			database, err := LoadDatabase(startupRepairFixture(tc.fixture))
			if err != nil {
				t.Fatalf("LoadDatabase failed: %v", err)
			}
			logOutput := buf.String()
			for _, snippet := range tc.snippets {
				if !strings.Contains(logOutput, snippet) {
					t.Fatalf("log missing %q\nfull log:\n%s", snippet, logOutput)
				}
			}
			if tc.check != nil {
				tc.check(t, database)
			}
		})
	}
}
