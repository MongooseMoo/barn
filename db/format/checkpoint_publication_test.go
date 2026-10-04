package format

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func publicationFixture(t *testing.T, path string, values []types.Value) string {
	t.Helper()
	objectStore := dbstore.NewStore()
	objectStore.SetPendingFinalizations(values)
	if err := WriteCheckpoint(path, objectStore, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	return path + ".new"
}

func assertCheckpointIdentities(t *testing.T, path string, values []types.Value) {
	t.Helper()
	loaded, err := LoadDatabase(path)
	if err != nil {
		t.Fatalf("matching checkpoint pair unavailable after publication failure: %v", err)
	}
	if len(loaded.PendingFinalizations) != len(values) {
		t.Fatalf("WAIF count = %d, want %d", len(loaded.PendingFinalizations), len(values))
	}
	for i, value := range values {
		if loaded.PendingFinalizations[i].WaifIdentity() != value.WaifIdentity() {
			t.Fatalf("WAIF identity %d changed", i)
		}
	}
}

func TestCheckpointPublicationRenameFailureRetainsMatchingGeneration(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, stage := range []string{"database", "sidecar"} {
			name := stage
			if previous {
				name += "_replace"
			} else {
				name += "_first"
			}
			t.Run(name, func(t *testing.T) {
				directory := t.TempDir()
				outPath := filepath.Join(directory, "out.db.new")
				old := []types.Value{types.NewWaif(9, 3)}
				fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
				if previous {
					publicationFixture(t, filepath.Join(directory, "out.db"), old)
					assertCheckpointIdentities(t, outPath, old)
				}
				staged := publicationFixture(t, filepath.Join(directory, "next.db"), fresh)
				failed := false
				fs := checkpointIO{rename: func(from, to string) error {
					target := outPath
					if stage == "sidecar" {
						target += waifIdentitySidecarSuffix
					}
					if to == target && !failed {
						failed = true
						return os.ErrPermission
					}
					return os.Rename(from, to)
				}, syncDirectory: syncParentDirectory}
				if err := publishCheckpointPair(outPath, staged, fs); !errors.Is(err, os.ErrPermission) {
					t.Fatalf("publication error = %v, want permission cause", err)
				}
				want := fresh
				if previous {
					want = old
				}
				assertCheckpointIdentities(t, outPath, want)
			})
		}
	}
}
