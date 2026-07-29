package task

import (
	"testing"
	"time"

	"barn/types"
)

func TestPersistenceSnapshotCopiesMutableFields(t *testing.T) {
	task := NewTaskFull(42, 7, nil, 100, 1)
	task.StartTime = time.Unix(123, 0)
	task.SetState(TaskQueued)
	task.IsExecSuspended = true
	task.ExecCommandName = "executables/sleep"
	task.ForkInfo = &types.ForkInfo{
		Variables: map[string]types.Value{
			"x": types.NewInt(1),
		},
		SourceLines: []string{"x = 1;"},
	}
	task.CallStack = []ActivationFrame{{
		This:   10,
		Verb:   "run",
		Args:   []types.Value{types.NewInt(2)},
		Player: 7,
	}}

	snapshot := task.PersistenceSnapshot()

	task.ForkInfo.Variables["x"] = types.NewInt(99)
	task.ForkInfo.SourceLines[0] = "x = 99;"
	task.CallStack[0].Verb = "changed"
	task.CallStack[0].Args[0] = types.NewInt(100)

	if got, want := snapshot.Fork.Variables["x"].String(), types.NewInt(1).String(); got != want {
		t.Errorf("snapshot fork variable = %s, want %s", got, want)
	}
	if got, want := snapshot.Fork.SourceLines[0], "x = 1;"; got != want {
		t.Errorf("snapshot source line = %q, want %q", got, want)
	}
	if got, want := snapshot.CallStack[0].Verb, "run"; got != want {
		t.Errorf("snapshot call stack verb = %q, want %q", got, want)
	}
	if got, want := snapshot.CallStack[0].Args[0].String(), types.NewInt(2).String(); got != want {
		t.Errorf("snapshot call stack arg = %s, want %s", got, want)
	}
	if !snapshot.IsExecSuspended || snapshot.ExecCommandName != "executables/sleep" {
		t.Errorf("snapshot exec state = %t %q", snapshot.IsExecSuspended, snapshot.ExecCommandName)
	}

	snapshot.Fork.Variables["x"] = types.NewInt(3)
	snapshot.Fork.SourceLines[0] = "x = 3;"
	snapshot.CallStack[0].Verb = "snapshot-mutated"
	nextSnapshot := task.PersistenceSnapshot()
	if got, want := nextSnapshot.Fork.Variables["x"].String(), types.NewInt(99).String(); got != want {
		t.Errorf("mutating snapshot changed task variable: got %s, want %s", got, want)
	}
	if got, want := nextSnapshot.Fork.SourceLines[0], "x = 99;"; got != want {
		t.Errorf("mutating snapshot changed task source line: got %q, want %q", got, want)
	}
	if got, want := nextSnapshot.CallStack[0].Verb, "changed"; got != want {
		t.Errorf("mutating snapshot changed task call stack: got %q, want %q", got, want)
	}
}

func TestQueuedTaskInfoRoundsStartTimeLikeCheckpoint(t *testing.T) {
	taskValue := NewTask(72, 2, 1000, 1)
	taskValue.StartTime = time.Unix(100, 600*time.Millisecond.Nanoseconds())
	taskValue.PushFrame(ActivationFrame{
		This:       4,
		Player:     2,
		Programmer: 2,
		Verb:       "delayed",
		VerbLoc:    4,
	})

	info := taskValue.ToQueuedTaskInfo(false)
	if got, want := info.Get(2).Int(), int64(101); got != want {
		t.Fatalf("queued start time = %d, want rounded %d", got, want)
	}

	taskValue.State = TaskSuspended
	taskValue.WakeTime = time.Unix(200, 600*time.Millisecond.Nanoseconds())
	info = taskValue.ToQueuedTaskInfo(false)
	if got, want := info.Get(2).Int(), int64(201); got != want {
		t.Fatalf("suspended wake time = %d, want rounded %d", got, want)
	}
}
