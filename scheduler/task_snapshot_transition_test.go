package scheduler

import (
	"testing"
	"time"

	"barn/config"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func TestSuspendPublishesTaskOwnedRootsVMAndStateUnderSchedulerLock(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	s := newSchedulerWithWorkerCount(store, config.Options{}, 1)
	t.Cleanup(s.Stop)

	const taskID = 92001
	tk := task.NewTaskFull(taskID, 0, compileTestProgram(t, s.registry, `
a = create(#0, #0, 1);
suspend();
`), 1<<50, 1e9)
	tk.Context.IsWizard = true
	tk.SetState(task.TaskQueued)
	s.mu.Lock()
	s.tasks[taskID] = tk
	s.mu.Unlock()
	task.GetManager().RegisterTask(tk)
	t.Cleanup(func() { task.GetManager().RemoveTask(taskID) })

	stages := make(chan string)
	release := make(chan struct{})
	s.taskLifecycleObserver = func(stage string, _ *task.Task) {
		stages <- stage
		<-release
	}
	runDone := make(chan error, 1)
	go func() { runDone <- s.runTask(tk) }()

	wantStage := func(want string) {
		t.Helper()
		select {
		case got := <-stages:
			if got != want {
				t.Fatalf("transition stage = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for transition stage %q", want)
		}
	}

	wantStage("suspend_before_publish")
	queued, suspended := s.TaskSnapshots()
	if len(queued) != 0 || len(suspended) != 0 {
		t.Fatalf("running task checkpointed before suspension publication: queued=%v suspended=%v", queued, suspended)
	}
	if tk.GetState() != task.TaskRunning || tk.BytecodeVMValue() != nil {
		t.Fatalf("pre-publication task = state %s VM %T, want running with no saved VM", tk.GetState(), tk.BytecodeVMValue())
	}
	release <- struct{}{}

	wantStage("suspend_during_publish")
	if s.mu.TryLock() {
		s.mu.Unlock()
		t.Fatal("task-owned GC request became visible without scheduler ownership")
	}
	if s.pendingWaifMu.TryLock() {
		s.pendingWaifMu.Unlock()
		t.Fatal("task-owned GC request became visible without finalization ownership")
	}
	// The observer runs inside the task lock after VM installation. Inspecting
	// task fields here would block, which is the transition guarantee under test.
	release <- struct{}{}

	wantStage("suspend_after_publish")
	if s.mu.TryLock() {
		s.mu.Unlock()
		t.Fatal("suspended VM/state became visible without scheduler ownership")
	}
	if tk.GetState() != task.TaskSuspended || tk.BytecodeVMValue() == nil {
		t.Fatalf("published stage = state %s VM %T, want suspended with VM", tk.GetState(), tk.BytecodeVMValue())
	}
	release <- struct{}{}

	if err := <-runDone; err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	queued, suspended = s.TaskSnapshots()
	if len(queued) != 0 || len(suspended) != 1 || suspended[0].VM == nil || suspended[0].ID != taskID {
		t.Fatalf("published snapshots: queued=%v suspended=%v, want one resumable suspended task", queued, suspended)
	}
	s.pendingWaifMu.Lock()
	if len(s.pendingAnonGC) != 1 || !s.pendingAnonGC[0].TaskOwned {
		s.pendingWaifMu.Unlock()
		t.Fatalf("pending anonymous GC = %#v, want one task-owned request", s.pendingAnonGC)
	}
	s.pendingWaifMu.Unlock()

	roots := suspended[0].RootValues()
	snapshot := store.SnapshotWithRoots(roots)
	if len(snapshot.PendingFinalizations) != 0 || len(snapshot.AnonymousObjects) != 1 {
		t.Fatalf("checkpoint = anonymous %d pending %v, want task-rooted anonymous and no promoted root", len(snapshot.AnonymousObjects), snapshot.PendingFinalizations)
	}
}

func TestTaskSnapshotsSkipResumedRunningVMUntilNextSuspend(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	s := newSchedulerWithWorkerCount(store, config.Options{}, 1)
	t.Cleanup(s.Stop)

	entered := make(chan struct{})
	release := make(chan struct{})
	s.registry.Register("checkpoint_block", func(*kernel.TaskContext, []types.Value) types.Result {
		close(entered)
		<-release
		return types.Ok(types.None)
	})

	const taskID = 92002
	tk := task.NewTaskFull(taskID, 0, compileTestProgram(t, s.registry, `
suspend();
checkpoint_block();
suspend();
`), 1<<50, 1e9)
	tk.Context.IsWizard = true
	tk.SetState(task.TaskQueued)
	s.mu.Lock()
	s.tasks[taskID] = tk
	s.mu.Unlock()
	task.GetManager().RegisterTask(tk)
	t.Cleanup(func() { task.GetManager().RemoveTask(taskID) })

	if err := s.runTask(tk); err != nil {
		t.Fatalf("initial runTask failed: %v", err)
	}
	if tk.GetState() != task.TaskSuspended || tk.BytecodeVMValue() == nil {
		t.Fatalf("initial suspension = state %s VM %T", tk.GetState(), tk.BytecodeVMValue())
	}
	if errCode := task.GetManager().ResumeTask(taskID, types.NewInt(7), 0, true); errCode != types.E_NONE {
		t.Fatalf("resume failed: %s", errCode)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- s.runTask(tk) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("resumed VM did not enter blocking builtin")
	}
	for i := 0; i < 100; i++ {
		queued, suspended := s.TaskSnapshots()
		if len(queued) != 0 || len(suspended) != 0 {
			t.Fatalf("running resumed VM checkpointed at iteration %d: queued=%v suspended=%v", i, queued, suspended)
		}
	}
	close(release)
	if err := <-runDone; err != nil {
		t.Fatalf("resumed runTask failed: %v", err)
	}
	queued, suspended := s.TaskSnapshots()
	if len(queued) != 0 || len(suspended) != 1 || suspended[0].ID != taskID || suspended[0].VM == nil {
		t.Fatalf("second suspension snapshots: queued=%v suspended=%v, want one complete VM snapshot", queued, suspended)
	}
}
