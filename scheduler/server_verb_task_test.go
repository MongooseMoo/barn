package scheduler

import (
	"testing"

	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

func resetServerVerbTaskManager(t *testing.T) {
	t.Helper()
	mgr := task.GetManager()
	for _, tk := range mgr.GetAllTasks() {
		tk.Kill()
		mgr.RemoveTask(tk.ID)
	}
}

func addServerVerbTestObject(t *testing.T, store *dbstore.Store, id types.ObjID, flags dbstore.ObjectFlags) {
	t.Helper()
	builder := dbstore.NewObjectBuilder(id)
	builder.SetOwner(2)
	builder.SetName("test")
	builder.SetFlags(flags)
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("add object #%d: %v", id, err)
	}
}

func TestRunServerVerbTaskRunsBeforeReturning(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })

	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(0, dbstore.NewProperty("started", types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define property: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb("server_started", []string{"server_started"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"#0.started = 1;"}))

	scheduler := NewScheduler(store)
	if _, err := scheduler.RunServerVerbTask(0, "server_started", nil, 0); err != nil {
		t.Fatalf("run server verb task: %v", err)
	}

	value, errCode := store.PropertyValue(0, "started")
	if errCode != types.E_NONE {
		t.Fatalf("read property: %v", errCode)
	}
	started, ok := value.(types.IntValue)
	if !ok || started.Val != 1 {
		t.Fatalf("started = %v, want 1 before RunServerVerbTask returns", value)
	}
}
