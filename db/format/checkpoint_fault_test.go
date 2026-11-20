//go:build elle

package format

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

func rowView(values ...int) store.PropertyView {
	elements := make([]types.Value, len(values))
	for i, value := range values {
		elements[i] = types.NewInt(int64(value))
	}
	return store.PropertyView{Value: types.NewList(elements)}
}

func TestCheckpointRowFaultsChangeOnlyJepsenRows(t *testing.T) {
	want := map[string][]int{
		"":                            {1, 2, 3},
		"checkpoint-drop-append":      {1, 2},
		"checkpoint-duplicate-append": {1, 2, 3, 3},
	}
	expected, ok := want[isolationprobe.ActiveFault()]
	if !ok {
		t.Skipf("run without BARN_ELLE_FAULT or with a checkpoint row fault, not %q", isolationprobe.ActiveFault())
	}
	if got := checkpointRowFault("jp10000003", rowView(1, 2, 3)); !got.Value.Equal(rowView(expected...).Value) {
		t.Fatalf("jp row written as %s, want %v", got.Value, expected)
	}
	for _, name := range []string{"jepsen_rows", "description"} {
		if got := checkpointRowFault(name, rowView(1, 2, 3)); !got.Value.Equal(rowView(1, 2, 3).Value) {
			t.Fatalf("%s changed to %s", name, got.Value)
		}
	}
	if got := checkpointRowFault("jp1", rowView()); got.Value.Len() != 0 {
		t.Fatalf("empty row changed to %s", got.Value)
	}
}

// After a crash between the publication renames, the faulty recovery keeps
// the next database and deletes the old sidecar instead of restoring a pair.
func TestCheckpointMixedPairFaultKeepsDatabaseWithoutSidecar(t *testing.T) {
	if isolationprobe.ActiveFault() != "checkpoint-mixed-pair" {
		t.Skip("run with BARN_ELLE_FAULT=checkpoint-mixed-pair")
	}
	directory := t.TempDir()
	out := publicationFixture(t, filepath.Join(directory, "out"), []types.Value{types.NewWaif(9, 3)})
	staged := publicationFixture(t, filepath.Join(directory, "next"), []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)})
	fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
	if _, _, err := prepareCheckpointJournal(out, staged, fs); err != nil {
		t.Fatal(err)
	}
	next, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameCheckpointFile(staged, out); err != nil {
		t.Fatal(err)
	}
	if err := recoverCheckpointPair(out, fs); err != nil {
		t.Fatal(err)
	}
	if current, err := os.ReadFile(out); err != nil || !bytes.Equal(current, next) {
		t.Fatalf("output database is not the next database: %v", err)
	}
	if exists(out + waifIdentitySidecarSuffix) {
		t.Fatal("old sidecar kept; the fault must leave the database without one")
	}
	if exists(out + checkpointJournalSuffix) {
		t.Fatal("journal remains")
	}
}
