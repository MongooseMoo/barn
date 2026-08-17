package vm

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"reflect"
	"testing"
)

// dirtyVM sets every field of a VM to a detectably non-zero value. The
// completeness of this function is asserted by reflection below, so adding a
// field to VM without teaching dirtyVM about it fails the test rather than
// silently leaving a reset hole.
func dirtyVM(machine *VM) {
	machine.Stack = append(machine.Stack, types.NewStr("dirty"), types.NewInt(7))
	machine.SP = 2
	machine.Frames = append(machine.Frames, &StackFrame{Verb: "dirty"})
	machine.FP = 3
	machine.Store = dbstore.NewStore()
	machine.Builtins = newTestSession(BuildVMRegistry())
	machine.Context = kernel.NewTaskContext()
	machine.Task = task.NewTask(1, 0, 1, 1)
	machine.TickLimit = 123
	machine.MaxStackDepth = 456
	machine.Ticks = 789
	machine.PendingWaifs = []types.Value{types.NewInt(1)}
	machine.PendingFinalizations = []types.Value{types.NewInt(2)}
	machine.frame = machine.Frames[0]
	machine.yielded = true
	machine.yieldResult = types.Result{Flow: types.FlowSuspend}
	machine.resumeError = types.E_INTRPT
}

func TestDirtyVMTouchesEveryField(t *testing.T) {
	machine := NewVM(nil, nil)
	dirtyVM(machine)

	value := reflect.ValueOf(*machine)
	for i := 0; i < value.NumField(); i++ {
		if value.Field(i).IsZero() {
			t.Fatalf("dirtyVM left VM field %q zero — teach dirtyVM (and (*VM).reset) about it",
				value.Type().Field(i).Name)
		}
	}
}

func TestResetRestoresFreshVMState(t *testing.T) {
	machine := NewVM(nil, nil)
	dirtyVM(machine)
	machine.reset()

	fresh := NewVM(nil, nil)
	got := reflect.ValueOf(*machine)
	want := reflect.ValueOf(*fresh)
	for i := 0; i < got.NumField(); i++ {
		field := got.Type().Field(i)
		name := field.Name
		if !field.IsExported() {
			// Unexported fields cannot be read through Interface(); asserted
			// explicitly in TestResetClearsUnexportedFields.
			continue
		}
		switch name {
		case "Stack", "Frames":
			// Compared by length below; capacity is deliberately retained.
			continue
		}
		if !reflect.DeepEqual(got.Field(i).Interface(), want.Field(i).Interface()) {
			t.Errorf("field %q after reset = %#v, want fresh-VM value %#v",
				name, got.Field(i).Interface(), want.Field(i).Interface())
		}
	}
	if len(machine.Stack) != 0 || len(machine.Frames) != 0 {
		t.Errorf("reset left Stack len %d, Frames len %d; want 0, 0", len(machine.Stack), len(machine.Frames))
	}
	if machine.frame != nil {
		t.Error("reset left the cached frame pointer set")
	}
	if machine.IsYielded() {
		t.Error("reset left the VM yielded")
	}
}

// The unexported fields are not reachable through Interface(), so they get their
// own explicit assertions.
func TestResetClearsUnexportedFields(t *testing.T) {
	machine := NewVM(nil, nil)
	dirtyVM(machine)
	machine.reset()

	if machine.frame != nil {
		t.Error("frame not cleared")
	}
	if machine.yielded {
		t.Error("yielded not cleared")
	}
	if machine.yieldResult.Flow != (types.Result{}).Flow || machine.yieldResult.ForkInfo != nil {
		t.Errorf("yieldResult not cleared: %#v", machine.yieldResult)
	}
	if machine.resumeError != types.E_NONE {
		t.Errorf("resumeError not cleared: %v", machine.resumeError)
	}
}

// A pooled VM must not keep the values it held alive: the backing arrays are
// reused, but until they are overwritten they would otherwise pin whole object
// graphs referenced from the operand stack.
func TestResetScrubsBackingArrays(t *testing.T) {
	machine := NewVM(nil, nil)
	machine.Push(types.NewStr("retained"))
	machine.Push(types.NewList([]types.Value{types.NewInt(1)}))
	machine.pushFrame(&StackFrame{Verb: "retained"})

	stackArray := machine.Stack[:len(machine.Stack):cap(machine.Stack)]
	framesArray := machine.Frames[:len(machine.Frames):cap(machine.Frames)]

	machine.reset()

	for i := range stackArray {
		if stackArray[i] != (types.Value{}) {
			t.Errorf("stack slot %d still holds %v after reset", i, stackArray[i])
		}
	}
	for i := range framesArray {
		if framesArray[i] != nil {
			t.Errorf("frame slot %d still holds a frame after reset", i)
		}
	}
}

func TestAcquireVMReturnsCleanVM(t *testing.T) {
	store := dbstore.NewStore()
	registry := BuildVMRegistry()
	session := newTestSession(registry)

	machine := AcquireVM(store, session)
	dirtyVM(machine)
	machine.yielded = false // a yielded VM is deliberately not pooled
	ReleaseVM(machine)

	reused := AcquireVM(store, session)
	if reused.SP != 0 || reused.FP != 0 || reused.Ticks != 0 {
		t.Errorf("reused VM dirty: SP=%d FP=%d Ticks=%d", reused.SP, reused.FP, reused.Ticks)
	}
	if len(reused.Stack) != 0 || len(reused.Frames) != 0 {
		t.Errorf("reused VM dirty: len(Stack)=%d len(Frames)=%d", len(reused.Stack), len(reused.Frames))
	}
	if reused.Context != nil || reused.PendingWaifs != nil || reused.PendingFinalizations != nil || reused.frame != nil {
		t.Error("reused VM still carries context/pending finalizations/frame")
	}
	if reused.TickLimit != defaultTickLimit || reused.MaxStackDepth != defaultMaxStackDepth {
		t.Errorf("reused VM limits = %d/%d, want %d/%d",
			reused.TickLimit, reused.MaxStackDepth, defaultTickLimit, defaultMaxStackDepth)
	}
	if reused.Store != store || reused.Builtins != session {
		t.Error("AcquireVM did not install the caller's store/registry")
	}
}

func TestReleaseVMDeclinesUnsafeVMs(t *testing.T) {
	t.Run("yielded", func(t *testing.T) {
		machine := NewVM(nil, nil)
		machine.yielded = true
		ReleaseVM(machine)
		if !machine.yielded {
			t.Error("ReleaseVM reset a yielded VM; it may still be resumed by its holder")
		}
	})

	t.Run("oversized stack", func(t *testing.T) {
		machine := NewVM(nil, nil)
		machine.Stack = make([]types.Value, 0, maxPooledStackCap+1)
		machine.Ticks = 42
		ReleaseVM(machine)
		if machine.Ticks != 42 {
			t.Error("ReleaseVM pooled a VM with an oversized stack")
		}
	})

	t.Run("oversized frames", func(t *testing.T) {
		machine := NewVM(nil, nil)
		machine.Frames = make([]*StackFrame, 0, maxPooledFramesCap+1)
		machine.Ticks = 42
		ReleaseVM(machine)
		if machine.Ticks != 42 {
			t.Error("ReleaseVM pooled a VM with an oversized frame slice")
		}
	})

	t.Run("nil", func(t *testing.T) {
		ReleaseVM(nil) // must not panic
	})
}

// Reusing a VM's operand stack must not corrupt a result already returned to the
// caller. This is the classic pool bug: if a Result ever carried a slice of
// vm.Stack rather than a copy, the next user of the pooled VM would overwrite it
// in place.
func TestPooledStackReuseDoesNotCorruptPriorResult(t *testing.T) {
	store := dbstore.NewStore()
	registry := BuildVMRegistry()
	session := newTestSession(registry)

	run := func(code string) types.Result {
		ctx := kernel.NewTaskContext()
		ctx.Store = store

		prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
		if len(diagnostics) > 0 {
			t.Fatalf("compile failed: %v", diagnostics)
		}
		machine := AcquireVM(store, session)
		machine.Context = ctx
		machine.Task = task.NewTask(1, types.ObjID(0), 30000, 1)
		result := machine.Run(prog)
		ReleaseVM(machine)
		return result
	}

	first := run(`return {1, "alpha", {2, "beta"}};`)
	if first.Flow != types.FlowReturn {
		t.Fatalf("first run flow = %v, want FlowReturn", first.Flow)
	}
	before := first.Val.String()

	// Re-run several times through the pool with values that would land in the
	// same stack slots, then check the first result is untouched.
	for i := 0; i < 8; i++ {
		if got := run(`return {99, "zzzzz", {98, "yyyyy"}};`); got.Flow != types.FlowReturn {
			t.Fatalf("reuse run %d flow = %v", i, got.Flow)
		}
	}

	if after := first.Val.String(); after != before {
		t.Errorf("earlier result mutated by pooled VM reuse: %s -> %s", before, after)
	}
}
