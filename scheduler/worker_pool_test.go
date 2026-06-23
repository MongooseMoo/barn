package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	dbstore "barn/db/store"
	"barn/parser"
	"barn/task"
	"barn/types"
)

func parseTestStatements(t *testing.T, code string) []parser.Stmt {
	t.Helper()
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() failed: %v", err)
	}
	return stmts
}

func newReadyTestTask(t *testing.T, id int64, owner types.ObjID) *task.Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(id, owner, parseTestStatements(t, "return 1;"), ticks, seconds)
	queued.StartTime = time.Now().Add(-time.Second)
	queued.Done = make(chan struct{})
	return queued
}

func TestSchedulerWorkerPoolStopsCleanly(t *testing.T) {
	s := newSchedulerWithWorkerCount(dbstore.NewStore(), false, 2)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() did not stop worker pool")
	}
}

func TestProcessReadyTasksRunsTaskOnceAndClosesDoneOnce(t *testing.T) {
	s := newSchedulerWithWorkerCount(dbstore.NewStore(), false, 2)
	defer s.Stop()

	var completes atomic.Int32
	queued := newReadyTestTask(t, 1001, 7)
	queued.OnComplete = func(types.Result) {
		completes.Add(1)
	}
	s.QueueTask(queued)
	defer task.GetManager().RemoveTask(queued.ID)

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("ProcessReadyTasks() = %d, want 1", got)
	}

	select {
	case <-queued.Done:
	case <-time.After(time.Second):
		t.Fatal("task Done channel was not closed")
	}
	if got := completes.Load(); got != 1 {
		t.Fatalf("OnComplete calls = %d, want 1", got)
	}
	if state := queued.GetState(); state != task.TaskCompleted {
		t.Fatalf("task state = %s, want completed", state)
	}
	if got := s.ProcessReadyTasks(); got != 0 {
		t.Fatalf("second ProcessReadyTasks() = %d, want 0", got)
	}
}

func TestProcessReadyTasksFlushesInReadyOrder(t *testing.T) {
	s := newSchedulerWithWorkerCount(dbstore.NewStore(), false, 2)
	defer s.Stop()

	var flushed []string
	s.SetTaskOutputFlusher(func(_ types.ObjID, suffix string) {
		flushed = append(flushed, suffix)
	})

	first := newReadyTestTask(t, 2001, 7)
	first.CommandOutputSuffix = "first"
	second := newReadyTestTask(t, 2002, 7)
	second.CommandOutputSuffix = "second"
	s.QueueTask(first)
	s.QueueTask(second)
	defer task.GetManager().RemoveTask(first.ID)
	defer task.GetManager().RemoveTask(second.ID)

	if got := s.ProcessReadyTasks(); got != 2 {
		t.Fatalf("ProcessReadyTasks() = %d, want 2", got)
	}

	if len(flushed) != 2 {
		t.Fatalf("flush count = %d, want 2", len(flushed))
	}
	if flushed[0] != "first" || flushed[1] != "second" {
		t.Fatalf("flush order = %#v, want [first second]", flushed)
	}
}
