package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// checkpointTaskOrderStore holds #0 to #3, with wizard #3 to run the setup.
func checkpointTaskOrderStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	flags := map[types.ObjID]dbstore.ObjectFlags{
		0: dbstore.FlagRead,
		1: dbstore.FlagRead,
		2: dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead,
		3: dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead,
	}
	for id := types.ObjID(0); id <= 3; id++ {
		b := dbstore.NewObjectBuilder(id)
		b.SetOwner(3)
		b.SetLocation(types.ObjNothing)
		b.SetFlags(flags[id])
		if err := store.Add(b.Build()); err != nil {
			t.Fatalf("add #%d: %v", id, err)
		}
	}
	return store
}

// Two checkpoints of the same task state are the same bytes, the WAIF
// sidecar included. Each queued, suspended and interrupted task holds its own
// WAIF, and #0.kept lists them all, so the order tasks are written in also
// decides the WAIF numbers in the object section and the sidecar.
func TestCheckpointTaskOrderIsDeterministic(t *testing.T) {
	const perKind = 8
	store := checkpointTaskOrderStore(t)
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	outcome := rt.Eval(3, []string{
		`c = create(#-1);`,
		`add_verb(c, {#3, "xd", "new"}, {"this", "none", "this"});`,
		`set_verb_code(c, "new", {"return new_waif();"});`,
		`kept = {};`,
		`for i in [1..16]`,
		`  w = c:new();`,
		`  kept = {@kept, w};`,
		`  fork (0)`,
		`    held = w;`,
		`    suspend();`,
		`  endfork`,
		`endfor`,
		`for i in [1..8]`,
		`  w = c:new();`,
		`  kept = {@kept, w};`,
		`  fork (3600)`,
		`    return w;`,
		`  endfork`,
		`endfor`,
		`add_property(#0, "kept", kept, {#3, "rw"});`,
		`return 1;`,
	})
	if outcome.Panic != nil || outcome.Result.Flow != types.FlowReturn {
		t.Fatalf("setup eval: %+v", outcome)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		queued, suspended := rt.TaskSnapshots()
		settled := len(queued) == perKind && len(suspended) == 2*perKind
		for _, s := range suspended {
			settled = settled && s.State == task.TaskSuspended
		}
		if settled {
			// Half the suspended tasks are made interrupted: blocked in read().
			for i, s := range suspended {
				if i%2 == 0 {
					rt.GetTask(s.ID).SetReadingPlayer(3)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tasks never settled: %d queued, %d suspended", len(queued), len(suspended))
		}
		rt.ProcessReadyTasks()
	}

	checkpoint := func() (database, sidecar []byte) {
		t.Helper()
		queued, suspended := rt.TaskSnapshots()
		path := filepath.Join(t.TempDir(), "task-order.db")
		if err := dbformat.WriteCheckpoint(path, store, queued, suspended, nil); err != nil {
			t.Fatalf("write checkpoint: %v", err)
		}
		database, err := os.ReadFile(path + ".new")
		if err != nil {
			t.Fatal(err)
		}
		sidecar, err = os.ReadFile(path + ".new.waifids")
		if err != nil {
			t.Fatal(err)
		}
		return database, sidecar
	}

	firstDatabase, firstSidecar := checkpoint()
	for _, header := range []string{"\n8 queued tasks\n", "\n8 suspended tasks\n", "\n8 interrupted tasks\n"} {
		if !bytes.Contains(firstDatabase, []byte(header)) {
			t.Fatalf("checkpoint has no %q section header", header[1:len(header)-1])
		}
	}

	for pass := 2; pass <= 5; pass++ {
		database, sidecar := checkpoint()
		if !bytes.Equal(database, firstDatabase) {
			at := firstDifference(database, firstDatabase)
			t.Fatalf("checkpoint %d differs from checkpoint 1 at byte %d; checkpoint 1 up to there:\n%s",
				pass, at, firstDatabase[:at])
		}
		if !bytes.Equal(sidecar, firstSidecar) {
			t.Fatalf("checkpoint %d WAIF sidecar differs from checkpoint 1 at byte %d", pass, firstDifference(sidecar, firstSidecar))
		}
	}
}

func firstDifference(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}
