package engine

import (
	"maps"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestFindReadingTaskChoosesOldestQueueSequence(t *testing.T) {
	const player types.ObjID = 7
	manager := NewTaskManager()
	var oldest *Task
	var nextOldest *Task

	for sequence := int64(64); sequence >= 1; sequence-- {
		candidate := NewTask(1000+sequence, 2, 100, 5)
		candidate.QueueSeq = sequence
		candidate.SetReadingPlayer(player)
		candidate.SetState(task.TaskSuspended)
		manager.RegisterTask(candidate)
		switch sequence {
		case 1:
			oldest = candidate
		case 2:
			nextOldest = candidate
		}
	}

	for range 100 {
		if got := manager.FindReadingTask(player); got != oldest {
			t.Fatalf("FindReadingTask() = task %d with sequence %d, want oldest task %d with sequence %d", got.ID, got.QueueSeq, oldest.ID, oldest.QueueSeq)
		}
	}

	// Once the first reader is no longer waiting, selection advances in FIFO
	// order rather than falling back to whichever map entry is visited first.
	oldest.SetReadingPlayer(0)
	if got := manager.FindReadingTask(player); got != nextOldest {
		t.Fatalf("FindReadingTask() after oldest reader left = task %d with sequence %d, want next-oldest task %d with sequence %d", got.ID, got.QueueSeq, nextOldest.ID, nextOldest.QueueSeq)
	}
}

// Finished tasks stay registered until the periodic cleanup. A scheduling scan
// that walked them cost most of the dispatcher's time under load: 2,400
// registered tasks for 23 live ones.
func TestUnfinishedLeavesOutCompletedAndKilledTasks(t *testing.T) {
	manager := NewTaskManager()
	states := map[int64]task.TaskState{1: task.TaskQueued, 2: task.TaskRunning, 3: task.TaskSuspended, 4: task.TaskCompleted, 5: task.TaskKilled}
	for id, state := range states {
		registered := NewTask(id, 2, 100, 5)
		registered.SetState(state)
		manager.RegisterTask(registered)
	}

	got := map[int64]bool{}
	for _, unfinished := range manager.Unfinished() {
		got[unfinished.ID] = true
	}
	if len(got) != 3 || !got[1] || !got[2] || !got[3] {
		t.Fatalf("Unfinished() = %v, want tasks 1, 2 and 3", got)
	}
	if n := len(manager.Snapshot()); n != len(states) {
		t.Fatalf("Snapshot() has %d tasks, want all %d still registered", n, len(states))
	}
}

// Unfinished ran on every scheduling tick and took the lock of every task in
// the catalog, finished ones included. With a few thousand finished tasks
// waiting for cleanup that was a tenth of the server's CPU, plus the lock
// traffic it caused in the tasks that were running. It must answer from the
// index without touching a task.
func TestUnfinishedTakesNoTaskLock(t *testing.T) {
	manager := NewTaskManager()
	finished := NewTask(1, 2, 100, 5)
	finished.SetState(task.TaskCompleted)
	running := NewTask(2, 2, 100, 5)
	running.SetState(task.TaskRunning)
	manager.RegisterTask(finished)
	manager.RegisterTask(running)

	finished.Mu.Lock()
	running.Mu.Lock()
	done := make(chan []*Task, 1)
	go func() { done <- manager.Unfinished() }()
	var got []*Task
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Unfinished() waited for a task's lock")
	}
	running.Mu.Unlock()
	finished.Mu.Unlock()
	if len(got) != 1 || got[0] != running {
		t.Fatalf("Unfinished() = %v, want only the running task", got)
	}
}

// The index must equal the catalog's unfinished tasks after every kind of
// change, including a killed task that is suspended again, a task removed from
// the catalog, and a task replaced by another with the same ID.
func TestUnfinishedIndexFollowsEveryStateChange(t *testing.T) {
	manager := NewTaskManager()
	check := func(step string) {
		t.Helper()
		want := map[int64]*Task{}
		for _, registered := range manager.Snapshot() {
			registered.Mu.RLock()
			settled := registered.SettledLocked()
			registered.Mu.RUnlock()
			if !settled {
				want[registered.ID] = registered
			}
		}
		got := map[int64]*Task{}
		for _, unfinished := range manager.Unfinished() {
			got[unfinished.ID] = unfinished
		}
		if !maps.Equal(got, want) {
			t.Fatalf("after %s: Unfinished() = %v, want %v", step, got, want)
		}
	}

	a, b, c := NewTask(1, 2, 100, 5), NewTask(2, 2, 100, 5), NewTask(3, 2, 100, 5)
	for _, registered := range []*Task{a, b, c} {
		manager.RegisterTask(registered)
	}
	check("registering three new tasks")

	steps := []struct {
		name string
		do   func()
	}{
		{"SetState(task.TaskQueued)", func() { a.SetState(task.TaskQueued) }},
		{"TryClaimQueued", func() { a.TryClaimQueued() }},
		{"SetState(task.TaskCompleted)", func() { a.SetState(task.TaskCompleted) }},
		{"Suspend", func() { b.Suspend(time.Hour) }},
		{"Kill", func() { b.Kill() }},
		{"Kill twice", func() { b.Kill() }},
		{"Suspend of a killed task", func() { b.Suspend(0) }},
		{"SuspendIndefinite", func() { c.SuspendIndefinite() }},
		{"Resume", func() { c.Resume(types.NewInt(0)) }},
		{"StartExecution", func() { c.StartExecution() }},
		{"SetState(task.TaskKilled)", func() { c.SetState(task.TaskKilled) }},
		{"SetExecutionActive(false)", func() { c.SetExecutionActive(false) }},
		{"RemoveTask of an unfinished task", func() { manager.RemoveTask(b.ID) }},
		{"a state change after removal", func() { b.SetState(task.TaskQueued) }},
		{"RemoveTaskIf", func() { manager.RemoveTaskIf(c.ID, c) }},
		{"replacing a task under the same ID", func() {
			manager.RegisterTask(NewTask(a.ID, 2, 100, 5))
			a.SetState(task.TaskQueued)
		}},
		{"CleanupCompletedTasks", func() {
			manager.GetTask(a.ID).SetState(task.TaskCompleted)
			manager.CleanupCompletedTasks()
		}},
	}
	for _, step := range steps {
		step.do()
		check(step.name)
	}
	if n := len(manager.Unfinished()); n != 0 {
		t.Fatalf("Unfinished() has %d tasks at the end, want none", n)
	}
}

// A task that dies with an error is killed before #0:handle_uncaught_error
// runs. PendingReadier keeps holding the task that forked it until the slice
// ends, so a scheduling scan must still be handed the killed task: dropping it
// at the state change let a parent that forked and yielded resume before the
// hook's writes (tick_exhaustion_extended_opcode_does_not_test_the_budget read
// the probe one write early).
func TestUnfinishedKeepsFinishedTaskUntilItsSliceEnds(t *testing.T) {
	for _, final := range []task.TaskState{task.TaskKilled, task.TaskCompleted} {
		manager := NewTaskManager()
		parent, child := NewTask(1, 2, 100, 5), NewTask(2, 2, 100, 5)
		manager.RegisterTask(parent)
		manager.RegisterTask(child)
		child.SetReadier(&parent.State)
		if !child.StartExecution() {
			t.Fatal("child did not start")
		}
		child.SetState(final)

		holds := func() bool {
			for _, unfinished := range manager.Unfinished() {
				if unfinished.PendingReadier(time.Now()) == &parent.State {
					return true
				}
			}
			return false
		}
		if !holds() {
			t.Fatalf("state %v: Unfinished() dropped a task whose slice is still executing, releasing its readier", final)
		}
		child.SetExecutionActive(false)
		if holds() {
			t.Fatalf("state %v: readier still held after the slice ended", final)
		}
		for _, unfinished := range manager.Unfinished() {
			if unfinished == child {
				t.Fatalf("state %v: finished task still listed after its slice ended", final)
			}
		}
	}
}

func TestFindReadingTaskBreaksEqualQueueSequenceByTaskID(t *testing.T) {
	const player types.ObjID = 7
	manager := NewTaskManager()
	var lowestID *Task

	for id := int64(1064); id >= 1001; id-- {
		candidate := NewTask(id, 2, 100, 5)
		candidate.QueueSeq = 1
		candidate.SetReadingPlayer(player)
		candidate.SetState(task.TaskSuspended)
		manager.RegisterTask(candidate)
		if id == 1001 {
			lowestID = candidate
		}
	}

	for range 100 {
		if got := manager.FindReadingTask(player); got != lowestID {
			t.Fatalf("FindReadingTask() = task %d, want deterministic lowest ID %d for equal sequence", got.ID, lowestID.ID)
		}
	}
}
