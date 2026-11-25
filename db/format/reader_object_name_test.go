package format

import (
	"bufio"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Object names whose meaningful bytes are spaces or a carriage return, as a
// MOO program can set them (issue #1137: `o.name = "   "`).
var spacedObjectNames = []string{"   ", "a ", " n ", "\tx\t", "r\r", ""}

// The writer writes an object name followed by a newline, and Toast's
// dbio_read_string (db_io.cc) removes only that newline. Every checkpoint
// Barn writes must read back with the same names, as Toast reads it.
func TestObjectNamesKeepSpacesAcrossCheckpoints(t *testing.T) {
	s := store.NewStore()
	for i, name := range spacedObjectNames {
		obj := store.NewObjectBuilder(types.ObjID(i))
		obj.SetName(name)
		obj.SetOwner(0)
		obj.SetLocation(types.ObjNothing)
		if err := s.Add(obj.Build()); err != nil {
			t.Fatal(err)
		}
	}
	current := s
	for round := 1; round <= 3; round++ {
		reloaded, err := dumpAndReload(current)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		for i, want := range spacedObjectNames {
			obj, ok := reloaded.Get(types.ObjID(i))
			if !ok {
				t.Fatalf("round %d: #%d missing", round, i)
			}
			if got := obj.Name; got != want {
				t.Fatalf("round %d: #%d name = %q, want %q", round, i, got, want)
			}
		}
		if err := CompareRoundTripStores(s, reloaded); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		current = reloaded
	}
}

// The v4 object reader keeps the name line's bytes the same way.
func TestV4ObjectNameKeepsSpaces(t *testing.T) {
	raw := "** LambdaMOO Database, Format Version 4 **\n1\n0\n0\n0\n" +
		"#0\n a b  \n\n0\n-1\n-1\n-1\n-1\n-1\n-1\n-1\n0\n0\n0\n" +
		"0 clocks\n0 queued tasks\n0 suspended tasks\n"
	database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(strings.NewReader(raw)), nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := captureDatabase(database)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.snapshot.Objects[0].Name; got != " a b  " {
		t.Fatalf("v4 name = %q, want %q", got, " a b  ")
	}
}
