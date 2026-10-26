package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func loadedForkForTest(t *testing.T, saved *dbformat.QueuedTask) (*Runtime, *task.State) {
	t.Helper()
	runtime := NewRuntime(dbstore.NewStore())
	t.Cleanup(runtime.Stop)
	if err := runtime.loadQueuedTask(saved); err != nil {
		t.Fatalf("load queued task: %v", err)
	}
	loaded := runtime.taskManager.GetTask(saved.ID)
	if loaded == nil {
		t.Fatal("loaded queued task is absent")
	}
	machine, ok := loaded.BytecodeVMValue().(*vm.VM)
	if !ok || machine.CurrentFrame() == nil || machine.CurrentFrame().Program != loaded.Program {
		t.Fatal("production loader did not install the executable cached frame")
	}
	return runtime, loaded
}

func queuedForkWriterRoundtrip(t *testing.T, runtime *Runtime, loaded *task.State) *dbformat.QueuedTask {
	t.Helper()
	var buffer bytes.Buffer
	writer := dbformat.NewWriter(&buffer, runtime.store.Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{loaded.PersistenceSnapshot()}, nil)
	if err := writer.WriteDatabase(); err != nil {
		t.Fatalf("loaded queued task -> persistence snapshot -> database writer: %v", err)
	}
	path := filepath.Join(t.TempDir(), "queued-fork.db")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	database, err := dbformat.LoadDatabaseReadOnly(path)
	if err != nil {
		t.Fatalf("read actual writer output: %v", err)
	}
	if len(database.QueuedTasks) != 1 {
		t.Fatalf("roundtrip queued task count=%d, want 1", len(database.QueuedTasks))
	}
	return database.QueuedTasks[0]
}

func TestRestoredForkCanonicalEnvironmentRoundtrip(t *testing.T) {
	for _, test := range []struct {
		name, source, savedName string
	}{
		{"parent superset", "return used;", "used"},
		{"case-insensitive saved name", "return uSeD;", "USED"},
		{"mutable predefined local", "return STR;", "str"},
	} {
		t.Run(test.name, func(t *testing.T) {
			saved := &dbformat.QueuedTask{ID: 8821, StartTime: time.Now().Add(time.Hour).Unix(),
				This: 7, Player: 3, Programmer: 4, VerbLoc: 9, Verb: "restored",
				Code: []string{test.source}, Variables: map[string]types.Value{
					test.savedName: types.NewInt(73), "parent_only": types.NewInt(99), "player": types.NewObj(444),
				}}
			for checkpoint := 0; checkpoint < 2; checkpoint++ {
				runtime, loaded := loadedForkForTest(t, saved)
				frame := loaded.BytecodeVMValue().(*vm.VM).CurrentFrame()
				if len(loaded.Program.VarNames) != 1 || !frame.Locals[0].Equal(types.NewInt(73)) {
					t.Fatalf("compiled names=%v executable local=%v, want canonical saved value 73", loaded.Program.VarNames, frame.Locals[0])
				}
				previous := saved
				saved = queuedForkWriterRoundtrip(t, runtime, loaded)
				if saved.ID != previous.ID || saved.StartTime != previous.StartTime || saved.This != 7 ||
					saved.Player != 3 || saved.Programmer != 4 || saved.VerbLoc != 9 || saved.Verb != "restored" {
					t.Fatalf("task/activation metadata changed: %+v", saved)
				}
				if len(saved.Variables) != len(loaded.Program.VarNames) || !reflect.DeepEqual(saved.Code, previous.Code) {
					t.Fatalf("noncanonical serialized environment: %v", saved.Variables)
				}
				if _, retained := saved.Variables["parent_only"]; retained {
					t.Fatal("unused parent local survived checkpoint")
				}
				if _, retained := saved.Variables["player"]; retained {
					t.Fatal("unused variable named player leaked into the child environment")
				}
			}
			runtime, loaded := loadedForkForTest(t, saved)
			if err := runtime.runTask(loaded); err != nil {
				t.Fatal(err)
			}
			if loaded.Result.Flow != types.FlowReturn || !loaded.Result.Val.Equal(types.NewInt(73)) {
				t.Fatalf("eventual restored fork result=%+v, want return 73", loaded.Result)
			}
		})
	}
}

func TestRestoredForkAbsentLocalIsUnbound(t *testing.T) {
	saved := &dbformat.QueuedTask{ID: 8822, This: 0, Player: 0, Programmer: 0, VerbLoc: 0, Verb: "restored",
		Code: []string{"return missing;"}, Variables: map[string]types.Value{}}
	runtime, loaded := loadedForkForTest(t, saved)
	frame := loaded.BytecodeVMValue().(*vm.VM).CurrentFrame()
	if !frame.Locals[0].IsUnbound() {
		t.Fatalf("absent local=%v, want uninitialized", frame.Locals[0])
	}
	if err := runtime.runTask(loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Result.Flow != types.FlowException || loaded.Result.Error != types.E_VARNF {
		t.Fatalf("absent variable result=%+v, want E_VARNF", loaded.Result)
	}
}

func TestRestoredForkAbsentPredefinedLocalIsUnbound(t *testing.T) {
	saved := &dbformat.QueuedTask{ID: 8824, This: 0, Player: 0, Programmer: 0, VerbLoc: 0, Verb: "restored",
		Code: []string{"return STR;"}, Variables: map[string]types.Value{}}
	runtime, loaded := loadedForkForTest(t, saved)
	frame := loaded.BytecodeVMValue().(*vm.VM).CurrentFrame()
	if !frame.Locals[0].IsUnbound() {
		t.Fatalf("absent predefined local=%v, want native uninitialized slot", frame.Locals[0])
	}
	if err := runtime.runTask(loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Result.Flow != types.FlowException || loaded.Result.Error != types.E_VARNF {
		t.Fatalf("absent predefined result=%+v, want E_VARNF", loaded.Result)
	}
}

func TestRestoredForkRejectsAmbiguousSavedCaseWithoutQueueing(t *testing.T) {
	runtime := NewRuntime(dbstore.NewStore())
	defer runtime.Stop()
	saved := &dbformat.QueuedTask{ID: 8825, Code: []string{"return used;"},
		Variables: map[string]types.Value{"used": types.NewInt(73), "USED": types.NewInt(74)}}
	err := runtime.loadQueuedTask(saved)
	if err == nil || !strings.Contains(err.Error(), "ambiguous saved local") {
		t.Fatalf("ambiguous serialized order was not explicitly rejected: %v", err)
	}
	if runtime.taskManager.GetTask(saved.ID) != nil {
		t.Fatal("ambiguous task was queued with a guessed value")
	}
}

func TestRestoredForkCanonicalValuesDoNotAliasSavedMap(t *testing.T) {
	saved := &dbformat.QueuedTask{ID: 8826, Code: []string{"return used;"},
		Variables: map[string]types.Value{"USED": types.NewInt(73), "parent_only": types.NewInt(99)}}
	_, loaded := loadedForkForTest(t, saved)
	saved.Variables["USED"] = types.NewInt(1)
	snapshot := loaded.PersistenceSnapshot()
	frame := loaded.BytecodeVMValue().(*vm.VM).CurrentFrame()
	if len(snapshot.Fork.Variables) != 1 || !snapshot.Fork.Variables["used"].Equal(types.NewInt(73)) ||
		!frame.Locals[0].Equal(snapshot.Fork.Variables["used"]) {
		t.Fatalf("canonical frame/snapshot changed with saved map: %+v", snapshot.Fork)
	}
}

func TestRestoredForkAbsentLocalRoundtripPreservesUnbound(t *testing.T) {
	saved := &dbformat.QueuedTask{ID: 8823, This: 0, Player: 0, Programmer: 0, VerbLoc: 0, Verb: "restored",
		Code: []string{"return `missing ! E_VARNF => 91';"}, Variables: map[string]types.Value{}}
	for checkpoint := 0; checkpoint < 2; checkpoint++ {
		runtime, loaded := loadedForkForTest(t, saved)
		saved = queuedForkWriterRoundtrip(t, runtime, loaded)
	}
	runtime, loaded := loadedForkForTest(t, saved)
	frame := loaded.BytecodeVMValue().(*vm.VM).CurrentFrame()
	if !frame.Locals[0].IsUnbound() {
		t.Fatalf("checkpoint changed absent variable to %v", frame.Locals[0])
	}
	if err := runtime.runTask(loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Result.Flow != types.FlowReturn || !loaded.Result.Val.Equal(types.NewInt(91)) {
		t.Fatalf("absent variable after two checkpoints=%+v, want E_VARNF caught as 91", loaded.Result)
	}
}
