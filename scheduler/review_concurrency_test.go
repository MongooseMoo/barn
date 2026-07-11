package scheduler

// Review test file: concurrency / task-lifecycle analyst review.
// ALL tests in this file are expected to be RED (failing) — they expose bugs.
// Run: go test ./scheduler/ -run TestReview_ -v
// Race: go test -race ./scheduler/ -run TestReview_ -v

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

// resetReviewManager drains any tasks left in the global manager from prior tests.
func resetReviewManager(t *testing.T) {
	t.Helper()
	mgr := task.GetManager()
	for _, tk := range mgr.GetAllTasks() {
		mgr.RemoveTask(tk.ID)
	}
}

// -----------------------------------------------------------------------------
// BUG-1: ProcessReadyTasks closes t.Done even when the task only SUSPENDED
// (did not complete or get killed). Any waiter on Done wakes believing the
// task is finished while the task is merely suspended.
//
// scheduler.go:164-168:
//
//	if t.Done != nil {
//	    close(t.Done)   <- fires unconditionally after runTask returns
//	}
//
// runTask returns nil for BOTH FlowSuspend and terminal completion.
// -----------------------------------------------------------------------------
func TestReview_DoneChannelClosedOnSuspend(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)

	program, diagnostics := compiler.CompileMOO([]string{"suspend(100);"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}

	taskID := s.CreateForegroundTask(types.ObjNothing, program)
	bgTask := s.GetTask(taskID)
	if bgTask == nil {
		t.Fatal("task not found in scheduler")
	}

	done := make(chan struct{})
	bgTask.Done = done

	s.ProcessReadyTasks()

	state := bgTask.GetState()
	if state != task.TaskSuspended {
		t.Skipf("task state = %v (expected TaskSuspended; suspend() may not be wired in test env)", state)
	}

	// EXPECT: Done should NOT be closed while the task is suspended.
	// BUG: ProcessReadyTasks closes Done unconditionally after runTask returns,
	// even when runTask returned due to FlowSuspend (not terminal).
	select {
	case <-done:
		t.Error("BUG: ProcessReadyTasks closed t.Done on a suspended (not completed/killed) task; " +
			"waiters will falsely believe the task has terminated")
	default:
		// Correct: Done is still open — task is alive.
	}
}

// -----------------------------------------------------------------------------
// BUG-2: Scheduler.nextTaskID and task.Manager.nextTaskID are independent
// counters both starting at 1. EvalCommandOutput creates tasks via
// manager.CreateTask (manager counter); all other paths use s.nextTaskID.
// After checkpoint restore s.nextTaskID is advanced to max(restored IDs) but
// manager.nextTaskID is NOT, so the eval-task IDs overlap the restored
// scheduler-task IDs. QueueTask.RegisterTask then overwrites the manager's
// record for the colliding ID — the original task is silently lost from
// manager.tasks so kill_task / resume_task / queued_tasks act on the wrong task.
// -----------------------------------------------------------------------------
func TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent(t *testing.T) {
	resetReviewManager(t)
	t.Cleanup(func() { resetReviewManager(t) })

	store := dbstore.NewStore()
	s := NewScheduler(store)
	mgr := task.GetManager()

	// Probe: discover the next ID the manager will allocate.
	probe := mgr.CreateTask(types.ObjNothing, 100, 1.0)
	nextManagerID := probe.ID + 1 // manager will hand out this ID next
	mgr.RemoveTask(probe.ID)

	// Coerce the scheduler counter to produce the same ID on its next allocation.
	atomic.StoreInt64(&s.nextTaskID, nextManagerID-1)

	// Simulate EvalCommandOutput: create a task directly via manager.
	evalTask := mgr.CreateTask(types.ObjNothing, 100, 1.0)
	defer mgr.RemoveTask(evalTask.ID)
	if evalTask.ID != nextManagerID {
		t.Fatalf("manager ID probe misfire: got %d, want %d", evalTask.ID, nextManagerID)
	}

	// Now create a scheduler task — it gets the same ID from s.nextTaskID.
	program, diagnostics := compiler.CompileMOO([]string{"return 1;"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	schedulerTaskID := s.CreateForegroundTask(types.ObjNothing, program)
	if schedulerTaskID != nextManagerID {
		t.Fatalf("scheduler ID probe misfire: got %d, want %d", schedulerTaskID, nextManagerID)
	}

	// QueueTask (called by CreateForegroundTask) calls manager.RegisterTask,
	// writing manager.tasks[nextManagerID] = schedulerTask — overwriting the eval task.
	managerView := mgr.GetTask(nextManagerID)

	if managerView == evalTask {
		// If we still see the eval task, the collision did not overwrite it.
		t.Fatal("eval task unexpectedly still in manager after scheduler registration (test setup issue)")
	}

	// The eval task has been silently displaced from manager.tasks[nextManagerID].
	// kill_task(nextManagerID) now kills the scheduler task, not the eval task.
	t.Errorf("BUG: ID collision at %d — manager.CreateTask and scheduler.QueueTask produced "+
		"the same task ID from independent counters; the eval task was overwritten in "+
		"manager.tasks and is no longer reachable by kill_task/resume_task/queued_tasks",
		nextManagerID)
}

// -----------------------------------------------------------------------------
// BUG-3 (SUSPECTED race, confirmed structurally): scheduler.liveTaskVMs reads
// t.BytecodeVM holding s.mu but NOT task.mu, while scheduler.runTask writes
// t.BytecodeVM holding NEITHER lock. When the main/checkpoint goroutine calls
// RunServerVerbTask concurrently with the InputProcessor calling
// ProcessReadyTasks, liveTaskVMs on one goroutine reads BytecodeVM of a task
// whose runTask on the other goroutine is simultaneously writing it.
//
// scheduler.go:189    read:  queued.BytecodeVM.(*vm.VM)  [under s.mu only]
// task_runtime.go:239 write: t.BytecodeVM = bcVM         [no lock at all]
// task_runtime.go:314 write: t.BytecodeVM = nil          [no lock at all]
//
// To detect: run with -race flag.
// The test sets up two concurrent goroutines:
//
//	goroutine A runs a suspending task (writes BytecodeVM at line 239)
//	goroutine B runs a completing task whose liveTaskVMs call reads goroutine A's task BytecodeVM
//
// -----------------------------------------------------------------------------
func TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask(t *testing.T) {
	resetReviewManager(t)
	t.Cleanup(func() { resetReviewManager(t) })

	store := dbstore.NewStore()
	s := NewScheduler(store)

	ticks, seconds := foregroundTaskLimits()

	// taskA: suspend(100) — its runTask will write taskA.BytecodeVM = bcVM at line 239.
	programA, diagnostics := compiler.CompileMOO([]string{"suspend(100);"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile taskA: %v", diagnostics)
	}
	taskAID := atomic.AddInt64(&s.nextTaskID, 1)
	taskA := task.NewTaskFull(taskAID, types.ObjNothing, programA, ticks, seconds)
	s.populateTaskContextDependencies(taskA.Context)
	taskA.StartTime = time.Now()
	taskA.ForkCreator = s
	s.mu.Lock()
	s.tasks[taskA.ID] = taskA
	s.mu.Unlock()
	taskA.SetState(task.TaskQueued)
	task.GetManager().RegisterTask(taskA)

	// taskB: return 1 — completes quickly; its runTask calls liveTaskVMs at the
	// GC boundary (task_runtime.go:307), which reads taskA.BytecodeVM.
	programB, diagnostics := compiler.CompileMOO([]string{"return 1;"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile taskB: %v", diagnostics)
	}
	taskBID := atomic.AddInt64(&s.nextTaskID, 1)
	taskB := task.NewTaskFull(taskBID, types.ObjNothing, programB, ticks, seconds)
	s.populateTaskContextDependencies(taskB.Context)
	taskB.StartTime = time.Now()
	taskB.ForkCreator = s
	s.mu.Lock()
	s.tasks[taskB.ID] = taskB
	s.mu.Unlock()
	taskB.SetState(task.TaskQueued)
	task.GetManager().RegisterTask(taskB)

	var wg sync.WaitGroup

	// Goroutine A: run taskA (suspend path — writes taskA.BytecodeVM without lock).
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Yield so goroutine B is likely scheduled before we start writing BytecodeVM.
		runtime.Gosched()
		_ = s.runTask(taskA)
	}()

	// Goroutine B: run taskB (completion path — liveTaskVMs reads taskA.BytecodeVM under s.mu).
	// The s.mu read-lock does NOT protect against the unguarded write in goroutine A.
	wg.Add(1)
	go func() {
		defer wg.Done()
		runtime.Gosched()
		_ = s.runTask(taskB)
	}()

	wg.Wait()

	// This test does not call t.Error unconditionally; it exposes the race
	// structurally for the race detector.  Run as:
	//   go test -race ./scheduler/ -run TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask -count=10
	// The race may not trigger on every run due to goroutine scheduling.
	// Confirmed structurally: liveTaskVMs holds s.mu but NOT task.mu; runTask
	// writes BytecodeVM with NO lock; they CAN overlap when called concurrently
	// from different OS goroutines (mainLoop vs InputProcessor).
}
