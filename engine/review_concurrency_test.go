package engine

// Review test file: concurrency / task-lifecycle analyst review.
// ALL tests in this file are expected to be RED (failing) — they expose bugs.
// Run: go test ./engine/ -run TestReview_ -v
// Race: go test -race ./engine/ -run TestReview_ -v

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// -----------------------------------------------------------------------------
// BUG-1: ProcessReadyTasks closes t.Done even when the task only SUSPENDED
// (did not complete or get killed). Any waiter on Done wakes believing the
// task is finished while the task is merely suspended.
//
// runtime.go:
//
//	if t.Done != nil {
//	    close(t.Done)   <- fires unconditionally after runTask returns
//	}
//
// runTask returns nil for BOTH FlowSuspend and terminal completion.
// -----------------------------------------------------------------------------
func TestReview_DoneChannelClosedOnSuspend(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)

	program, diagnostics := s.registry.Compiler().CompileMOO([]string{"suspend(100);"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}

	taskID := s.CreateForegroundTask(types.ObjNothing, program)
	bgTask := s.GetTask(taskID)
	if bgTask == nil {
		t.Fatal("task not found in runtime")
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
// Task IDs have one owner: the runtime counter advanced by checkpoint restore.
// Eval and queued tasks must consume that same sequence so manager registration
// cannot overwrite a different live task with a colliding ID.
// -----------------------------------------------------------------------------
func TestReview_IDCollisionManagerAndRuntimeCountersAreIndependent(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)
	mgr := s.taskManager

	// Simulate a restored database whose highest task ID was 99. Eval and queued
	// tasks must both advance the runtime-owned counter from that point.
	atomic.StoreInt64(&s.nextTaskID, 99)
	line := s.EvalCommandOutput(types.ObjNothing, "return task_id();")
	if line != "{1, 100}" {
		t.Fatalf("eval task ID = %v, want {1, 100}", line)
	}

	program, diagnostics := s.registry.Compiler().CompileMOO([]string{"return 1;"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	runtimeTaskID := s.CreateForegroundTask(types.ObjNothing, program)
	if runtimeTaskID != 101 {
		t.Fatalf("runtime task ID = %d, want 101", runtimeTaskID)
	}

	if managerView := mgr.GetTask(runtimeTaskID); managerView == nil {
		t.Fatal("runtime task was not registered with its task manager")
	}
}

// Concurrent suspension and completion exercise saved-VM publication under
// the race detector. Task.VM and SetVM share State.Mu; runtime execution leases
// separately govern whether a VM's mutable frames may be walked.
func TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)

	ticks, seconds := foregroundTaskLimits(newTestRegistry())

	// taskA retains its interpreter across suspension.
	programA, diagnostics := s.registry.Compiler().CompileMOO([]string{"suspend(100);"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile taskA: %v", diagnostics)
	}
	taskAID := atomic.AddInt64(&s.nextTaskID, 1)
	taskA := NewTaskFull(taskAID, types.ObjNothing, programA, ticks, seconds)
	s.populateContextDependencies(taskA.Context)
	taskA.StartTime = time.Now()
	taskA.ForkCreator = s
	s.mu.Lock()
	s.taskManager.RegisterTask(taskA)
	s.mu.Unlock()
	taskA.SetState(task.TaskQueued)
	s.taskManager.RegisterTask(taskA)

	// taskB completes while taskA may be publishing its saved interpreter.
	programB, diagnostics := s.registry.Compiler().CompileMOO([]string{"return 1;"})
	if len(diagnostics) > 0 {
		t.Fatalf("compile taskB: %v", diagnostics)
	}
	taskBID := atomic.AddInt64(&s.nextTaskID, 1)
	taskB := NewTaskFull(taskBID, types.ObjNothing, programB, ticks, seconds)
	s.populateContextDependencies(taskB.Context)
	taskB.StartTime = time.Now()
	taskB.ForkCreator = s
	s.mu.Lock()
	s.taskManager.RegisterTask(taskB)
	s.mu.Unlock()
	taskB.SetState(task.TaskQueued)
	s.taskManager.RegisterTask(taskB)

	var wg sync.WaitGroup

	// Goroutine A runs the suspending task.
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Allow the completing task to run before this task suspends.
		runtime.Gosched()
		_ = s.runTask(taskA)
	}()

	// Goroutine B runs the completing task.
	wg.Add(1)
	go func() {
		defer wg.Done()
		runtime.Gosched()
		_ = s.runTask(taskB)
	}()

	wg.Wait()

	// The race detector supplies this test's failure signal. Run as:
	//   go test -race ./engine/ -run TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask -count=10
	// The interleaving varies with goroutine scheduling.
}
