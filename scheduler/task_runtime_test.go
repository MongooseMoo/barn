package scheduler

import (
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
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

	program, diagnostics := compiler.CompileMOO([]string{"return 1;"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	taskID := s.CreateBackgroundTask(types.ObjNothing, program, 0)

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
	mgr := s.taskManager

	ticks, seconds := foregroundTaskLimits()
	program, diagnostics := compiler.CompileMOO([]string{"suspend(); return 42;"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	id := atomic.AddInt64(&s.nextTaskID, 1)
	tk := task.NewTaskFull(id, types.ObjNothing, program, ticks, seconds)
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

func TestReadStdinErrorResumesAsLiteralValue(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	s.registry.SetProcessStdin(builtins.NewProcessStdin(reader))

	program, diagnostics := compiler.CompileMOO([]string{"return read_stdin();"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	taskID := s.CreateForegroundTask(types.ObjNothing, program)
	t.Cleanup(func() {
		s.taskManager.RemoveTask(taskID)
		s.mu.Lock()
		delete(s.tasks, taskID)
		s.mu.Unlock()
	})
	readTask := s.tasks[taskID]

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("initial scheduler pass ran %d tasks, want 1", got)
	}
	if got := readTask.GetState(); got != task.TaskSuspended {
		t.Fatalf("read_stdin task state = %v, want TaskSuspended", got)
	}

	if _, err := io.WriteString(writer, "a-prefixed input\n"); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for readTask.GetState() == task.TaskSuspended && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := readTask.GetState(); got != task.TaskQueued {
		t.Fatalf("read_stdin task state after input = %v, want TaskQueued", got)
	}

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("resume scheduler pass ran %d tasks, want 1", got)
	}
	if got := readTask.Result.Flow; got != types.FlowReturn {
		t.Fatalf("read_stdin result flow = %v, want FlowReturn (result=%v)", got, readTask.Result.Val)
	}
	if got := readTask.Result.Val; got.Type() != types.TYPE_ERR || got.ErrCode() != types.E_NACC {
		t.Fatalf("read_stdin result = %v, want literal E_NACC", got)
	}
}

func TestForkedTaskRequeuesAcrossSuspendAndCreatesNestedFork(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)

	program, diagnostics := compiler.CompileMOO([]string{
		"fork (0)",
		"  suspend(0);",
		"  fork (0)",
		"    1 + 1;",
		"  endfork",
		"  suspend(0);",
		"endfork",
		"return 1;",
	}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	mgr := s.taskManager
	parentID := s.CreateForegroundTask(types.ObjNothing, program)
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for id := range s.tasks {
			mgr.RemoveTask(id)
		}
	}()

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("initial scheduler pass ran %d tasks, want 1", got)
	}
	if got := s.tasks[parentID].GetState(); got != task.TaskCompleted {
		t.Fatalf("parent state = %v, want TaskCompleted", got)
	}

	var outer *task.Task
	for _, queued := range s.tasks {
		if queued.IsForked {
			outer = queued
			break
		}
	}
	if outer == nil {
		t.Fatal("outer fork task was not created")
	}

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("outer fork scheduler pass ran %d tasks, want 1", got)
	}
	if got := outer.GetState(); got != task.TaskQueued {
		t.Fatalf("outer fork state after first suspend = %v, want TaskQueued", got)
	}

	for range 4 {
		if s.ProcessReadyTasks() == 0 {
			break
		}
	}

	forkedCount := 0
	for _, queued := range s.tasks {
		if !queued.IsForked {
			continue
		}
		forkedCount++
		if got := queued.GetState(); got != task.TaskCompleted {
			t.Fatalf("forked task %d state = %v, want TaskCompleted", queued.ID, got)
		}
	}
	if forkedCount != 2 {
		t.Fatalf("forked task count = %d, want 2", forkedCount)
	}
}
