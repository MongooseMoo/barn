package task

import (
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestCallStackProgrammer(t *testing.T) {
	task := &Task{}
	if who, frames := task.CallStackProgrammer(0); who != types.ObjNothing || frames != 0 {
		t.Fatalf("empty stack gave (#%d, %d), want (#-1, 0)", who, frames)
	}
	task.PushFrame(types.ActivationFrame{Programmer: types.ObjID(3)})
	task.PushFrame(types.ActivationFrame{Programmer: types.ObjID(5)})
	for depth, want := range []types.ObjID{5, 3, types.ObjNothing} {
		if who, frames := task.CallStackProgrammer(depth); who != want || frames != 2 {
			t.Fatalf("depth %d gave (#%d, %d), want (#%d, 2)", depth, who, frames, want)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { task.CallStackProgrammer(1) }); allocs != 0 {
		t.Fatalf("CallStackProgrammer allocated %v times", allocs)
	}
}

func TestSetTopFrameProgrammer(t *testing.T) {
	task := &Task{}
	if task.SetTopFrameProgrammer(types.ObjID(7)) {
		t.Fatal("SetTopFrameProgrammer succeeded with an empty call stack")
	}

	task.PushFrame(types.ActivationFrame{Programmer: types.ObjID(1)})
	if !task.SetTopFrameProgrammer(types.ObjID(7)) {
		t.Fatal("SetTopFrameProgrammer failed with a populated call stack")
	}
	if got := task.GetCallStack()[0].Programmer; got != types.ObjID(7) {
		t.Fatalf("top-frame programmer = %d, want #7", got)
	}
}

func TestSetTopFrameProgrammerConcurrentWithFrameGrowth(t *testing.T) {
	task := &Task{CallStack: make([]types.ActivationFrame, 1)}

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range iterations {
			task.PushFrame(types.ActivationFrame{})
			task.PopFrame()
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			if !task.SetTopFrameProgrammer(types.ObjID(7)) {
				t.Error("call stack unexpectedly empty")
				return
			}
		}
	}()
	wg.Wait()

	// Perform one final deterministic update after the concurrent slice growth;
	// the live stack, rather than an abandoned backing array, must be updated.
	task.SetTopFrameProgrammer(types.ObjID(7))
	if got := task.GetCallStack()[0].Programmer; got != types.ObjID(7) {
		t.Fatalf("top-frame programmer = %d, want #7", got)
	}
}
