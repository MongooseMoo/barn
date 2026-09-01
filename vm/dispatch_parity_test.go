package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// The dispatch fast path in executeLoop handles the hottest opcodes inline and
// falls through to Execute for everything else. Step drives Execute only, so
// running the same program both ways and comparing the observable end state
// (result, ticks, locals, operand stack) pins the two paths to each other.
//
// The corpus deliberately exercises every fast-path guard's negative branch:
// unbound variables, non-int arithmetic, float overflow, the MaxInt64 range
// end, object ranges, finalizable overwrites, and empty-stack shapes never
// appear in compiled code but are covered by the SP guards.
var dispatchParityCorpus = []struct {
	name string
	code string
}{
	{"int_loop", "x = 0; for i in [1..500]; x = x + i; endfor; return x;"},
	{"nested_loop", "c = 0; for i in [1..30]; for j in [1..30]; c = c + 1; endfor; endfor; return c;"},
	{"float_loop", "x = 0.0; for i in [1..200]; x = x + 1.5; endfor; return x;"},
	{"mixed_add_promotes_or_errors", "x = 1; return x + 2.5;"},
	{"string_add", `s = "a"; for i in [1..50]; s = s + "b"; endfor; return s;`},
	{"list_build", "l = {}; for i in [1..100]; l = {@l, i}; endfor; return l;"},
	{"list_index", "l = {1, 2, 3, 4, 5}; x = 0; for i in [1..100]; x = x + l[1 + (i % 5)]; endfor; return x;"},
	{"while_loop", "i = 0; while (i < 100) i = i + 1; endwhile; return i;"},
	{"if_chain", "c = 0; for i in [1..20]; if (i % 2 == 0) c = c + 1; elseif (i > 5) c = c - 1; else c = c * 2; endif; endfor; return c;"},
	{"for_in_list", "s = 0; for x in ({1, 2, 3, 4, 5, 6}); s = s + x; endfor; return s;"},
	{"unbound_var", "for i in [1..3]; x = y; endfor; return x;"},
	{"int_plus_string_errors", `x = 1; for i in [1..3]; x = x + "a"; endfor; return x;`},
	{"float_overflow_errors", "x = 1.0e308; for i in [1..5]; x = x + x; endfor; return x;"},
	{"maxint_range_end", "c = 0; for i in [9223372036854775805..9223372036854775807]; c = c + 1; endfor; return c;"},
	{"object_range", "c = 0; for o in [#1..#5]; c = c + 1; endfor; return c;"},
	{"empty_range", "c = 0; for i in [5..1]; c = c + 1; endfor; return c;"},
	{"negative_step_via_sub", "x = 100; for i in [1..10]; x = x - i; endfor; return x;"},
	{"overwrite_list_local", "l = {1, 2}; for i in [1..20]; l = {i}; endfor; return l;"},
	{"nested_call_in_loop", "x = 0; for i in [1..50]; x = x + abs(-i); endfor; return x;"},
	{"break_continue", "c = 0; for i in [1..50]; if (i == 10) break; endif; if (i % 2) continue; endif; c = c + i; endfor; return c;"},
	{"try_in_loop", "c = 0; for i in [1..10]; try; c = c + i / (i - 5); except (E_DIV); c = c - 1; endtry; endfor; return c;"},
	{"return_in_loop", "for i in [1..100]; if (i == 42) return i; endif; endfor; return -1;"},
}

type parityState struct {
	flow   types.ControlFlow
	errc   types.ErrorCode
	val    types.Value
	ticks  int64
	locals []types.Value
	stack  []types.Value
}

func newParityVM(t *testing.T) (*VM, func(string) *bytecode.Program) {
	t.Helper()
	registry := BuildVMRegistry()
	store := dbstore.NewStore()
	compile := func(code string) *bytecode.Program {
		prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
		if len(diagnostics) > 0 {
			t.Fatalf("compile failed: %v", diagnostics)
		}
		return prog
	}
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.TicksRemaining = 1 << 40
	m := NewVM(store, newTestSession(registry))
	m.Context = ctx
	m.Task = task.NewTask(1, types.ObjID(0), ctx.TicksRemaining, 1)
	m.TickLimit = 1 << 40
	return m, compile
}

func snapshotLocals(m *VM, frame *StackFrame) []types.Value {
	if frame == nil {
		return nil
	}
	out := make([]types.Value, len(frame.Locals))
	copy(out, frame.Locals)
	return out
}

// runFast executes prog through Run (fast path + generic fallback).
func runFast(t *testing.T, code string) parityState {
	t.Helper()
	m, compile := newParityVM(t)
	prog := compile(code)
	var initial *StackFrame
	m.beginProgram(prog)
	initial = m.frame
	res := m.executeLoop()
	return parityState{
		flow:   res.Flow,
		errc:   res.Error,
		val:    res.Val,
		ticks:  m.Ticks,
		locals: snapshotLocals(m, initial),
		stack:  append([]types.Value(nil), m.Stack[:m.SP]...),
	}
}

// runStepped executes prog one Execute at a time, bypassing the fast path.
// Errors are reported as the raw error code at the point Step returns them;
// Run reports the same code after unwinding, unless the program catches it.
func runStepped(t *testing.T, code string) parityState {
	t.Helper()
	m, compile := newParityVM(t)
	prog := compile(code)
	m.beginProgram(prog)
	initial := m.frame
	for m.frame != nil {
		if err := m.Step(); err != nil {
			handled, exceptionValue := m.HandleError(err)
			if handled {
				continue
			}
			return parityState{
				flow:   types.FlowException,
				errc:   extractErrorCode(err),
				val:    exceptionValue,
				ticks:  m.Ticks,
				locals: snapshotLocals(m, initial),
				stack:  append([]types.Value(nil), m.Stack[:m.SP]...),
			}
		}
		if m.yielded {
			t.Fatalf("parity corpus must not yield")
		}
	}
	st := parityState{flow: types.FlowReturn, ticks: m.Ticks, locals: snapshotLocals(m, initial)}
	if m.SP > 0 {
		st.val = m.Pop()
	} else {
		st.val = types.NewInt(0)
	}
	st.stack = append([]types.Value(nil), m.Stack[:m.SP]...)
	return st
}

func TestDispatchFastPathParity(t *testing.T) {
	for _, c := range dispatchParityCorpus {
		t.Run(c.name, func(t *testing.T) {
			fast := runFast(t, c.code)
			slow := runStepped(t, c.code)
			if fast.flow != slow.flow || fast.errc != slow.errc {
				t.Fatalf("flow/error differ: fast=%v/%v slow=%v/%v", fast.flow, fast.errc, slow.flow, slow.errc)
			}
			if fast.flow == types.FlowReturn && !fast.val.Equal(slow.val) {
				t.Fatalf("result differs: fast=%v slow=%v", fast.val, slow.val)
			}
			if fast.ticks != slow.ticks {
				t.Fatalf("ticks differ: fast=%d slow=%d", fast.ticks, slow.ticks)
			}
			if len(fast.locals) != len(slow.locals) {
				t.Fatalf("local count differs: %d vs %d", len(fast.locals), len(slow.locals))
			}
			for i := range fast.locals {
				if fast.locals[i].IsUnbound() != slow.locals[i].IsUnbound() ||
					(!fast.locals[i].IsUnbound() && !fast.locals[i].Equal(slow.locals[i])) {
					t.Fatalf("local %d differs: fast=%v slow=%v", i, fast.locals[i], slow.locals[i])
				}
			}
			if len(fast.stack) != len(slow.stack) {
				t.Fatalf("operand stack depth differs: %d vs %d", len(fast.stack), len(slow.stack))
			}
		})
	}
}

// The fast-path back-edges charge ticks and enforce the limit themselves;
// the raise must land on the exact tick and leave the loop variable where the
// generic path would.
func TestDispatchFastPathTickLimitExact(t *testing.T) {
	for _, c := range []struct {
		name  string
		code  string
		limit int64
	}{
		{"for_range", "c = 0; for i in [1..1000]; c = c + 1; endfor; return c;", 100},
		{"while", "i = 0; while (i < 1000) i = i + 1; endwhile; return i;", 100},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, compile := newParityVM(t)
			m.TickLimit = c.limit
			m.Context.TicksRemaining = c.limit
			res := m.Run(compile(c.code))
			if res.Flow != types.FlowException || res.Error != types.E_MAXREC {
				t.Fatalf("expected E_MAXREC, got %v/%v", res.Flow, res.Error)
			}
			if m.Ticks != c.limit {
				t.Fatalf("raised at tick %d, want exactly %d", m.Ticks, c.limit)
			}
			if m.Context.TicksRemaining != 0 {
				t.Fatalf("context TicksRemaining = %d, want 0", m.Context.TicksRemaining)
			}
		})
	}
}
