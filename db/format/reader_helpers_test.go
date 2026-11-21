package format

import (
	"bufio"
	"slices"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A name an object defines again over its ancestor's definition names only
// the first slot; the shadowed slot keeps its position under a placeholder.
func TestDistinctSlotNamesRenamesShadowedSlots(t *testing.T) {
	got := distinctSlotNames([]string{"0", "0", "Name", "name", "_inherited_4"})
	want := []string{"0", "_inherited_1", "Name", "_inherited_3", "_inherited_4"}
	if !slices.Equal(got, want) {
		t.Fatalf("distinctSlotNames = %q, want %q", got, want)
	}
	names := []string{"a", "b"}
	if got := distinctSlotNames(names); !slices.Equal(got, names) {
		t.Fatalf("distinct names changed: %q", got)
	}
}

// A loaded object that defines a name its parent also defines keeps every slot
// at its position through each checkpoint: the shadowed parent slot is written
// back under its placeholder at its own position, not after the later slots.
func TestShadowedSlotKeepsPositionAcrossCheckpoints(t *testing.T) {
	// #1 defines "a" over #0's "a" and "b"; its slots hold 10 ("a"), 11 (the
	// shadowed "a") and 12 ("b").
	raw := "** LambdaMOO Database, Format Version 4 **\n2\n0\n0\n0\n" +
		"#0\nroot\n\n0\n-1\n-1\n-1\n-1\n-1\n1\n-1\n0\n2\na\nb\n2\n" +
		"0\n1\n-1\n5\n0\n2\n-1\n5\n" +
		"#1\nchild\n\n0\n-1\n-1\n-1\n-1\n0\n-1\n-1\n0\n1\na\n3\n" +
		"0\n10\n-1\n5\n0\n11\n-1\n5\n0\n12\n-1\n5\n" +
		"0 clocks\n0 queued tasks\n0 suspended tasks\n"
	database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(strings.NewReader(raw)), nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := captureDatabase(database)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"a": 10, "_inherited_1": 11, "b": 12}
	for round := 0; round < 3; round++ {
		child := state.snapshot.Objects[1]
		if names := state.snapshot.PropertyNames[1]; !slices.Equal(names, []string{"a", "_inherited_1", "b"}) {
			t.Fatalf("round %d: slot names %q", round, names)
		}
		for name, value := range want {
			prop, ok := child.Properties[store.PropertyNameKey(name)]
			if !ok || prop.Value != types.NewInt(value) {
				t.Fatalf("round %d: slot %q = %+v (present %v), want %d", round, name, prop, ok, value)
			}
		}
		reloaded, err := writeAndParse(state, nil)
		if err != nil {
			t.Fatal(err)
		}
		if state, err = captureDatabase(reloaded); err != nil {
			t.Fatal(err)
		}
	}
}
