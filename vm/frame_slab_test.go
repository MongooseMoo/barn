package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

// Locals come off a contiguous per-VM stack and are released LIFO; popped
// frame structs are recycled and handed back zeroed with their buffers.
func TestFrameSlabAllocatesAndRecyclesLIFO(t *testing.T) {
	machine := NewVM(nil, nil)
	prog := &bytecode.Program{NumLocals: 3}

	outer := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(3), localsOnStack: true})
	machine.pushFrame(outer)
	outer.Locals[0] = types.NewInt(1)
	inner := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(3), localsOnStack: true})
	machine.pushFrame(inner)
	inner.Locals[0] = types.NewInt(2)
	inner.LoopStack = append(inner.LoopStack, bytecode.LoopState{})

	if len(machine.localStack) != 6 {
		t.Fatalf("localStack len = %d, want 6", len(machine.localStack))
	}
	if outer.Locals[0].Int() != 1 || inner.Locals[0].Int() != 2 {
		t.Fatalf("frames share locals: outer=%v inner=%v", outer.Locals[0], inner.Locals[0])
	}
	for i := 1; i < 3; i++ {
		if inner.Locals[i] != types.Unbound {
			t.Fatalf("fresh local %d = %v, want Unbound", i, inner.Locals[i])
		}
	}

	machine.popFrame()
	if len(machine.localStack) != 3 {
		t.Fatalf("after inner pop localStack len = %d, want 3", len(machine.localStack))
	}
	if len(machine.framePool) != 1 || machine.framePool[0] != inner {
		t.Fatalf("inner frame not recycled")
	}
	if inner.Program != nil || len(inner.Locals) != 0 || len(inner.LoopStack) != 0 || cap(inner.LoopStack) == 0 {
		t.Fatalf("recycled frame not zeroed with buffers kept: %+v", *inner)
	}
	if outer.Locals[0].Int() != 1 {
		t.Fatalf("outer locals clobbered by inner pop: %v", outer.Locals[0])
	}

	// The next frame reuses the recycled struct and the released slots.
	again := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(3), localsOnStack: true})
	if again != inner {
		t.Fatalf("frameFrom did not reuse the pooled frame")
	}
	if len(machine.framePool) != 0 || len(machine.localStack) != 6 {
		t.Fatalf("reuse bookkeeping: pool=%d locals=%d", len(machine.framePool), len(machine.localStack))
	}
	for i := range again.Locals {
		if again.Locals[i] != types.Unbound {
			t.Fatalf("reused local %d = %v, want Unbound", i, again.Locals[i])
		}
	}
	machine.pushFrame(again)
	machine.popFrame()
	machine.popFrame()
	if len(machine.localStack) != 0 || len(machine.framePool) != 2 {
		t.Fatalf("after unwinding: locals=%d pool=%d", len(machine.localStack), len(machine.framePool))
	}
}

// A frame whose Locals are its own slice (restored from a snapshot) pops
// without touching the shared locals stack.
func TestFrameSlabLeavesOwnedLocalsAlone(t *testing.T) {
	machine := NewVM(nil, nil)
	prog := &bytecode.Program{NumLocals: 2}
	stackFrame := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(2), localsOnStack: true})
	machine.pushFrame(stackFrame)
	owned := &StackFrame{Program: prog, Locals: make([]types.Value, 2)}
	machine.pushFrame(owned)
	machine.popFrame()
	if len(machine.localStack) != 2 {
		t.Fatalf("owned-locals pop changed localStack len to %d", len(machine.localStack))
	}
	machine.popFrame()
	if len(machine.localStack) != 0 {
		t.Fatalf("stack-locals pop left len %d", len(machine.localStack))
	}
}

// Growing the locals stack must not disturb frames that already hold slices
// into the previous backing array.
func TestFrameSlabGrowthKeepsOuterLocals(t *testing.T) {
	machine := NewVM(nil, nil)
	prog := &bytecode.Program{NumLocals: 4}
	var frames []*StackFrame
	for depth := 0; depth < 200; depth++ {
		f := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(4), localsOnStack: true})
		f.Locals[0] = types.NewInt(int64(depth))
		machine.pushFrame(f)
		frames = append(frames, f)
	}
	for depth, f := range frames {
		if f.Locals[0].Int() != int64(depth) {
			t.Fatalf("frame %d local = %v after growth", depth, f.Locals[0])
		}
	}
	for range frames {
		machine.popFrame()
	}
	if len(machine.localStack) != 0 {
		t.Fatalf("localStack len %d after full unwind", len(machine.localStack))
	}
	requireFreshLocalsUnbound(t, machine, "after growth and unwind", 200*4)
}

// requireFreshLocalsUnbound allocates n locals and requires every one Unbound.
func requireFreshLocalsUnbound(t *testing.T, machine *VM, what string, n int) {
	t.Helper()
	for i, local := range machine.allocLocals(n) {
		if local != types.Unbound {
			t.Fatalf("%s: fresh local %d = %v, want Unbound", what, i, local)
		}
	}
}

// Locals handed out again are Unbound however their slots were last used: by
// a popped frame, by a frame whose locals were left in the array the stack
// grew out of, by a frame killed with its VM reset for the pool, and by a
// retry checkpoint restored over the VM.
func TestFreshLocalsAreUnboundAfterEveryRelease(t *testing.T) {
	prog := &bytecode.Program{NumLocals: 4}
	push := func(machine *VM) *StackFrame {
		f := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(prog.NumLocals), localsOnStack: true})
		for i := range f.Locals {
			f.Locals[i] = types.NewInt(int64(100 + i))
		}
		machine.pushFrame(f)
		return f
	}

	machine := NewVM(nil, nil)
	push(machine)
	machine.popFrame()
	requireFreshLocalsUnbound(t, machine, "after a pop", prog.NumLocals)
	machine.localStack = machine.localStack[:0]

	// The outer frame's slots are copied into the grown array; its locals
	// stay in the old one. Popping it must still leave the grown array's
	// slots Unbound.
	machine = NewVM(nil, nil)
	outer := push(machine)
	before := &machine.localStack[:1][0]
	inner := machine.frameFrom(StackFrame{Program: prog, Locals: machine.allocLocals(initialStackCap * 2), localsOnStack: true})
	machine.pushFrame(inner)
	if &machine.localStack[0] == before || &outer.Locals[0] != before {
		t.Fatal("the inner frame did not grow the locals stack")
	}
	machine.popFrame()
	machine.popFrame()
	requireFreshLocalsUnbound(t, machine, "after popping a frame left in the old array", initialStackCap*2+prog.NumLocals)

	machine = NewVM(nil, nil)
	push(machine)
	push(machine)
	machine.reset()
	requireFreshLocalsUnbound(t, machine, "after reset", 2*prog.NumLocals)

	machine = NewVM(nil, nil)
	push(machine)
	machine.yielded = true // only a yielded VM takes a retry checkpoint
	checkpoint := machine.CheckpointForRetry()
	push(machine)
	checkpoint.Restore(machine)
	push(machine)
	machine.popFrame()
	requireFreshLocalsUnbound(t, machine, "after a retry restore", 2*prog.NumLocals)
}
