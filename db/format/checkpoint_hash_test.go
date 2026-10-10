package format

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// observeDatabaseHashes records every whole-database hash pass by file name.
func observeDatabaseHashes(t *testing.T) map[string]int {
	t.Helper()
	passes := make(map[string]int)
	previous := databaseHashObserver
	databaseHashObserver = func(path string) {
		if !strings.HasSuffix(path, waifIdentitySidecarSuffix) {
			passes[path]++
		}
	}
	t.Cleanup(func() { databaseHashObserver = previous })
	return passes
}

func writeWaifCheckpoint(t *testing.T, path string, values []types.Value) {
	t.Helper()
	objectStore := dbstore.NewStore()
	objectStore.SetPendingFinalizations(values)
	if err := WriteCheckpoint(path, objectStore, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func assertHashPasses(t *testing.T, passes map[string]int, want map[string]int) {
	t.Helper()
	for path, count := range passes {
		if want[path] != count {
			t.Errorf("%s hashed %d times, want %d", path, count, want[path])
		}
	}
	for path, count := range want {
		if _, seen := passes[path]; !seen && count != 0 {
			t.Errorf("%s hashed 0 times, want %d", path, count)
		}
	}
}

func TestCheckpointHashesEachGenerationOnce(t *testing.T) {
	for _, linkFails := range []bool{false, true} {
		name := "link"
		if linkFails {
			name = "copy"
		}
		t.Run(name, func(t *testing.T) {
			if linkFails {
				previous := linkCheckpointFile
				linkCheckpointFile = func(string, string) error { return errors.ErrUnsupported }
				t.Cleanup(func() { linkCheckpointFile = previous })
			}
			directory := t.TempDir()
			path := filepath.Join(directory, "out.db")
			old := []types.Value{types.NewWaif(9, 3)}
			fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}

			passes := observeDatabaseHashes(t)
			writeWaifCheckpoint(t, path, old)
			assertHashPasses(t, passes, map[string]int{path + ".tmp": 1})
			assertCheckpointIdentities(t, path+".new", old)

			passes = observeDatabaseHashes(t)
			writeWaifCheckpoint(t, path, fresh)
			assertHashPasses(t, passes, map[string]int{path + ".tmp": 1, path + ".new": 1})
			assertCheckpointIdentities(t, path+".new", fresh)
		})
	}
}

func TestFreezeCheckpointFileRejectsFileReplacedAfterHashing(t *testing.T) {
	for _, linkFails := range []bool{false, true} {
		name := "link"
		if linkFails {
			name = "copy"
		}
		t.Run(name, func(t *testing.T) {
			if linkFails {
				previous := linkCheckpointFile
				linkCheckpointFile = func(string, string) error { return errors.ErrUnsupported }
				t.Cleanup(func() { linkCheckpointFile = previous })
			}
			directory := t.TempDir()
			source := filepath.Join(directory, "source")
			if err := os.WriteFile(source, []byte("hashed generation\n"), 0600); err != nil {
				t.Fatal(err)
			}
			hashed, err := hashCheckpointFile(source)
			if err != nil {
				t.Fatal(err)
			}
			replacement := filepath.Join(directory, "replacement")
			if err := os.WriteFile(replacement, []byte("hashed generation\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, source); err != nil {
				t.Fatal(err)
			}
			if err := freezeCheckpointFile(source, filepath.Join(directory, "frozen"), hashed); err == nil {
				t.Fatal("froze a file that was replaced after it was hashed")
			}
		})
	}
}

func TestFreezeCheckpointFileCopyRejectsChangedContents(t *testing.T) {
	previous := linkCheckpointFile
	linkCheckpointFile = func(string, string) error { return errors.ErrUnsupported }
	t.Cleanup(func() { linkCheckpointFile = previous })
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	if err := os.WriteFile(source, []byte("hashed generation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hashed, err := hashCheckpointFile(source)
	if err != nil {
		t.Fatal(err)
	}
	hashed.digest[0] ^= 0xff
	frozen := filepath.Join(directory, "frozen")
	if err := freezeCheckpointFile(source, frozen, hashed); err == nil {
		t.Fatal("copied a file whose bytes do not match the hashed digest")
	}
}

func TestReadOnlyLoadRefusesPendingPublicationWithoutRecovering(t *testing.T) {
	directory := t.TempDir()
	old := []types.Value{types.NewWaif(9, 3)}
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	out := publicationFixture(t, filepath.Join(directory, "out"), old)
	staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
	publicationFailed := false
	fs := checkpointIO{rename: func(from, to string) error {
		if to == out+waifIdentitySidecarSuffix {
			publicationFailed = true
			return os.ErrPermission
		}
		if publicationFailed && to == out {
			return errors.New("injected rollback failure")
		}
		return renameCheckpointFile(from, to)
	}, syncDirectory: syncParentDirectory}
	if err := publishCheckpointPair(out, staged, fs); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("publication error = %v", err)
	}
	before, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDatabaseReadOnly(out); !errors.Is(err, ErrCheckpointRecoveryPending) {
		t.Fatalf("read-only load error = %v, want ErrCheckpointRecoveryPending", err)
	}
	after, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("read-only load changed the directory: %v -> %v", before, after)
	}
	for i := range before {
		if before[i].Name() != after[i].Name() {
			t.Fatalf("read-only load changed the directory: %v -> %v", before, after)
		}
	}
	if _, err := os.Stat(out + checkpointJournalSuffix); err != nil {
		t.Fatalf("read-only load removed the journal: %v", err)
	}

	// The owning load still recovers, and a clean database loads read-only.
	assertCheckpointIdentities(t, out, old)
	loaded, err := LoadDatabaseReadOnly(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.PendingFinalizations) != len(old) {
		t.Fatalf("WAIF count = %d, want %d", len(loaded.PendingFinalizations), len(old))
	}
}
