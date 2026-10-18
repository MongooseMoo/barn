package format

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointStageAbortReleasesFilesAndPath(t *testing.T) {
	for _, phase := range []string{"begin", "invalid-store", "captured", "writer-failed", "serialized", "sync-failed", "sidecar-failed"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "abort.db")
			checkpoint, err := BeginCheckpoint(path)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "invalid-store" {
				if err := checkpoint.Capture(nil, nil, nil, nil); err == nil {
					t.Fatal("nil store accepted")
				}
			} else if phase == "writer-failed" {
				if err := checkpoint.Capture(store.NewStore(), nil, []task.Snapshot{{ID: 12, State: task.TaskSuspended}}, nil); err != nil {
					t.Fatal(err)
				}
				if err := checkpoint.Serialize(); err == nil {
					t.Fatal("lossy suspended task accepted")
				}
			} else if phase != "begin" {
				if err := checkpoint.Capture(store.NewStore(), nil, nil, nil); err != nil {
					t.Fatal(err)
				}
				if phase != "captured" {
					if err := checkpoint.Serialize(); err != nil {
						t.Fatal(err)
					}
				}
			}
			if phase == "sync-failed" {
				// Closing the actual staged file makes Sync return a native error.
				if err := checkpoint.file.Close(); err != nil {
					t.Fatal(err)
				}
				if err := checkpoint.Publish(); !errors.Is(err, os.ErrClosed) {
					t.Fatalf("sync error = %v", err)
				}
			}
			if phase == "sidecar-failed" {
				if err := os.Mkdir(path+".tmp"+waifIdentitySidecarSuffix, 0700); err != nil {
					t.Fatal(err)
				}
				if err := checkpoint.Publish(); err == nil {
					t.Fatal("sidecar creation unexpectedly succeeded")
				}
			}
			capture := checkpoint.capture
			closeErr := checkpoint.Close()
			if closeErr != nil && (phase != "sync-failed" || !errors.Is(closeErr, os.ErrClosed)) {
				t.Fatal(closeErr)
			}
			if capture != nil {
				if _, _, err := capture.Materialize(); err == nil {
					t.Fatal("aborted stage retained its store timestamp")
				}
			}
			if checkpoint.capture != nil || checkpoint.queued != nil || checkpoint.suspended != nil || checkpoint.connections != nil {
				t.Fatal("aborted stage retained owned state")
			}
			if err := checkpoint.Close(); err != nil {
				t.Fatalf("repeated Close: %v", err)
			}
			for _, suffix := range []string{".tmp", ".tmp" + waifIdentitySidecarSuffix, ".new"} {
				if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("abort retained %s: %v", suffix, err)
				}
			}
			// Reusing the exact output proves Close released its path lock.
			next, err := BeginCheckpoint(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := next.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckpointStageRejectsOutOfOrderAndRepeatedCalls(t *testing.T) {
	checkpoint, err := BeginCheckpoint(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer checkpoint.Close()
	if err := checkpoint.Publish(); err == nil {
		t.Fatal("published before serialization")
	}
	if err := checkpoint.Serialize(); err == nil {
		t.Fatal("serialized before capture")
	}
	if err := checkpoint.Capture(store.NewStore(), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Capture(store.NewStore(), nil, nil, nil); err == nil {
		t.Fatal("repeated capture accepted")
	}
	if err := checkpoint.Serialize(); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Serialize(); err == nil {
		t.Fatal("repeated serialization accepted")
	}
	if err := checkpoint.Publish(); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Publish(); err == nil {
		t.Fatal("repeated publication accepted")
	}
	if err := checkpoint.Close(); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Serialize(); err == nil {
		t.Fatal("serialized after Close")
	}
	if err := checkpoint.Publish(); err == nil {
		t.Fatal("published after Close")
	}
}

func TestCheckpointStageClosePreservesFailedPublicationRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.db")
	objectStore := store.NewStore()
	old := []types.Value{types.NewWaif(9, 3)}
	objectStore.SetPendingFinalizations(old)
	if err := WriteCheckpoint(path, objectStore, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	objectStore.SetPendingFinalizations(fresh)
	checkpoint, err := BeginCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Capture(objectStore, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Serialize(); err != nil {
		t.Fatal(err)
	}
	publicationFailed := false
	checkpoint.fs.rename = func(from, to string) error {
		if to == path+".new"+waifIdentitySidecarSuffix {
			publicationFailed = true
			return os.ErrPermission
		}
		if publicationFailed && to == path+".new" {
			return os.ErrPermission
		}
		return renameCheckpointFile(from, to)
	}
	if err := checkpoint.Publish(); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("publication error = %v", err)
	}
	if err := checkpoint.Close(); err != nil {
		t.Fatal(err)
	}
	_, generation, err := readCheckpointJournal(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	assertCheckpointIdentities(t, filepath.Join(generation, "previous"), old)
	assertCheckpointIdentities(t, filepath.Join(generation, "next"), fresh)
	// Loading reacquires the same path, repairs the failed publication and keeps
	// the saved previous pair. Close must not discard either recovery generation.
	assertCheckpointIdentities(t, path+".new", old)
}
