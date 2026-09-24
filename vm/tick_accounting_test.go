package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Tick charges per construct, matching ToastStunt's COUNT_TICK and
// COUNT_EOP_TICK classification. The managed conformance suite
// (vm/tick_accounting.yaml in moo-conformance-tests) is the Toast-verified
// authority; these cases keep the VM honest without a server. Each program
// measures `a = ticks_left(); <code>; b = ticks_left();`, whose empty form
// costs 2 (the PUT into a and the second ticks_left() call).
func TestTickChargesMatchToast(t *testing.T) {
	cases := []struct {
		name  string
		setup string
		code  string
		want  int64
	}{
		{"baseline", "", "", 2},
		{"assign_immediate", "", "x = 5;", 3},
		{"assign_wide_literal", "", "x = 123456789;", 3},
		{"negative_literals_fold", "", "x = -5; y = -123456789; z = -1.5;", 5},
		{"int_add_fast_path", "y = 7;", "x = y + 2;", 4},
		{"float_add_fast_path", "y = 1.5;", "x = y + 2.5;", 4},
		{"string_add_generic_path", "", `x = "a" + "b";`, 4},
		{"arithmetic", "y = 7;", "x = y - 2; x = y * 2; x = y / 2; x = y % 2;", 10},
		{"comparisons", "y = 2;", "x = y == 2; x = y != 2; x = y < 2; x = y <= 2; x = y > 2; x = y >= 2;", 14},
		{"bitwise_and_power", "y = 6;", "x = y &. 3; x = y |. 3; x = y ^. 3; x = y << 1; x = y >> 1; x = ~y; x = y ^ 2;", 16},
		{"logic", "y = 1;", "x = y && 0; x = y || 0; x = !y;", 8},
		{"ternary", "y = 1;", "x = y ? 2 | 3;", 4},
		{"list_literals", "y = {1, 2};", "x = {}; x = {1}; x = {1, 2, 3}; x = {@y, 3}; x = {3, @y};", 11},
		{"self_append", "v = {};", "v = {@v, 1}; v = {@v};", 6},
		{"map_literal", "", "x = [1 -> 2, 3 -> 4];", 3},
		{"index_and_range", "y = {1, 2, 3};", "x = y[2]; x = y[1..2]; x = y[$];", 8},
		{"index_assignment", "y = {{1, 2}, {3, 4}};", "y[1] = 5; y[2][1] = 5;", 7},
		{"range_assignment", "y = {{1, 2}, {3, 4}};", "y[1..1] = {7}; y[2][1..1] = {9};", 7},
		{"builtin_calls", "", "x = time(); x = max(1, 2, 3);", 7},
		{"builtin_splice_call", "y = {1, 2};", "x = max(@y);", 5},
		{"if_elseif", "y = 0;", "if (y) x = 1; elseif (y) x = 2; else x = 3; endif", 5},
		{"while", "i = 0;", "while (i < 3) i = i + 1; endwhile", 16},
		{"named_while", "i = 0;", "while loop (i < 3) i = i + 1; endwhile", 16},
		{"for_range", "s = 0;", "for z in [1..10] s = s + z; endfor", 33},
		{"for_list", "", "for z in ({1, 2, 3}) endfor", 7},
		{"for_map", "", "for v, k in ([1 -> 2, 3 -> 4]) endfor", 5},
		{"break_continue", "", "for z in [1..3] continue; endfor for z in [1..3] break; endfor", 11},
		{"try_except", "y = 0;", "try x = 1 / y; except e (E_DIV) endtry try x = 1; except (ANY) endtry", 8},
		{"try_finally", "", "try x = 1; finally y = 2; endtry", 5},
		{"catch_expression", "y = 0;", "x = `1 / y ! E_DIV'; x = `1 / y ! ANY => 5';", 10},
		{"scatter", "", "{p, q} = {1, 2}; {p, ?q = 7} = {1}; {p, @r} = {1, 2, 3}; {?p, @r, ?q} = {1};", 11},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := tc.setup + " a = ticks_left(); " + tc.code + " b = ticks_left(); return a - b;"
			result := runBytecodeProgram(t, program, nil, nil)
			requireInt(t, result, tc.want)
		})
	}
}

// A tick count crossing a 1024-tick seconds checkpoint must be charged the
// same whether the fast path or the generic path executes the instruction.
func TestTickChargesAcrossSecondsCheckpoints(t *testing.T) {
	result := runBytecodeProgram(t, "x = 0; a = ticks_left(); for i in [1..1000] x = x + i; endfor b = ticks_left(); return a - b;", nil, nil)
	// 1001 FOR_RANGE tests, 1000 ADDs, 1000 PUTs, plus the baseline 2.
	requireInt(t, result, 3003)
}

// Toast tests the budget before a ticking opcode runs, so the opcode that
// exhausts it never executes.
func TestTickExhaustionStopsBeforeTheChargingOpcode(t *testing.T) {
	runWithLimit := func(limit int64) (types.Result, types.Value) {
		machine, prog := newTickTestVM(t, "x = 0; while (1) x = x + 1; endwhile")
		machine.TickLimit = limit
		result := machine.Run(prog)
		frame := machine.CurrentFrame()
		if frame == nil {
			t.Fatalf("aborted VM kept no frame")
		}
		for i, name := range frame.Program.VarNames {
			if name == "x" {
				return result, frame.Locals[i]
			}
		}
		t.Fatalf("no local x")
		return result, types.None
	}
	// PUT x costs 1 and each iteration 3 (WHILE, ADD, PUT): with 10 ticks the
	// third iteration's PUT reaches zero and is aborted, leaving x = 2.
	result, x := runWithLimit(10)
	if result.Flow != types.FlowException || result.Error != types.E_MAXREC {
		t.Fatalf("result = %v %v; want E_MAXREC", result.Flow, result.Error)
	}
	if x.Type() != types.TYPE_INT || x.Int() != 2 {
		t.Fatalf("x = %v at exhaustion; want 2", x)
	}
}

func newTickTestVM(t *testing.T, code string) (*VM, *bytecode.Program) {
	t.Helper()
	registry := BuildVMRegistry()
	prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}
	machine := NewVM(dbstore.NewStore(), newTestSessionWithTaskManager(registry))
	return machine, prog
}
