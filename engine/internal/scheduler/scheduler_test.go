package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
)

func testTask(id int64, at time.Time) *task.Task {
	t := task.NewTask(id, 0, 1000, 1)
	t.StartTime = at
	t.SetState(task.TaskQueued)
	return t
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
