package engine

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
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
	s := NewRuntime(store)

	program, diagnostics := s.registry.Compiler().CompileMOO([]string{"return 1;"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	taskID := s.CreateBackgroundTask(types.ObjNothing, program, 0)

	s.mu.Lock()
	bgTask := s.taskManager.GetTask(taskID)
	bgTask.StartTime = time.Now().Add(-10 * time.Minute)
	s.mu.Unlock()

	s.ProcessReadyTasks()

	if got := bgTask.GetState(); got != task.TaskCompleted {
		t.Fatalf("task state = %v, want TaskCompleted (stale StartTime should not expire the budget deadline)", got)
	}
}

func TestQueueTaskInitializesNewTaskFullContextBeforeRetryCapture(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)
	defer s.Stop()
	program := compileTestProgram(t, s.registry, "return 42;")
	queued := task.NewTaskFull(99001, types.ObjNothing, program, 1000, 10)
	if queued.Context.Store != nil {
		t.Fatal("NewTaskFull unexpectedly populated its context store")
	}

	s.QueueTask(queued)
	if queued.Context.Store != store {
		t.Fatal("QueueTask did not populate the runtime store before execution")
	}
	if queued.Context.StoreTxn != store.DirectTxn() {
		t.Fatal("QueueTask did not populate the direct transaction before retry capture")
	}
	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("processed tasks = %d, want 1", got)
	}
	if got := queued.GetState(); got != task.TaskCompleted {
		t.Fatalf("queued task state = %v, want completed", got)
	}
	if got := queued.Result; got.Flow != types.FlowReturn || got.Val.Type() != types.TYPE_INT || got.Val.Int() != 42 {
		t.Fatalf("queued task result = %+v, want return 42", got)
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
	s := NewRuntime(store)
	mgr := s.taskManager

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	program, diagnostics := s.registry.Compiler().CompileMOO([]string{"suspend(); return 42;"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	id := atomic.AddInt64(&s.nextTaskID, 1)
	tk := task.NewTaskFull(id, types.ObjNothing, program, ticks, seconds)
	s.populateTaskContextDependencies(tk.Context)
	tk.StartTime = time.Now()
	tk.ForkCreator = s
	s.mu.Lock()
	s.taskManager.RegisterTask(tk)
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
	// runtime readiness gate (engine/runtime.go: !StartTime.After(now)) fires.
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
	s := NewRuntime(store)

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	configureTestHost(s.session, func(host *builtins.Host) { host.ProcessStdin = builtins.NewProcessStdin(reader) })

	program, diagnostics := s.registry.Compiler().CompileMOO([]string{"return read_stdin();"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	taskID := s.CreateForegroundTask(types.ObjNothing, program)
	t.Cleanup(func() {
		s.taskManager.RemoveTask(taskID)
		s.mu.Lock()
		s.taskManager.RemoveTask(taskID)
		s.mu.Unlock()
	})
	readTask := s.taskManager.GetTask(taskID)

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
	s := NewRuntime(store)

	program, diagnostics := s.registry.Compiler().CompileMOO([]string{
		"fork (0)",
		"  suspend(0);",
		"  fork (0)",
		"    1 + 1;",
		"  endfork",
		"  suspend(0);",
		"endfork",
		"return 1;",
	})
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	mgr := s.taskManager
	parentID := s.CreateForegroundTask(types.ObjNothing, program)
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, catalogTask := range s.taskManager.Snapshot() {
			id := catalogTask.ID
			mgr.RemoveTask(id)
		}
	}()

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("initial scheduler pass ran %d tasks, want 1", got)
	}
	if got := s.taskManager.GetTask(parentID).GetState(); got != task.TaskCompleted {
		t.Fatalf("parent state = %v, want TaskCompleted", got)
	}

	var outer *task.Task
	for _, queued := range s.taskManager.Snapshot() {
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
	for _, queued := range s.taskManager.Snapshot() {
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

func TestSuspendFinalizesReleasedWaifBeforeResume(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if errCode := store.DirectTxn().DefineProperty(0, "waif_recycle_log", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define recycle log: %v", errCode)
	}

	s := NewRuntime(store)
	t.Cleanup(s.Stop)
	program := compileTestProgram(t, s.registry, strings.Join([]string{
		"c = create(-1);",
		`add_verb(c, {#0, "xd", ":recycle"}, {"this", "none", "this"});`,
		`set_verb_code(c, ":recycle", {"#0.waif_recycle_log = {@#0.waif_recycle_log, typeof(this) == WAIF};"});`,
		`add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});`,
		`set_verb_code(c, "new", {"return new_waif();"});`,
		"w = c:new();",
		"w = 0;",
		"suspend(0);",
		"return #0.waif_recycle_log;",
	}, "\n"))
	taskID := s.CreateBackgroundTask(0, program, 0)
	running := s.GetTask(taskID)
	running.Context.IsWizard = true
	for pass := 0; pass < 8 && running.GetState() != task.TaskCompleted && running.GetState() != task.TaskKilled; pass++ {
		if processed := s.ProcessReadyTasks(); processed == 0 {
			t.Fatalf("task made no progress in state %v", running.GetState())
		}
	}
	if got := running.Result; got.Flow != types.FlowReturn || got.Val.String() != "{1}" {
		t.Fatalf("released waif recycle log = %+v, want {1}", got)
	}
}
