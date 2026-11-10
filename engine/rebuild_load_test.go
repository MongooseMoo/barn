package engine

import (
	"path/filepath"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// barnCheckpointOfToastImport imports the native Toast checkpoint, whose root
// frame is suspended inside Toast fork vector 0, writes the imported task as
// a Barn checkpoint, and returns that checkpoint as the reader produced it.
func barnCheckpointOfToastImport(t *testing.T) *dbformat.Database {
	t.Helper()
	database := loadNativeImportFixture(t, "testdata/toast-suspended-call.db", "51158bee3d37e0b59225611e3a138dd274261e4c5993a8846f73fc74d229f0d8")
	if got := database.SuspendedTasks[0].Snapshot.VM.NativeRootVector; got != 0 {
		t.Fatalf("fixture root vector = %d, want fork vector 0", got)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	saved := database.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Now().Add(time.Hour) // not due: stays suspended
	if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	queued, suspended := rt.TaskSnapshots()
	if len(suspended) != 1 {
		t.Fatalf("suspended snapshots = %d, want 1", len(suspended))
	}
	if origin := suspended[0].VM.Frames[0].Program.Origin; !origin.Native || origin.NativeVector != 0 || origin.Fork != 0 {
		t.Fatalf("imported root frame origin = %+v, want Toast fork vector 0", origin)
	}
	path := filepath.Join(t.TempDir(), "barn.db")
	if err := dbformat.WriteCheckpoint(path, store, queued, suspended, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SuspendedTasks) != 1 || reloaded.SuspendedTasks[0].Snapshot.VM.Frames[0].Rebuild == nil {
		t.Fatal("Barn checkpoint lost the imported task or wrote its bytecode")
	}
	return reloaded
}

// A Toast task suspended in a fork body, imported, then checkpointed by
// Barn, is rebuilt with the same native-vector compile and resumes exactly
// as the direct import does (TestToastSuspendedCallImportResumesExactlyOnce).
func TestToastForkVectorTaskSurvivesBarnCheckpoint(t *testing.T) {
	reloaded := barnCheckpointOfToastImport(t)
	store, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	saved := reloaded.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	if err := rt.LoadSuspendedTasks(reloaded.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatal("Barn checkpoint of the imported task was not restored")
	}
	if origin := restored.VM().Frames[0].Program.Origin; !origin.Native || origin.NativeVector != 0 {
		t.Fatalf("rebuilt root frame origin = %+v", origin)
	}
	driveNativeImportedContinuation(t, rt, restored)
	object, _ := store.DirectTxn().PropertyValue(1, "conformance_suspended_call_checkpoint")
	read := func(name string) int64 {
		value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
		if code != types.E_NONE || value.Type() != types.TYPE_INT {
			t.Fatalf("%s=%s error=%s", name, value, code)
		}
		return value.Int()
	}
	if before, after, result, finished := read("before"), read("after"), read("result"), read("finished"); before != 1 || after != 1 || result != 173 || finished != 1 {
		t.Fatalf("resumed before/after/result/finished=%d/%d/%d/%d, want 1/1/173/1", before, after, result, finished)
	}
}

// Q, 2026-11-10 ("Drop task, keep booting"): a saved task whose program is
// not what its source compiles to is dropped with an ERROR record and the
// load goes on; any other restore failure still fails the load.
func TestLoadSuspendedTasksDropsProgramMismatchOnly(t *testing.T) {
	reloaded := barnCheckpointOfToastImport(t)
	store, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	mismatched := reloaded.SuspendedTasks[0]
	mismatched.Snapshot.VM.Frames[0].Rebuild.Fingerprint[0] ^= 1

	var loadError error
	records := captureRecords(t, func() { loadError = rt.LoadSuspendedTasks(reloaded.SuspendedTasks) })
	if loadError != nil {
		t.Fatalf("a program mismatch failed the load: %v", loadError)
	}
	if rt.GetTask(mismatched.Snapshot.ID) != nil {
		t.Fatal("mismatched task was restored")
	}
	if len(records) != 1 || records[0]["level"] != "ERROR" || records[0]["task_id"] != float64(mismatched.Snapshot.ID) ||
		records[0]["verb"] != mismatched.Snapshot.VerbName || records[0]["reason"] == nil {
		t.Fatalf("refusal records = %v", records)
	}

	malformed := []*dbformat.SuspendedTask{{Snapshot: task.Snapshot{ID: 99, VM: &task.VMSnapshot{}}}}
	if err := rt.LoadSuspendedTasks(malformed); err == nil {
		t.Fatal("a malformed task record did not fail the load")
	}
}
