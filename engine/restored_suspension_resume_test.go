package engine

import (
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A timed suspension restored from a checkpoint and resumed before its saved
// deadline is ready at once, as Toast's resume_task makes it (start_tv = 0).
// The checkpoint saves only the deadline; before the fix the loader also made
// it the task's StartTime, and readiness waits for the later of StartTime and
// WakeTime, so resume() held the task until the original deadline.
func TestRestoredTimedSuspensionResumedEarlyIsReadyNow(t *testing.T) {
	reloaded := barnCheckpointOfToastImport(t)
	store, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	saved := reloaded.SuspendedTasks[0]
	deadline := time.Now().Add(time.Hour)
	saved.Snapshot.StartTime = deadline
	if err := rt.LoadSuspendedTasks(reloaded.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(saved.Snapshot.ID)
	if restored == nil || restored.GetState() != task.TaskSuspended {
		t.Fatal("timed suspension was not restored suspended")
	}
	if start, _, _, _ := restored.QueuedTaskSnapshot(false); !start.Equal(deadline) {
		t.Fatalf("queued_tasks start = %v, want the saved deadline %v", start, deadline)
	}

	// resume() as a committed task row (builtins/tasks.go builtinResume).
	row, ok := store.TaskRow(saved.Snapshot.ID)
	if !ok {
		t.Fatal("restored task has no row")
	}
	row.Status, row.Value, row.Version = dbstore.TaskRowQueued, types.NewInt(42), row.Version+1
	rt.applyTaskRows([]dbstore.PublishedTaskRow{{ID: saved.Snapshot.ID, TaskRow: row}})

	now := time.Now()
	if restored.GetState() != task.TaskQueued {
		t.Fatalf("state after resume = %v, want queued", restored.GetState())
	}
	if ready := restored.ReadyDeadline(now); ready.IsZero() || ready.After(now) {
		t.Fatalf("resumed task ready at %v, want now (%v); saved deadline %v", ready, now, deadline)
	}
	if start, _, _ := restored.SchedulingSnapshot(); start.After(now) {
		t.Fatalf("resumed task's scheduling start %v is after now %v", start, now)
	}
}
