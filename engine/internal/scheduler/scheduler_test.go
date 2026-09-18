package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func testTask(id int64, at time.Time) *task.Task {
	t := task.NewTask(id, 0, 1000, 1)
	t.StartTime = at
	t.SetState(task.TaskQueued)
	return t
}

func TestReadyBatchRetainsAdmittedTasksAheadOfNewCompletions(t *testing.T) {
	s := New(1, func(*task.Task) bool { return false }, func(*task.Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first, second := testTask(1, now), testTask(2, now)
	s.Enqueue(first)
	s.Enqueue(second)
	batch := s.ReadyBatch(now, nil)
	if len(batch) != 1 || batch[0] != first {
		t.Fatalf("first batch = %v", batch)
	}
	if second.GetState() != task.TaskQueued {
		t.Fatal("undispatched sibling was claimed")
	}
	completed := testTask(3, now)
	completed.SetBytecodeVM(struct{}{}) // Readiness marker, never executed.
	completed.SuspendIndefinite()
	if !completed.CompleteExec(types.NewInt(0)) {
		t.Fatal("completion failed")
	}
	catalog := []*task.Task{second, completed}
	batch = s.ReadyBatch(time.Now(), catalog)
	if len(batch) != 1 || batch[0] != second {
		t.Fatalf("admitted sibling lost its place: %v", batch)
	}
	batch = s.ReadyBatch(time.Now(), []*task.Task{completed})
	if len(batch) != 1 || batch[0] != completed {
		t.Fatalf("completion lost or duplicated: %v", batch)
	}
	if batch = s.ReadyBatch(time.Now(), nil); len(batch) != 0 {
		t.Fatalf("duplicate pending tasks: %v", batch)
	}
}

func TestReadyBatchSkipsKilledPendingTasksAndPreservesBatchBoundaries(t *testing.T) {
	s := New(2, func(t *task.Task) bool { return t.ID != 3 }, func(*task.Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	tasks := []*task.Task{testTask(1, now), testTask(2, now), testTask(3, now), testTask(4, now)}
	for _, task := range tasks {
		s.Enqueue(task)
	}
	batch := s.ReadyBatch(now, nil)
	if len(batch) != 2 || batch[0] != tasks[0] || batch[1] != tasks[1] {
		t.Fatalf("optimistic batch = %v", batch)
	}
	tasks[2].SetState(task.TaskKilled)
	batch = s.ReadyBatch(now, nil)
	if len(batch) != 1 || batch[0] != tasks[3] {
		t.Fatalf("batch after sibling kill = %v", batch)
	}
}

func TestReadyUsesFIFOForEqualReadyTimes(t *testing.T) {
	s := New(1, func(*task.Task) bool { return true }, func(*task.Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first, second := testTask(1, now), testTask(2, now)
	s.Enqueue(first)
	s.Enqueue(second)
	ready := s.Ready(now.Add(time.Second), nil)
	if len(ready) != 2 || ready[0] != first || ready[1] != second {
		t.Fatalf("ready order = %v, want [first second]", ready)
	}
}

func TestReadyLeavesUnstartedSiblingsQueued(t *testing.T) {
	s := New(1, func(*task.Task) bool { return false }, func(*task.Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first, second := testTask(1, now), testTask(2, now)
	s.Enqueue(first)
	s.Enqueue(second)
	ready := s.Ready(now.Add(time.Second), nil)
	if len(ready) != 2 {
		t.Fatalf("ready count = %d, want 2", len(ready))
	}
	for _, task := range ready {
		if !task.TryClaimQueued() {
			t.Fatalf("task %d was claimed before dispatch", task.ID)
		}
	}
}

func TestReadyMergesCompletedExecBeforeLaterFork(t *testing.T) {
	s := New(1, func(*task.Task) bool { return false }, func(*task.Task) error { return nil })
	t.Cleanup(s.Stop)
	completed := testTask(1, time.Now())
	completed.SetBytecodeVM(struct{}{}) // Readiness marker only; no VM is executed.
	completed.SuspendIndefinite()
	if !completed.CompleteExec(types.NewInt(0)) {
		t.Fatal("completion failed")
	}
	later := testTask(2, time.Now().Add(time.Millisecond))
	earlier := testTask(3, time.Now().Add(-time.Second))
	s.Enqueue(later)
	s.Enqueue(earlier)
	ordinary := testTask(4, time.Now().Add(-time.Hour))
	ordinary.SetBytecodeVM(struct{}{})
	ready := s.Ready(time.Now().Add(time.Second), []*task.Task{completed, later, earlier, ordinary})
	if len(ready) != 4 || ready[0] != completed || ready[1] != earlier || ready[2] != later || ready[3] != ordinary {
		t.Fatalf("ready = %v; completed external task must precede waiting forks", ready)
	}
}

func TestPlanIsolatesNonRetryableTasks(t *testing.T) {
	s := New(2, func(t *task.Task) bool { return t.ID != 2 }, func(*task.Task) error { return nil })
	t.Cleanup(s.Stop)
	tasks := []*task.Task{testTask(1, time.Time{}), testTask(2, time.Time{}), testTask(3, time.Time{})}
	batches := s.Plan(tasks)
	if len(batches) != 3 || len(batches[1]) != 1 || batches[1][0] != tasks[1] {
		t.Fatalf("batches = %#v, want retryable/non-retryable isolation", batches)
	}
}

func TestRunPreservesAssociationOrder(t *testing.T) {
	wantErr := errors.New("second")
	s := New(2, func(*task.Task) bool { return true }, func(t *task.Task) error {
		if t.ID == 2 {
			return wantErr
		}
		time.Sleep(time.Millisecond)
		return nil
	})
	t.Cleanup(s.Stop)
	first, second := testTask(1, time.Time{}), testTask(2, time.Time{})
	results := s.Run([]*task.Task{first, second})
	if results[0].Task != first || results[1].Task != second || !errors.Is(results[1].Err, wantErr) {
		t.Fatalf("results = %#v, want ordered task/result association", results)
	}
}
