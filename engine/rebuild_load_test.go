package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// barnCheckpointOfToastImport imports the native Toast checkpoint, whose root
// frame is suspended inside Toast fork vector 0, writes the imported task as
// a Barn checkpoint, and returns that checkpoint as the reader produced it.
// Barn writes the task back in Toast's form, with the pc, operand stack,
// variables and root vector Toast itself wrote.
func barnCheckpointOfToastImport(t *testing.T) *dbformat.Database {
	t.Helper()
	database, reloaded := checkpointToastImport(t, func(rt *Runtime, _ int64) []task.Snapshot {
		_, suspended := rt.TaskSnapshots()
		return suspended
	})
	native := database.SuspendedTasks[0].Snapshot.VM
	written := reloaded.SuspendedTasks[0].Snapshot.VM
	if written.NativeRootVector != native.NativeRootVector || written.NativeFunctionID != 0 || len(written.Frames) != len(native.Frames) {
		t.Fatalf("written VM vector/func/frames = %d/%d/%d, want %d/0/%d", written.NativeRootVector, written.NativeFunctionID, len(written.Frames), native.NativeRootVector, len(native.Frames))
	}
	for index, frame := range written.Frames {
		want := native.Frames[index]
		if frame.Native == nil || frame.Rebuild != nil {
			t.Fatalf("frame %d was not written in Toast's form", index)
		}
		if frame.IP != want.IP || frame.Native.ErrorPC != want.Native.ErrorPC || len(frame.Stack) != len(want.Stack) {
			t.Fatalf("frame %d pc/error pc/stack = %d/%d/%v, Toast wrote %d/%d/%v", index, frame.IP, frame.Native.ErrorPC, frame.Stack, want.IP, want.Native.ErrorPC, want.Stack)
		}
		for slot, value := range frame.Stack {
			if !value.Equal(want.Stack[slot]) || frame.Native.OperandTypes[slot] != want.Native.OperandTypes[slot] {
				t.Fatalf("frame %d stack slot %d = %s (type %d), Toast wrote %s (type %d)", index, slot, value, frame.Native.OperandTypes[slot], want.Stack[slot], want.Native.OperandTypes[slot])
			}
		}
		// Every variable both wrote has Toast's value. The import keeps only
		// the variables the frame's own vector names, so a fork frame does
		// not get back parent variables its body never uses.
		for slot, name := range want.Program.VarNames {
			found := false
			for writtenSlot, writtenName := range frame.Program.VarNames {
				if strings.EqualFold(name, writtenName) {
					found = true
					if !frame.Locals[writtenSlot].Equal(want.Locals[slot]) {
						t.Fatalf("frame %d variable %s = %s, Toast wrote %s", index, name, frame.Locals[writtenSlot], want.Locals[slot])
					}
				}
			}
			if !found && !want.Locals[slot].IsUnbound() && !want.Locals[slot].IsNone() {
				t.Logf("frame %d: Toast wrote variable %s, which the import did not keep", index, name)
			}
		}
	}
	return reloaded
}

// checkpointToastImport imports the native Toast fixture task (not due),
// writes the snapshots capture returns as a checkpoint, and returns the
// fixture and the reloaded checkpoint.
func checkpointToastImport(t *testing.T, capture func(rt *Runtime, id int64) []task.Snapshot) (*dbformat.Database, *dbformat.Database) {
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
	if origin := rt.GetTask(saved.Snapshot.ID).PersistenceSnapshot().VM.Frames[0].Program.Origin; !origin.Native || origin.NativeVector != 0 || origin.Fork != 0 {
		t.Fatalf("imported root frame origin = %+v, want Toast fork vector 0", origin)
	}
	suspended := capture(rt, saved.Snapshot.ID)
	if len(suspended) != 1 {
		t.Fatalf("suspended snapshots = %d, want 1", len(suspended))
	}
	path := filepath.Join(t.TempDir(), "barn.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SuspendedTasks) != 1 {
		t.Fatal("checkpoint lost the imported task")
	}
	return database, reloaded
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

// Older 64-bit Toast servers wrote an indefinite suspension's start time as -1;
// Toast's reader loads a negative start time as indefinite (INTNUM_MAX), so the
// task stays suspended instead of resuming at once.
func TestLoadSuspendedTaskWithNegativeStartTimeStaysIndefinite(t *testing.T) {
	reloaded := barnCheckpointOfToastImport(t)
	store, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	saved := reloaded.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Unix(-1, 0)
	if err := rt.LoadSuspendedTasks(reloaded.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatal("task was not restored")
	}
	if !restored.WakeTime.IsZero() || !restored.StartTime.Equal(task.IndefiniteSuspendStartTime) {
		t.Fatalf("wake/start = %v/%v, want zero wake and the indefinite start sentinel", restored.WakeTime, restored.StartTime)
	}
}

// Q, 2026-11-10 ("Drop task, keep booting"): a saved task whose program is
// not what its source compiles to is dropped with an ERROR record and the
// load goes on; any other restore failure still fails the load.
func TestLoadSuspendedTasksDropsProgramMismatchOnly(t *testing.T) {
	// Checkpoints written before Barn wrote Toast's form hold Barn frames
	// that name their program by fingerprint; write one that way.
	_, reloaded := checkpointToastImport(t, func(rt *Runtime, id int64) []task.Snapshot {
		return []task.Snapshot{rt.GetTask(id).PersistenceSnapshot()}
	})
	if reloaded.SuspendedTasks[0].Snapshot.VM.Frames[0].Rebuild == nil {
		t.Fatal("checkpoint of the Barn snapshot did not write a Barn frame")
	}
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
