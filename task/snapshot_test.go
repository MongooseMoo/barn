package task

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

func TestTransformPersistenceValuesMatchesQueuedAndSuspendedWriterSurfaces(t *testing.T) {
	anon := func(id types.ObjID) types.Value { return types.NewAnon(id) }
	queued := Snapshot{
		Fork: &ForkSnapshot{Variables: map[string]types.Value{
			"first":  anon(101),
			"second": anon(102),
		}},
		CallStack: []ActivationFrame{{ThisValue: anon(103)}},
	}
	suspended := Snapshot{
		// A yielded fork retains Fork metadata in memory, but the suspended-task
		// writer serializes its VM instead. This decoy must not become a root.
		Fork:          &ForkSnapshot{Variables: map[string]types.Value{"decoy": anon(999)}},
		ReadingPlayer: types.ObjNothing,
		WakeValue:     anon(201),
		TaskLocal:     anon(202),
		CallStack:     []ActivationFrame{{ThisValue: anon(998)}},
		VM: &VMSnapshot{Frames: []VMFrameSnapshot{{
			Program:        bytecode.Program{Constants: []types.Value{anon(203), anon(204)}},
			Locals:         []types.Value{anon(205)},
			Stack:          []types.Value{anon(206)},
			ThisValue:      anon(207),
			Args:           []types.Value{anon(208)},
			PendingError:   VMErrorSnapshot{Present: true, Value: anon(209)},
			SavedThisValue: anon(210),
		}}},
	}
	interrupted := Snapshot{
		IsHTTPReadSuspended: true,
		WakeValue:           anon(399),
		TaskLocal:           anon(301),
		VM: &VMSnapshot{Frames: []VMFrameSnapshot{{
			Locals: []types.Value{anon(302)},
		}}},
	}

	visited := make(map[types.ObjID]int)
	transform := func(value types.Value) types.Value {
		if value.Type() != types.TYPE_ANON {
			return value
		}
		visited[value.ID()]++
		return types.NewAnon(value.ID() + 1000)
	}
	queued.TransformPersistenceValues(transform)
	suspended.TransformPersistenceValues(transform)
	interrupted.TransformPersistenceValues(transform)

	for _, id := range []types.ObjID{101, 102, 103, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 301, 302} {
		if got := visited[id]; got != 1 {
			t.Errorf("serialized value #%d visited %d times, want once", id, got)
		}
	}
	for _, id := range []types.ObjID{399, 998, 999} {
		if got := visited[id]; got != 0 {
			t.Errorf("non-serialized value #%d visited %d times, want zero", id, got)
		}
	}
	if got, want := queued.Fork.Variables["first"].ID(), types.ObjID(1101); got != want {
		t.Errorf("rewritten queued variable id = %d, want %d", got, want)
	}
	if got, want := suspended.VM.Frames[0].SavedThisValue.ID(), types.ObjID(1210); got != want {
		t.Errorf("rewritten saved-this id = %d, want %d", got, want)
	}
}

func TestPersistenceSnapshotCopiesMutableFields(t *testing.T) {
	task := NewTaskFull(42, 7, nil, 100, 1)
	task.StartTime = time.Unix(123, 0)
	task.SetState(TaskQueued)
	task.IsExecSuspended = true
	task.ExecCommandName = "executables/sleep"
	task.IsHTTPReadSuspended = true
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
	if !snapshot.IsHTTPReadSuspended {
		t.Error("snapshot lost HTTP read suspension state")
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
