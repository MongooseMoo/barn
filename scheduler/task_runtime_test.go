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

// TestRunTaskStaleStartTimeDoesNotExpireDeadline covers a checkpoint-restored
// (or otherwise long-delayed) background task whose StartTime is already far
// in the past by the time it actually runs — e.g. the server was offline for
// a while between a checkpoint and the next restart. The budget deadline
// must be anchored to when the task actually starts executing, not to that
// stale StartTime, or it computes an already-expired context.Deadline and
// the task is killed instantly with context.DeadlineExceeded even though its
// code never got a chance to run.
func TestRunTaskStaleStartTimeDoesNotExpireDeadline(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)

	p := parser.NewParser("return 1;")
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	taskID := s.CreateBackgroundTask(types.ObjNothing, stmts, 0)

	s.mu.Lock()
	bgTask := s.tasks[taskID]
	bgTask.StartTime = time.Now().Add(-10 * time.Minute)
	s.mu.Unlock()

	s.ProcessReadyTasks()

	if got := bgTask.GetState(); got != task.TaskCompleted {
		t.Fatalf("task state = %v, want TaskCompleted (stale StartTime should not expire the budget deadline)", got)
	}
}

// TestIndefiniteSuspendNotAutoWokenThenResumeRuns verifies F28v2 end-to-end: an
// indefinite suspend() (no/negative seconds) gets the far-future
// IndefiniteSuspendStartTime sentinel (so it sorts LAST in queued_tasks,
// mirroring ToastStunt's INTNUM_MAX start_tv at tasks.cc:1306-1307), the
// scheduler NEVER auto-wakes it, and an explicit resume() still wakes it so it
// runs to completion. Regression guard: the sentinel must not break resume.
func TestIndefiniteSuspendNotAutoWokenThenResumeRuns(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)

	// Drain any tasks left in the global manager by other tests so our resume
	// lookup and ProcessReadyTasks iteration see only this task.
	mgr := task.GetManager()
	for _, tk := range mgr.GetAllTasks() {
		mgr.RemoveTask(tk.ID)
	}

	ticks, seconds := foregroundTaskLimits()
	p := parser.NewParser("suspend(); return 42;")
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	id := atomic.AddInt64(&s.nextTaskID, 1)
	tk := task.NewTaskFull(id, types.ObjNothing, stmts, ticks, seconds)
	s.populateTaskContextDependencies(tk.Context)
	tk.StartTime = time.Now()
	tk.ForkCreator = s
	s.mu.Lock()
	s.tasks[tk.ID] = tk
	s.mu.Unlock()
	tk.SetState(task.TaskQueued)
	mgr.RegisterTask(tk)
	defer mgr.RemoveTask(tk.ID)

	// First run: executes up to the indefinite suspend().
	if err := s.runTask(tk); err != nil {
		t.Fatalf("runTask (initial): %v", err)
	}
	if got := tk.GetState(); got != task.TaskSuspended {
		t.Fatalf("after suspend(): state = %v, want TaskSuspended", got)
	}
	if !tk.StartTime.Equal(task.IndefiniteSuspendStartTime) {
		t.Fatalf("after suspend(): StartTime = %v, want sentinel %v",
			tk.StartTime, task.IndefiniteSuspendStartTime)
	}
	if tk.WakeDue(time.Now()) {
		t.Fatalf("indefinite suspend reported WakeDue=true; it must never auto-wake")
	}

	// The scheduler must NOT auto-wake an indefinitely-suspended task.
	s.ProcessReadyTasks()
	if got := tk.GetState(); got != task.TaskSuspended {
		t.Fatalf("after ProcessReadyTasks: state = %v, want still TaskSuspended (no auto-wake)", got)
	}

	// An explicit resume() must wake it and clear the sentinel so the
	// scheduler readiness gate (scheduler.go: !StartTime.After(now)) fires.
	if ec := mgr.ResumeTask(tk.ID, types.NewInt(0), types.ObjNothing, true); ec != types.E_NONE {
		t.Fatalf("ResumeTask returned %v, want E_NONE", ec)
	}
	if tk.StartTime.Equal(task.IndefiniteSuspendStartTime) {
		t.Fatalf("after resume(): StartTime still the sentinel; the scheduler would never run it")
	}

	// Now the scheduler runs the resumed task to completion.
	s.ProcessReadyTasks()
	if got := tk.GetState(); got != task.TaskCompleted {
		t.Fatalf("after resume + ProcessReadyTasks: state = %v, want TaskCompleted", got)
	}
}
