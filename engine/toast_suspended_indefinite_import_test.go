package engine

import (
	"math"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Toast stamps a task suspended with no timeout with start time INTNUM_MAX
// (tasks.cc enqueue_suspended_task) and writes that number in the checkpoint.
// The task must stay suspended after Barn loads it: no wake deadline, still
// listed by queued_tasks, and resumed only by resume(). See
// testdata/toast-suspended-indefinite.md for the producer.
func TestToastIndefinitelySuspendedImportStaysSuspended(t *testing.T) {
	database := loadNativeImportFixture(t, "testdata/toast-suspended-indefinite.db", "ad6b96ec1d9fa2dd0eb10b6f04d6974fc28adeedd6aa4cf891f339ec8a6a9ada")
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	saved := database.SuspendedTasks[0]
	if got := saved.Snapshot.StartTime.Unix(); got != math.MaxInt64 {
		t.Fatalf("fixture start time=%d, want Toast's 64-bit INTNUM_MAX", got)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	holder := saved.Snapshot.This
	resumed := func() types.Value {
		t.Helper()
		value, code := store.DirectTxn().PropertyValue(holder, "resumed")
		if code != types.E_NONE {
			t.Fatalf("read resumed: %s", code)
		}
		return value
	}
	if got := resumed(); !got.Equal(types.NewList(nil)) {
		t.Fatalf("fixture resumed=%s, want {}", got)
	}

	runtime := newTestRuntimeWithBuiltins(t, store)
	defer runtime.Stop()
	if err := runtime.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := runtime.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatal("task was not restored")
	}
	if !restored.WakeTime.IsZero() || !restored.StartTime.Equal(task.IndefiniteSuspendStartTime) {
		t.Fatalf("wake/start = %v/%v, want zero wake and the indefinite start sentinel", restored.WakeTime, restored.StartTime)
	}
	now := time.Now()
	if deadline := restored.ReadyDeadline(now); !deadline.IsZero() {
		t.Fatalf("ready deadline=%v (now %v), want none", deadline, now)
	}
	if state := restored.GetState(); state != task.TaskSuspended {
		t.Fatalf("state=%s, want suspended", state)
	}
	if got := resumed(); !got.Equal(types.NewList(nil)) {
		t.Fatalf("resumed=%s after load, want {}", got)
	}
}
