//go:build elle

package format

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// pauseEvents interleaves pause points with the filesystem calls around them.
type pauseEvents struct {
	events []string
	seen   map[string]int
}

func (p *pauseEvents) io() checkpointIO {
	return checkpointIO{
		rename: func(from, to string) error {
			p.events = append(p.events, "rename "+filepath.Base(to))
			return renameCheckpointFile(from, to)
		},
		syncDirectory: func(path string) error {
			p.events = append(p.events, "sync")
			return syncParentDirectory(path)
		},
	}
}

func (p *pauseEvents) install(t *testing.T, check func(point string, artifacts []string)) {
	t.Helper()
	p.seen = make(map[string]int)
	isolationprobe.SetPause(func(point string, artifacts []string) {
		p.events = append(p.events, point)
		p.seen[point]++
		check(point, artifacts)
	})
	t.Cleanup(func() { isolationprobe.SetPause(nil) })
}

func sameFileAs(t *testing.T, path string, want os.FileInfo) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(info, want)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestCheckpointPausePointsAtPublicationBoundaries(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "out.db")
	out, temp := path+".new", path+".tmp"
	journal := out + checkpointJournalSuffix
	publicationFixture(t, path, []types.Value{types.NewWaif(9, 3)})
	oldDatabase, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	oldSidecar, err := os.Stat(out + waifIdentitySidecarSuffix)
	if err != nil {
		t.Fatal(err)
	}

	var p pauseEvents
	p.install(t, func(point string, artifacts []string) {
		if len(artifacts) < 2 || artifacts[0] != out || artifacts[1] != temp {
			t.Errorf("%s artifacts = %v", point, artifacts)
		}
		if point == isolationprobe.CheckpointJournalPublished ||
			point == isolationprobe.CheckpointDatabaseRenamed ||
			point == isolationprobe.CheckpointPairRenamed {
			if len(artifacts) != 4 || artifacts[2] != journal || !exists(artifacts[3]) {
				t.Errorf("%s artifacts = %v", point, artifacts)
			}
		}
		switch point {
		case isolationprobe.CheckpointCapture:
			if info, err := os.Stat(temp); err != nil || info.Size() != 0 {
				t.Errorf("capture: temporary database written before Serialize: %v", err)
			}
		case isolationprobe.CheckpointSerialize:
			if info, err := os.Stat(temp); err != nil || info.Size() == 0 {
				t.Errorf("serialize: temporary database not written: %v", err)
			}
			if exists(temp + waifIdentitySidecarSuffix) {
				t.Error("serialize: sidecar prepared before Publish")
			}
		case isolationprobe.CheckpointStaged:
			if !exists(temp + waifIdentitySidecarSuffix) {
				t.Error("staged: sidecar not prepared")
			}
			if exists(journal) {
				t.Error("staged: journal published")
			}
		case isolationprobe.CheckpointJournalPublished:
			if !exists(journal) || !sameFileAs(t, out, oldDatabase) || !exists(temp) {
				t.Error("journal-published: journal missing or output database already replaced")
			}
		case isolationprobe.CheckpointDatabaseRenamed:
			if exists(temp) || sameFileAs(t, out, oldDatabase) {
				t.Error("database-renamed: output database not replaced")
			}
			if !sameFileAs(t, out+waifIdentitySidecarSuffix, oldSidecar) || !exists(temp+waifIdentitySidecarSuffix) {
				t.Error("database-renamed: sidecar already renamed")
			}
		case isolationprobe.CheckpointPairRenamed:
			if exists(temp+waifIdentitySidecarSuffix) || sameFileAs(t, out+waifIdentitySidecarSuffix, oldSidecar) {
				t.Error("pair-renamed: sidecar not renamed")
			}
			if !exists(journal) {
				t.Error("pair-renamed: journal already cleaned up")
			}
		}
	})

	checkpoint, err := BeginCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.fs = p.io()
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	objectStore := dbstore.NewStore()
	objectStore.SetPendingFinalizations(fresh)
	err = checkpoint.Capture(objectStore, nil, nil, nil)
	if err == nil {
		err = checkpoint.Serialize()
	}
	if err == nil {
		err = checkpoint.Publish()
	}
	if closeErr := checkpoint.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		isolationprobe.CheckpointCapture,
		isolationprobe.CheckpointSerialize,
		isolationprobe.CheckpointStaged,
		"sync", "sync", "rename " + filepath.Base(journal), "sync",
		isolationprobe.CheckpointJournalPublished,
		"rename " + filepath.Base(out),
		isolationprobe.CheckpointDatabaseRenamed,
		"rename " + filepath.Base(out+waifIdentitySidecarSuffix), "sync",
		isolationprobe.CheckpointPairRenamed,
		"sync", "sync",
	}
	if !slices.Equal(p.events, want) {
		t.Fatalf("events =\n%q\nwant\n%q", p.events, want)
	}
	if exists(journal) {
		t.Fatal("journal not cleaned up")
	}
	assertCheckpointIdentities(t, out, fresh)
	if p.seen[isolationprobe.CheckpointRecoveryDatabaseRestored] != 0 {
		t.Fatal("recovery point reached without recovery")
	}
}

func TestCheckpointPausePointBetweenRecoveryRestoreRenames(t *testing.T) {
	directory := t.TempDir()
	old := []types.Value{types.NewWaif(9, 3)}
	out := publicationFixture(t, filepath.Join(directory, "out"), old)
	staged := publicationFixture(t, filepath.Join(directory, "next"), []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)})
	fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
	if _, _, err := prepareCheckpointJournal(out, staged, fs); err != nil {
		t.Fatal(err)
	}
	// Interrupted after the database rename: a mixed pair with its journal.
	if err := renameCheckpointFile(staged, out); err != nil {
		t.Fatal(err)
	}
	journal := out + checkpointJournalSuffix
	mixed, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var p pauseEvents
	p.install(t, func(point string, artifacts []string) {
		if point != isolationprobe.CheckpointRecoveryDatabaseRestored {
			t.Errorf("unexpected point %s", point)
			return
		}
		if len(artifacts) != 4 || artifacts[0] != out || artifacts[1] != journal {
			t.Fatalf("artifacts = %v", artifacts)
		}
		restored := artifacts[3]
		if exists(restored) || !exists(restored+waifIdentitySidecarSuffix) || !exists(journal) {
			t.Error("recovery point not between the restore renames")
		}
		saved, err := os.ReadFile(filepath.Join(artifacts[2], "previous"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(mixed, saved) {
			t.Fatal("fixture: next and previous databases are identical")
		}
		if current, err := os.ReadFile(out); err != nil || !bytes.Equal(current, saved) {
			t.Errorf("output database not restored from the saved previous pair: %v", err)
		}
	})
	if err := recoverCheckpointPair(out, p.io()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"rename " + filepath.Base(out),
		isolationprobe.CheckpointRecoveryDatabaseRestored,
		"rename " + filepath.Base(out+waifIdentitySidecarSuffix),
		"sync", "sync",
	}
	if !slices.Equal(p.events, want) {
		t.Fatalf("events =\n%q\nwant\n%q", p.events, want)
	}
	assertCheckpointIdentities(t, out, old)
}
