package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A loaded anonymous object held only by a numbered object's last_move is
// persistently reachable: the collector that runs after every task must not
// recycle it, and the next checkpoint must still write it. This is the running
// server path of conformance last_move_anon_checkpoint (an offline dump runs no
// task, so no collector, and kept the object).
func TestLastMoveOnlyAnonymousSurvivesTaskCollectorAndCheckpoint(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	class, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create class: %v", errCode)
	}

	const anonID types.ObjID = 100
	anon := dbstore.NewObjectBuilder(anonID)
	anon.SetAnonymous(true)
	anon.SetName("last_move_anon")
	anon.SetOwner(0)
	anon.SetLocation(types.ObjNothing)
	anon.SetParents([]types.ObjID{class})
	store.AddAnonymous(anon.Build())

	holderID := store.NextID()
	holder := dbstore.NewObjectBuilder(holderID)
	holder.SetOwner(0)
	holder.SetLocation(types.ObjNothing)
	holder.SetParents([]types.ObjID{0})
	holder.SetLastMove(types.NewMap([][2]types.Value{
		{types.NewStr("source"), types.NewAnon(anonID)},
		{types.NewStr("time"), types.NewInt(0)},
	}))
	if err := store.Add(holder.Build()); err != nil {
		t.Fatalf("add holder: %v", err)
	}

	rt := NewRuntime(store)
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)
	runEvalTask := func(code string) types.Value {
		t.Helper()
		program := compileTestProgram(t, rt.registry, code)
		taskID := rt.CreateBackgroundTask(0, program, 0)
		running := rt.GetTask(taskID)
		running.Context.IsWizard = true
		for pass := 0; pass < 8; pass++ {
			state := running.GetState()
			if state == task.TaskCompleted || state == task.TaskKilled {
				break
			}
			if processed := rt.ProcessReadyTasks(); processed == 0 {
				t.Fatalf("eval task %d made no progress in state %v", taskID, state)
			}
		}
		if state := running.GetState(); state != task.TaskCompleted {
			t.Fatalf("eval task %d ended in state %v, want completed", taskID, state)
		}
		if running.Result.Flow != types.FlowReturn {
			t.Fatalf("eval task %d result = %+v, want return", taskID, running.Result)
		}
		return running.Result.Val
	}

	// Any finished task runs the orphan-anonymous collector over the store.
	runEvalTask("return 1;")
	probe := fmt.Sprintf(`s = #%d.last_move["source"]; return {typeof(s), valid(s)};`, holderID)
	for pass := 1; pass <= 2; pass++ {
		if got := runEvalTask(probe).String(); got != "{12, 1}" {
			t.Fatalf("probe %d: last_move source = %s, want {12, 1}", pass, got)
		}
	}

	path := filepath.Join(t.TempDir(), "last-move-anon.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, nil, nil); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}
	raw, err := os.ReadFile(path + ".new")
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if !strings.Contains(string(raw), "last_move_anon") {
		t.Fatalf("checkpoint after a task does not contain the anonymous object held only by last_move")
	}
}
