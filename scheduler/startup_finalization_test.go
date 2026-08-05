package scheduler

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func TestRunStartupPendingFinalizationsUsesOrderedRegisteredTasks(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })

	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 9, 0)
	store.AddVerb(9, dbstore.NewVerb(":recycle", []string{":recycle", "recycle"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"observe_startup_finalization(this);"}))

	anonID, errCode := store.CreateObject([]types.ObjID{9}, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %s", errCode)
	}
	waif := types.NewWaif(9, 2)
	store.SetPendingFinalizations([]types.Value{waif, types.NewAnon(anonID)})

	s := NewScheduler(store)
	t.Cleanup(s.Stop)
	var observed []types.Value
	s.registry.Register("observe_startup_finalization", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		if len(args) != 1 {
			t.Fatalf("observe args = %v", args)
		}
		tk, ok := ctx.Task.(*task.Task)
		if !ok || tk == nil || task.GetManager().GetTask(tk.ID) != tk || s.GetTask(tk.ID) != tk {
			t.Fatalf("recycle hook task is not registered: task=%T id=%d", ctx.Task, ctx.TaskID)
		}
		if !sameFinalizationRoot(ctx.FinalizingValue, args[0]) {
			t.Fatalf("finalizing value = %v, hook this = %v", ctx.FinalizingValue, args[0])
		}
		observed = append(observed, args[0])
		return types.Ok(types.None)
	})

	if err := s.RunStartupPendingFinalizations(); err != nil {
		t.Fatalf("RunStartupPendingFinalizations: %v", err)
	}
	if len(observed) != 2 || observed[0].WaifIdentity() != waif.WaifIdentity() || observed[1].Type() != types.TYPE_ANON || observed[1].ID() != anonID {
		t.Fatalf("observed order = %v, want exact WAIF then anonymous #%d", observed, anonID)
	}
	if got := store.Snapshot().PendingFinalizations; len(got) != 0 {
		t.Fatalf("pending roots after execution = %v, want empty", got)
	}
	if store.Valid(anonID) {
		t.Fatalf("anonymous object #%d remained valid after startup recycle", anonID)
	}
}
