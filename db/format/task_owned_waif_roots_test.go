package format

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointKeepsSuspendedTaskWaifAndAnonymousRootsTaskOwned(t *testing.T) {
	store := dbstore.NewStore()
	class := dbstore.NewObjectBuilder(9)
	class.SetOwner(3)
	if err := store.Add(class.Build()); err != nil {
		t.Fatalf("add WAIF class: %v", err)
	}
	anonID, errCode := store.CreateObject(nil, 3, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}
	waif := types.NewWaif(9, 3)
	waif.SetProperty("anonymous", types.NewAnon(anonID))
	suspended := task.Snapshot{
		ID:            41,
		Owner:         3,
		State:         task.TaskSuspended,
		StartTime:     time.Unix(123, 0),
		Programmer:    3,
		VerbLoc:       0,
		VerbName:      "holder",
		This:          0,
		ReadingPlayer: types.ObjNothing,
		VM: &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{
			Program: bytecode.Program{Source: []string{"suspend(60);"}, VarNames: []string{"state"}, NumLocals: 1},
			Locals:  []types.Value{types.NewList([]types.Value{waif, types.NewAnon(anonID)})},
			This:    0,
			Player:  3,
			Verb:    "holder",
			VerbLoc: 0,
		}}},
	}

	path := filepath.Join(t.TempDir(), "panic.db")
	if err := WriteCheckpoint(path, store, nil, []task.Snapshot{suspended}, nil); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	loaded, err := LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if len(loaded.PendingFinalizations) != 0 {
		t.Fatalf("pending roots = %v, want task-owned roots only", loaded.PendingFinalizations)
	}
	if len(loaded.SuspendedTasks) != 1 || len(loaded.AnonymousObjs) != 1 {
		t.Fatalf("suspended tasks = %d, anonymous objects = %d, want 1 and 1", len(loaded.SuspendedTasks), len(loaded.AnonymousObjs))
	}
	local := loaded.SuspendedTasks[0].Snapshot.VM.Frames[0].Locals[0]
	loadedWaif := local.Get(1)
	loadedAnon := local.Get(2)
	waifAnon, ok := loadedWaif.GetProperty("anonymous")
	if !ok || waifAnon.Type() != types.TYPE_ANON || !waifAnon.Equal(loadedAnon) {
		t.Fatalf("WAIF anonymous root = %v, ok=%t; direct root = %v", waifAnon, ok, loadedAnon)
	}
}
