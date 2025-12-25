package types

import "testing"

func TestNewTaskContext(t *testing.T) {
	ctx := NewTaskContext()

	if ctx.TicksRemaining != 30000 {
		t.Errorf("Expected default tick limit of 30000, got %d", ctx.TicksRemaining)
	}

	if ctx.Player != ObjNothing {
		t.Errorf("Expected Player to be ObjNothing, got %v", ctx.Player)
	}

	if ctx.Programmer != ObjNothing {
		t.Errorf("Expected Programmer to be ObjNothing, got %v", ctx.Programmer)
	}

	if ctx.ThisObj != ObjNothing {
		t.Errorf("Expected ThisObj to be ObjNothing, got %v", ctx.ThisObj)
	}

	if ctx.Verb != "" {
		t.Errorf("Expected Verb to be empty, got %q", ctx.Verb)
	}
}

func TestConsumeTick(t *testing.T) {
	ctx := NewTaskContext()
	initialTicks := ctx.TicksRemaining

	// Consume one tick
	ok := ctx.ConsumeTick()
	if !ok {
		t.Error("Expected ConsumeTick to return true when ticks remain")
	}

	if ctx.TicksRemaining != initialTicks-1 {
		t.Errorf("Expected ticks to decrement from %d to %d, got %d",
			initialTicks, initialTicks-1, ctx.TicksRemaining)
	}

	// Consume all remaining ticks
	ctx.TicksRemaining = 1
	ok = ctx.ConsumeTick()
	if ok {
		t.Error("Expected ConsumeTick to return false when ticks exhausted")
	}

	if ctx.TicksRemaining != 0 {
		t.Errorf("Expected ticks to be 0, got %d", ctx.TicksRemaining)
	}
}
