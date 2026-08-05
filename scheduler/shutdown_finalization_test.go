package scheduler

import (
	"testing"

	dbstore "barn/db/store"
	"barn/types"
)

func TestShutdownCompletionPreservesAnonymousRootsForCheckpoint(t *testing.T) {
	tests := []struct {
		name      string
		code      []string
		wantAnons int
	}{
		{
			name: "single local",
			code: []string{
				"a = create(#0, #2, 1);",
			},
			wantAnons: 1,
		},
		{
			name: "local cycle",
			code: []string{
				"a = create(#0, #2, 1);",
				"b = create(#0, #2, 1);",
				"a.next = b;",
				"b.next = a;",
			},
			wantAnons: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetServerVerbTaskManager(t)
			t.Cleanup(func() { resetServerVerbTaskManager(t) })
			store := dbstore.NewStore()
			addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
			addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
			if errCode := store.DefineProperty(0, "next", dbstore.NewProperty(types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
				t.Fatalf("define next property: %v", errCode)
			}
			store.AddVerb(0, dbstore.NewVerb("shutdown_started", []string{"shutdown_started"}, 2,
				dbstore.VerbRead|dbstore.VerbExecute,
				dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, tc.code))

			scheduler := NewScheduler(store)
			t.Cleanup(scheduler.Stop)
			scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)
			scheduler.BeginShutdown()
			if _, err := scheduler.RunServerVerbTask(0, "shutdown_started", nil, 0); err != nil {
				t.Fatalf("run shutdown_started: %v", err)
			}

			snapshot := store.Snapshot()
			if got := len(snapshot.AnonymousObjects); got != tc.wantAnons {
				t.Fatalf("checkpoint anonymous objects = %d, want %d (pending=%v)", got, tc.wantAnons, snapshot.PendingFinalizations)
			}
			if got := len(snapshot.PendingFinalizations); got != 1 {
				t.Fatalf("checkpoint pending roots = %v, want one canonical root", snapshot.PendingFinalizations)
			}
		})
	}
}

func TestShutdownCompletionPreservesTaskLocalWaif(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	store.AddVerb(0, dbstore.NewVerb("shutdown_started", []string{"shutdown_started"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"set_task_local(new_waif());"}))

	scheduler := NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)
	scheduler.BeginShutdown()
	if _, err := scheduler.RunServerVerbTask(0, "shutdown_started", nil, 0); err != nil {
		t.Fatalf("run shutdown_started: %v", err)
	}

	pending := store.Snapshot().PendingFinalizations
	if len(pending) != 1 || pending[0].Type() != types.TYPE_WAIF {
		t.Fatalf("checkpoint pending roots = %v, want task-local WAIF", pending)
	}
}
