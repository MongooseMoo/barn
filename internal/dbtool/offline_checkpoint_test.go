package dbtool

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// writeTaskAndWaifCheckpoint builds a checkpoint holding a WAIF in a property, a
// queued fork, a task suspended with a WAIF in a local, and one connection
// record, the way a running server writes one. It returns the checkpoint path.
func writeTaskAndWaifCheckpoint(t *testing.T) string {
	t.Helper()
	store := evalStore(t)
	session, err := NewEvalSession(store, config.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close eval session: %v", err)
		}
	}()
	outcome := session.runtime.Eval(3, []string{
		`c = create(#-1);`,
		`add_verb(c, {#3, "xd", "new"}, {"this", "none", "this"});`,
		`set_verb_code(c, "new", {"return new_waif();"});`,
		`add_property(#0, "kept", c:new(), {#3, "rw"});`,
		`w = c:new();`,
		`fork (0)`,
		`  held = w;`,
		`  suspend();`,
		`endfork`,
		`fork (3600)`,
		`  return 1;`,
		`endfork`,
		`return 1;`,
	})
	if !completed(outcome) || outcome.Result.Flow != types.FlowReturn {
		t.Fatalf("setup eval: %+v", outcome)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		queued, suspended := session.runtime.TaskSnapshots()
		if len(queued) == 1 && len(suspended) == 1 && suspended[0].State == task.TaskSuspended {
			source := filepath.Join(t.TempDir(), "source.db")
			connections := []dbformat.ActiveConnection{{Player: 3, Listener: 0}}
			if err := dbformat.WriteCheckpoint(source, store, queued, suspended, connections); err != nil {
				t.Fatalf("write source checkpoint: %v", err)
			}
			return source + ".new"
		}
		if time.Now().After(deadline) {
			t.Fatalf("tasks never settled: %d queued, %d suspended", len(queued), len(suspended))
		}
		session.runtime.ProcessReadyTasks()
	}
}

func suspendedLocalIsWaif(snapshot task.Snapshot) bool {
	for _, frame := range snapshot.VM.Frames {
		for _, local := range frame.Locals {
			if local.Type() == types.TYPE_WAIF {
				return true
			}
		}
	}
	return false
}

func TestOfflineCheckpointKeepsTasksWaifsAndIsStable(t *testing.T) {
	source := writeTaskAndWaifCheckpoint(t)
	options := config.DefaultOptions()

	var out bytes.Buffer
	if err := WriteOfflineCheckpoint(&out, source, options, false, ""); err != nil {
		t.Fatalf("first offline checkpoint: %v", err)
	}
	if !strings.Contains(out.String(), "wrote "+source+".new: 1 queued, 1 suspended or interrupted tasks, 1 connections") {
		t.Fatalf("first offline checkpoint output = %q", out.String())
	}
	first := source + ".new"
	loaded, err := dbformat.LoadDatabaseReadOnly(first)
	if err != nil {
		t.Fatalf("reload first checkpoint: %v", err)
	}
	if len(loaded.QueuedTasks) != 1 || len(loaded.SuspendedTasks) != 1 || len(loaded.ActiveConnections) != 1 {
		t.Fatalf("reloaded %d queued, %d suspended, %d connections; want 1, 1, 1",
			len(loaded.QueuedTasks), len(loaded.SuspendedTasks), len(loaded.ActiveConnections))
	}
	if !suspendedLocalIsWaif(loaded.SuspendedTasks[0].Snapshot) {
		t.Fatal("suspended task lost its WAIF local")
	}
	reloadedStore, err := loaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatalf("construct reloaded store: %v", err)
	}
	if kept, errCode := reloadedStore.DirectTxn().PropertyValue(0, "kept"); errCode != types.E_NONE || kept.Type() != types.TYPE_WAIF {
		t.Fatalf("#0.kept = %v (%v), want a WAIF", kept, errCode)
	}
	if _, err := os.Stat(first + ".waifids"); err != nil {
		t.Fatalf("WAIF sidecar: %v", err)
	}

	if err := WriteOfflineCheckpoint(io.Discard, first, options, false, ""); err != nil {
		t.Fatalf("second offline checkpoint: %v", err)
	}
	for _, suffix := range []string{"", ".waifids"} {
		a, err := os.ReadFile(first + suffix)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(first + ".new" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("second pass %q differs from first pass", suffix)
		}
	}
}

// With a program to run, every task due within a second runs (a suspend(0)
// resumes) and the checkpoint holds what is still waiting.
func TestOfflineCheckpointRunsProgramUntilNoTaskIsDue(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	if err := dbformat.WriteCheckpoint(source, evalStore(t), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(dir, "world.moo")
	if err := os.WriteFile(program, []byte(`add_property(#0, "made", 0, {#3, "rw"});
suspend(0);
#0.made = 7;
fork (3600)
  return 1;
endfork
suspend();
return 1;
`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WriteOfflineCheckpoint(&out, source+".new", config.DefaultOptions(), false, program); err != nil {
		t.Fatalf("offline checkpoint: %v", err)
	}
	if !strings.Contains(out.String(), ".new: 1 queued, 1 suspended or interrupted tasks, 0 connections") {
		t.Fatalf("output = %q", out.String())
	}
	loaded, err := dbformat.LoadDatabaseReadOnly(source + ".new.new")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(loaded.QueuedTasks) != 1 || len(loaded.SuspendedTasks) != 1 {
		t.Fatalf("reloaded %d queued, %d suspended; want 1, 1", len(loaded.QueuedTasks), len(loaded.SuspendedTasks))
	}
	store, err := loaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if made, errCode := store.DirectTxn().PropertyValue(0, "made"); errCode != types.E_NONE || !made.Equal(types.NewInt(7)) {
		t.Fatalf("#0.made = %v (%v), want 7", made, errCode)
	}
}

func TestOfflineCheckpointOmitSuspendedKeepsQueuedTasks(t *testing.T) {
	source := writeTaskAndWaifCheckpoint(t)
	if err := WriteOfflineCheckpoint(io.Discard, source, config.DefaultOptions(), true, ""); err != nil {
		t.Fatalf("offline checkpoint: %v", err)
	}
	loaded, err := dbformat.LoadDatabaseReadOnly(source + ".new")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(loaded.QueuedTasks) != 1 || len(loaded.SuspendedTasks) != 0 || len(loaded.ActiveConnections) != 1 {
		t.Fatalf("reloaded %d queued, %d suspended, %d connections; want 1, 0, 1",
			len(loaded.QueuedTasks), len(loaded.SuspendedTasks), len(loaded.ActiveConnections))
	}
	reloadedStore, err := loaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatalf("construct reloaded store: %v", err)
	}
	if kept, errCode := reloadedStore.DirectTxn().PropertyValue(0, "kept"); errCode != types.E_NONE || kept.Type() != types.TYPE_WAIF {
		t.Fatalf("#0.kept = %v (%v), want a WAIF", kept, errCode)
	}
}
