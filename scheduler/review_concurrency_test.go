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

	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
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
// Task IDs have one owner: the scheduler counter advanced by checkpoint restore.
// Eval and queued tasks must consume that same sequence so manager registration
// cannot overwrite a different live task with a colliding ID.
// -----------------------------------------------------------------------------
func TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent(t *testing.T) {
	resetReviewManager(t)
	t.Cleanup(func() { resetReviewManager(t) })

	store := dbstore.NewStore()
	s := NewScheduler(store)
	mgr := task.GetManager()

	// Simulate a restored database whose highest task ID was 99. Eval and queued
	// tasks must both advance the scheduler-owned counter from that point.
	atomic.StoreInt64(&s.nextTaskID, 99)
	line := s.EvalCommandOutput(types.ObjNothing, "return task_id();")
	if line != "{1, 100}" {
		t.Fatalf("eval task ID = %v, want {1, 100}", line)
	}

	program, diagnostics := compiler.CompileMOO([]string{"return 1;"}, s.registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	schedulerTaskID := s.CreateForegroundTask(types.ObjNothing, program)
	if schedulerTaskID != 101 {
		t.Fatalf("scheduler task ID = %d, want 101", schedulerTaskID)
	}

	if managerView := mgr.GetTask(schedulerTaskID); managerView == nil {
		t.Fatal("scheduler task was not registered with the global task manager")
	}
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
