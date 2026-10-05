package task

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestFindReadingTaskChoosesOldestQueueSequence(t *testing.T) {
	const player types.ObjID = 7
	manager := NewManager()
	var oldest *Task
	var nextOldest *Task

	for sequence := int64(64); sequence >= 1; sequence-- {
		candidate := NewTask(1000+sequence, 2, 100, 5)
		candidate.QueueSeq = sequence
		candidate.SetReadingPlayer(player)
		candidate.SetState(TaskSuspended)
		manager.RegisterTask(candidate)
		if sequence == 1 {
			oldest = candidate
		} else if sequence == 2 {
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
	manager := NewManager()
	states := map[int64]TaskState{1: TaskQueued, 2: TaskRunning, 3: TaskSuspended, 4: TaskCompleted, 5: TaskKilled}
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
	manager := NewManager()
	finished := NewTask(1, 2, 100, 5)
	finished.SetState(TaskCompleted)
	running := NewTask(2, 2, 100, 5)
	running.SetState(TaskRunning)
	manager.RegisterTask(finished)
	manager.RegisterTask(running)

	finished.mu.Lock()
	running.mu.Lock()
	done := make(chan []*Task, 1)
	go func() { done <- manager.Unfinished() }()
	var got []*Task
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Unfinished() waited for a task's lock")
	}
	running.mu.Unlock()
	finished.mu.Unlock()
	if len(got) != 1 || got[0] != running {
		t.Fatalf("Unfinished() = %v, want only the running task", got)
	}
}

// The index must equal the catalog's unfinished tasks after every kind of
// change, including a killed task that is suspended again, a task removed from
// the catalog, and a task replaced by another with the same ID.
func TestUnfinishedIndexFollowsEveryStateChange(t *testing.T) {
	manager := NewManager()
	check := func(step string) {
		t.Helper()
		want := map[int64]*Task{}
		for _, registered := range manager.Snapshot() {
			if !registered.GetState().finished() {
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
		{"SetState(TaskQueued)", func() { a.SetState(TaskQueued) }},
		{"TryClaimQueued", func() { a.TryClaimQueued() }},
		{"SetState(TaskCompleted)", func() { a.SetState(TaskCompleted) }},
		{"Suspend", func() { b.Suspend(time.Hour) }},
		{"Kill", func() { b.Kill() }},
		{"Kill twice", func() { b.Kill() }},
		{"Suspend of a killed task", func() { b.Suspend(0) }},
		{"SuspendIndefinite", func() { c.SuspendIndefinite() }},
		{"Resume", func() { c.Resume(types.NewInt(0)) }},
		{"StartExecution", func() { c.StartExecution() }},
		{"SetState(TaskKilled)", func() { c.SetState(TaskKilled) }},
		{"RemoveTask of an unfinished task", func() { manager.RemoveTask(b.ID) }},
		{"a state change after removal", func() { b.SetState(TaskQueued) }},
		{"RemoveTaskIf", func() { manager.RemoveTaskIf(c.ID, c) }},
		{"replacing a task under the same ID", func() {
			manager.RegisterTask(NewTask(a.ID, 2, 100, 5))
			a.SetState(TaskQueued)
		}},
		{"CleanupCompletedTasks", func() {
			manager.GetTask(a.ID).SetState(TaskCompleted)
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

// A state written anywhere but setStateLocked would leave the index wrong, and
// nothing else would notice.
func TestTaskStateIsWrittenInOnePlace(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	write := regexp.MustCompile(`\.State\s*=[^=]`)
	var sites []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for number, line := range strings.Split(string(source), "\n") {
			if write.MatchString(line) {
				sites = append(sites, fmt.Sprintf("%s:%d: %s", file, number+1, strings.TrimSpace(line)))
			}
		}
	}
	if len(sites) != 1 || !strings.Contains(sites[0], "t.State = state") {
		t.Fatalf("task state is assigned outside setStateLocked:\n%s", strings.Join(sites, "\n"))
	}
}

func TestFindReadingTaskBreaksEqualQueueSequenceByTaskID(t *testing.T) {
	const player types.ObjID = 7
	manager := NewManager()
	var lowestID *Task

	for id := int64(1064); id >= 1001; id-- {
		candidate := NewTask(id, 2, 100, 5)
		candidate.QueueSeq = 1
		candidate.SetReadingPlayer(player)
		candidate.SetState(TaskSuspended)
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
