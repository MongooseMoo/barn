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
