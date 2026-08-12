package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestWriteCheckpointWritesOnlyToNewFile(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "checkpoint.db")
	if err := WriteCheckpoint(path, loaded.NewStoreFromDatabase(), nil, nil, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}

	// Input path must not be created or modified.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("input path %s should not exist after checkpoint", path)
	}

	// Output path+".new" must be written and loadable.
	if _, err := LoadDatabase(path + ".new"); err != nil {
		t.Fatalf("load output checkpoint: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary checkpoint %s should be renamed away", path+".tmp")
	}
}

func TestWriteCheckpointPreservesWaifIdentityWithoutChangingPortableDump(t *testing.T) {
	objectStore := dbstore.NewStore()
	waif := types.NewWaif(9, 3)
	other := types.NewWaif(9, 3)
	objectStore.SetPendingFinalizations([]types.Value{waif, other})
	path := filepath.Join(t.TempDir(), "waif-identity.db")
	if err := WriteCheckpoint(path, objectStore, nil, nil, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	raw, err := os.ReadFile(path + ".new")
	if err != nil {
		t.Fatalf("read portable checkpoint: %v", err)
	}
	if !strings.Contains(string(raw), "13\nc 0\n") || !strings.Contains(string(raw), "13\nc 1\n") {
		t.Fatalf("checkpoint changed portable WAIF marker:\n%s", raw)
	}
	if strings.Contains(string(raw), waif.WaifIdentity().String()) {
		t.Fatal("portable database contains Barn-private WAIF identity")
	}

	reloaded, err := LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("LoadDatabase: %v", err)
	}
	if got := reloaded.PendingFinalizations[0].WaifIdentity(); got != waif.WaifIdentity() {
		t.Fatalf("identity after checkpoint = %s, want %s", got, waif.WaifIdentity())
	}
	if got := reloaded.PendingFinalizations[1].WaifIdentity(); got != other.WaifIdentity() {
		t.Fatalf("second identity after checkpoint = %s, want %s", got, other.WaifIdentity())
	}
	if reloaded.PendingFinalizations[0].WaifIdentity() == reloaded.PendingFinalizations[1].WaifIdentity() {
		t.Fatal("distinct WAIFs became identical after checkpoint")
	}
}

func TestWriteCheckpointPreservesAnonymousGraphsRootedOnlyBySuspendedTasks(t *testing.T) {
	objectStore := dbstore.NewStore()
	for _, builder := range []*dbstore.ObjectBuilder{
		dbstore.NewObjectBuilder(0),
		dbstore.NewObjectBuilder(2),
	} {
		builder.SetOwner(2)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser)
		if err := objectStore.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", builder.ID(), err)
		}
	}

	anonA, errCode := objectStore.CreateObject(nil, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous A: %v", errCode)
	}
	anonB, errCode := objectStore.CreateObject(nil, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous B: %v", errCode)
	}
	for _, definition := range []struct {
		id     types.ObjID
		marker string
		next   types.ObjID
	}{
		{id: anonA, marker: "A payload", next: anonB},
		{id: anonB, marker: "B payload", next: anonA},
	} {
		if errCode := objectStore.DefineProperty(definition.id, "marker", dbstore.NewProperty(types.NewStr(definition.marker), 2, dbstore.PropRead, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.marker: %v", definition.id, errCode)
		}
		if errCode := objectStore.DefineProperty(definition.id, "next", dbstore.NewProperty(types.NewAnon(definition.next), 2, dbstore.PropRead, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.next: %v", definition.id, errCode)
		}
	}

	suspended := make([]task.Snapshot, 0, 2)
	for index, held := range []types.Value{types.NewAnon(anonA), types.NewAnon(anonB)} {
		suspended = append(suspended, task.Snapshot{
			ID:            int64(index + 1),
			Owner:         2,
			State:         task.TaskSuspended,
			WakeValue:     types.NewInt(0),
			TaskLocal:     types.NewEmptyMap(),
			Programmer:    2,
			This:          0,
			ReadingPlayer: types.ObjNothing,
			VM: &task.VMSnapshot{
				MaxStackDepth: 50,
				Frames: []task.VMFrameSnapshot{{
					Program: bytecode.Program{
						Code:      []byte{byte(bytecode.OP_RETURN_NONE)},
						Source:    []string{"return;"},
						VarNames:  []string{"held"},
						NumLocals: 1,
						LineInfo:  []bytecode.LineEntry{{StartIP: 0, Line: 1}},
					},
					Locals:    []types.Value{held},
					This:      0,
					ThisValue: types.NewObj(0),
					Player:    2,
				}},
			},
		})
	}

	path := filepath.Join(t.TempDir(), "task-rooted-anonymous.db")
	if err := WriteCheckpoint(path, objectStore, nil, suspended, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}
	raw, err := os.ReadFile(path + ".new")
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if output := string(raw); !strings.Contains(output, "0 values pending finalization") || !strings.Contains(output, "2 suspended tasks") || !strings.Contains(output, "0 interrupted tasks") {
		t.Fatalf("checkpoint task classification changed: pending/suspended/interrupted headers missing")
	}
	reloaded, err := LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	if got := len(reloaded.PendingFinalizations); got != 0 {
		t.Fatalf("pending finalizations = %d, want 0", got)
	}
	if got := len(reloaded.SuspendedTasks); got != 2 {
		t.Fatalf("suspended tasks = %d, want 2", got)
	}
	if got := len(reloaded.AnonymousObjs); got != 2 {
		t.Fatalf("anonymous objects = %d, want both task-rooted cycle members", got)
	}

	emitted := make(map[types.ObjID]struct{}, len(reloaded.AnonymousObjs))
	for _, builder := range reloaded.AnonymousObjs {
		emitted[builder.ID()] = struct{}{}
	}
	heldIDs := make(map[types.ObjID]struct{}, 2)
	for _, suspendedTask := range reloaded.SuspendedTasks {
		held := suspendedTask.Snapshot.VM.Frames[0].Locals[0]
		if held.Type() != types.TYPE_ANON {
			t.Fatalf("task %d held value = %v, want anonymous reference", suspendedTask.Snapshot.ID, held)
		}
		if _, ok := emitted[held.ID()]; !ok {
			t.Fatalf("task %d held anonymous id #%d, not an emitted serialization id %v", suspendedTask.Snapshot.ID, held.ID(), emitted)
		}
		heldIDs[held.ID()] = struct{}{}
	}
	if len(heldIDs) != 2 {
		t.Fatalf("task-held anonymous ids = %v, want both emitted cycle members", heldIDs)
	}

	reloadedStore := reloaded.NewStoreFromDatabase()
	markers := make(map[string]struct{}, 2)
	for id := range heldIDs {
		marker, errCode := reloadedStore.PropertyValue(id, "marker")
		if errCode != types.E_NONE || marker.Type() != types.TYPE_STR {
			t.Fatalf("#%d.marker = %v, err=%v", id, marker, errCode)
		}
		markers[marker.Str()] = struct{}{}
		next, errCode := reloadedStore.PropertyValue(id, "next")
		if errCode != types.E_NONE || next.Type() != types.TYPE_ANON {
			t.Fatalf("#%d.next = %v, err=%v", id, next, errCode)
		}
		if _, ok := heldIDs[next.ID()]; !ok {
			t.Fatalf("#%d.next = %v, want the other emitted cycle member", id, next)
		}
	}
	if _, ok := markers["A payload"]; !ok {
		t.Fatalf("markers = %v, missing A payload", markers)
	}
	if _, ok := markers["B payload"]; !ok {
		t.Fatalf("markers = %v, missing B payload", markers)
	}
}
