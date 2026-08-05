package scheduler

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func TestRunStartupPendingFinalizationsUsesToastPerTypeOrder(t *testing.T) {
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

	anon1, errCode := store.CreateObject([]types.ObjID{9}, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %s", errCode)
	}
	anon2, errCode := store.CreateObject([]types.ObjID{9}, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create second anonymous object: %s", errCode)
	}
	waif1 := types.NewWaif(9, 2)
	waif2 := types.NewWaif(9, 2)
	store.SetPendingFinalizations([]types.Value{waif1, types.NewAnon(anon1), waif2, types.NewAnon(anon2)})

	s := NewScheduler(store)
	t.Cleanup(s.Stop)
	s.QueueStartupPendingFinalizations(store.TakePendingFinalizations())
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
	if len(observed) != 4 ||
		observed[0].Type() != types.TYPE_ANON || observed[0].ID() != anon2 ||
		observed[1].Type() != types.TYPE_ANON || observed[1].ID() != anon1 ||
		observed[2].WaifIdentity() != waif1.WaifIdentity() ||
		observed[3].WaifIdentity() != waif2.WaifIdentity() {
		t.Fatalf("observed order = %v, want anonymous #%d, anonymous #%d, then WAIF encounter order", observed, anon2, anon1)
	}
	if got := store.Snapshot().PendingFinalizations; len(got) != 0 {
		t.Fatalf("pending roots after execution = %v, want empty", got)
	}
	if store.Valid(anon1) || store.Valid(anon2) {
		t.Fatalf("anonymous objects remained valid after startup recycle: #%d=%v #%d=%v", anon1, store.Valid(anon1), anon2, store.Valid(anon2))
	}
}

func TestShutdownBoundaryOwnsQueuedStartupRootsBeforeReady(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	s := NewScheduler(store)
	t.Cleanup(s.Stop)
	var handedOff []types.Value
	s.SetPendingFinalizationSink(func(values []types.Value) {
		handedOff = append(handedOff, values...)
		store.AppendPendingFinalizations(values)
	})
	waif := types.NewWaif(0, 0)
	anonID, errCode := store.CreateObject([]types.ObjID{0}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous root: %s", errCode)
	}
	s.QueueStartupPendingFinalizations([]types.Value{waif, types.NewAnon(anonID)})

	<-s.BeginShutdown(nil)
	if len(handedOff) != 2 || handedOff[0].Type() != types.TYPE_ANON || handedOff[0].ID() != anonID || handedOff[1].WaifIdentity() != waif.WaifIdentity() {
		t.Fatalf("shutdown startup handoff = %v, want anonymous then WAIF", handedOff)
	}
	if pending := store.Snapshot().PendingFinalizations; len(pending) != 2 {
		t.Fatalf("checkpoint startup roots = %v, want two roots", pending)
	}
	if err := s.RunStartupPendingFinalizations(); err != nil {
		t.Fatalf("drained startup queue returned error: %v", err)
	}
}
