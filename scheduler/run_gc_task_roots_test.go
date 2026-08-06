package scheduler

import (
	"testing"

	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

func TestExplicitRunGCPreservesAnonymousCycleHeldBySuspendedSiblingVMs(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	anonA, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous A: %v", errCode)
	}
	anonB, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous B: %v", errCode)
	}
	if errCode := store.DefineProperty(anonA, "next", dbstore.NewProperty(types.NewAnon(anonB), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}
	if errCode := store.DefineProperty(anonB, "next", dbstore.NewProperty(types.NewAnon(anonA), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define B.next: %v", errCode)
	}
	for name, value := range map[string]types.Value{
		"hold_left":  types.NewAnon(anonA),
		"hold_right": types.NewAnon(anonB),
	} {
		if errCode := store.DefineProperty(0, name, dbstore.NewProperty(value, 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.%s: %v", 0, name, errCode)
		}
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	taskIDs := make([]int64, 0, 2)
	for _, property := range []string{"hold_left", "hold_right"} {
		program := compileTestProgram(t, scheduler.registry, "held = #0."+property+"; suspend(); return held;")
		taskIDs = append(taskIDs, scheduler.CreateBackgroundTask(0, program, 0))
	}
	if got := scheduler.ProcessReadyTasks(); got != 2 {
		t.Fatalf("processed tasks = %d, want two holders", got)
	}
	for _, taskID := range taskIDs {
		if state := scheduler.GetTask(taskID).GetState(); state != task.TaskSuspended {
			t.Fatalf("task %d state = %v, want suspended", taskID, state)
		}
	}
	for _, property := range []string{"hold_left", "hold_right"} {
		if errCode := store.DeleteDefinedProperty(0, property); errCode != types.E_NONE {
			t.Fatalf("delete #%d.%s: %v", 0, property, errCode)
		}
	}

	if lines := scheduler.EvalCommandOutput(0, "run_gc(); return 1;", "", ""); len(lines) != 1 || lines[0] != "{1, 1}" {
		t.Fatalf("run_gc eval output = %v, want successful return", lines)
	}
	for _, id := range []types.ObjID{anonA, anonB} {
		if !store.Valid(id) {
			t.Errorf("task-owned anonymous object #%d was recycled by explicit run_gc", id)
		}
	}
}
