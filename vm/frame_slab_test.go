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
}
