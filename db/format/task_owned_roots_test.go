package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barn/bytecode"
	"barn/db/store"
	"barn/task"
	"barn/types"
)

func TestSuspendedTaskRootsSerializeWithoutPendingPromotion(t *testing.T) {
	db := store.NewStore()
	if err := db.Add(store.NewObject(0, 0)); err != nil {
		t.Fatalf("add class object: %v", err)
	}
	if errCode := db.DefineProperty(0, "held", store.NewProperty(types.None, 0, store.PropRead|store.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define WAIF property: %v", errCode)
	}
	anonID, errCode := db.CreateObject([]types.ObjID{0}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}
	if err := db.Add(store.NewObject(9, 0)); err != nil {
		t.Fatalf("add numbered object above anonymous identity: %v", err)
	}
	waif := types.NewWaif(0, 0)
	waif.SetProperty("held", types.NewAnon(anonID))
	suspended := task.Snapshot{
		ID:            41,
		Owner:         0,
		State:         task.TaskSuspended,
		Programmer:    0,
		VerbLoc:       0,
		VerbName:      "hold",
		This:          0,
		ReadingPlayer: types.ObjNothing,
		VM: &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{
			Program: bytecode.Program{
				Source:    []string{"suspend();"},
				VarNames:  []string{"w"},
				NumLocals: 1,
			},
			Locals:     []types.Value{waif},
			This:       0,
			ThisValue:  types.NewObj(0),
			Player:     0,
			Verb:       "hold",
			StoredVerb: "hold",
			VerbLoc:    0,
		}}},
	}

	snapshot := db.SnapshotWithRoots(suspended.RootValues())
	if got := len(snapshot.PendingFinalizations); got != 0 {
		t.Fatalf("pending finalization roots = %v, want none", snapshot.PendingFinalizations)
	}
	if got := len(snapshot.AnonymousObjects); got != 1 {
		t.Fatalf("serialized anonymous objects = %d, want one task-owned object", got)
	}
	rewritten := snapshot.RewriteTaskValue(waif)
	held, ok := rewritten.GetProperty("held")
	if !ok || held.ID() != 10 {
		t.Fatalf("rewritten task-owned anonymous reference = %v, %t, want #10", held, ok)
	}

	var dumpPath string
	{
		dumpPath = filepath.Join(t.TempDir(), "panic.db")
		file, err := os.Create(dumpPath)
		if err != nil {
			t.Fatalf("create dump: %v", err)
		}
		writer := NewWriter(file, snapshot)
		writer.SetTaskSnapshots(nil, []task.Snapshot{suspended})
		if err := writer.WriteDatabase(); err != nil {
			file.Close()
			t.Fatalf("write database: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}
	}
	dumpBytes, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read database: %v", err)
	}
	dump := string(dumpBytes)
	for _, want := range []string{"0 values pending finalization", "1 suspended tasks"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("checkpoint does not contain %q", want)
		}
	}
	loaded, err := LoadDatabase(dumpPath)
	if err != nil {
		t.Fatalf("load database: %v", err)
	}
	if got := len(loaded.PendingFinalizations); got != 0 {
		t.Fatalf("loaded pending roots = %v, want none", loaded.PendingFinalizations)
	}
	if got := len(loaded.SuspendedTasks); got != 1 {
		t.Fatalf("loaded suspended tasks = %d, want one", got)
	}
	if got := len(loaded.AnonymousObjs); got != 1 {
		t.Fatalf("loaded anonymous objects = %d, want one", got)
	}
}
