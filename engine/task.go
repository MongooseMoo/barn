package engine

import (
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// Task owns an executable MOO task. The interpreter and builtins share its
// inline State; only the engine owns the saved interpreter and catalog binding.
type Task struct {
	task.State
	machine *vm.VM
	catalog *TaskManager
}

func NewTask(id int64, owner types.ObjID, ticks int64, seconds float64) *Task {
	t := &Task{}
	t.Init(id, owner, ticks, seconds)
	t.BindSchedulingLocked(t, nil)
	return t
}

func NewTaskFull(id int64, owner types.ObjID, program *bytecode.Program, ticks int64, seconds float64) *Task {
	t := NewTask(id, owner, ticks, seconds)
	t.Program = program
	t.Context = task.NewContext()
	t.Context.Player = owner
	t.Context.Programmer = owner
	t.Context.TicksRemaining = ticks
	return t
}

// VM loads the saved interpreter under the same lock as task state. It is a
// leaf operation: callers may hold runtime/catalog locks. Inspecting the VM's
// frames still requires the runtime's execution lease or GC quiescence.
func (t *Task) VM() *vm.VM {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.machine
}

func (t *Task) SetVM(machine *vm.VM) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.machine = machine
}

// TaskSettled updates the existing unfinished index while State.Mu is held.
// It must never acquire the catalog mutex or a runtime lock.
func (t *Task) TaskSettled(settled bool) {
	if t.catalog != nil {
		t.catalog.noteFinished(t, settled)
	}
}

// PersistenceSnapshot captures state and saved VM in one lock interval. The
// caller must separately establish that execution cannot mutate VM frames.
func (t *Task) PersistenceSnapshot() task.Snapshot {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	snapshot := t.PersistenceSnapshotLocked()
	if t.machine != nil {
		snapshot.VM = t.machine.PersistenceVMSnapshot()
	}
	return snapshot
}
