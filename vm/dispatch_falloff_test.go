package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
)

// These tests pin the behavior that the C5 dispatch-loop change relies on: the
// compiler emits a terminal frame-popping opcode (OP_RETURN / OP_RETURN_NONE) at
// the end of EVERY program, so a program that "falls off the end" with no explicit
// trailing return still yields the MOO default of 0. The executeLoop hot path drops
// the per-opcode end-of-code bounds check and depends on this invariant; if it ever
// regressed, an un-terminated program would read past Code (OOB) instead of
// returning 0. Keep these green.

// A verb body with no explicit return falls off the end -> implicit return 0.
func TestFallOffEndReturnsZero(t *testing.T) {
	result := runBytecodeProgram(t, "x = 5; y = x + 10;", nil, nil)
	requireInt(t, result, 0)
}

// An empty program -> implicit return 0.
func TestEmptyProgramReturnsZero(t *testing.T) {
	result := runBytecodeProgram(t, "", nil, nil)
	requireInt(t, result, 0)
}

// A single expression statement with no return falls off the end -> 0
// (the expression result is popped; the implicit terminator returns 0).
func TestExprStmtNoReturnFallsOffToZero(t *testing.T) {
	result := runBytecodeProgram(t, "1 + 2;", nil, nil)
	requireInt(t, result, 0)
}

// Structural guard for the terminator invariant: every compiled program must end
// in a terminal frame-popping opcode (OP_RETURN or OP_RETURN_NONE). This is what
// lets executeLoop drop the per-op bounds check. If a future compiler change stops
// emitting the terminator, this fails loudly instead of producing an OOB read.
func TestEveryCompiledProgramEndsWithTerminator(t *testing.T) {
	cases := []string{
		"",                              // empty
		"x = 5;",                        // assignment, no return
		"1 + 2;",                        // bare expr
		"return 42;",                    // explicit return
		"for i in [1..3] x = i; endfor", // loop as last statement
		"if (1) x = 2; endif",           // conditional
	}
	registry := BuildVMRegistry()
	for _, src := range cases {
		prog, diagnostics := compiler.CompileMOO([]string{src}, registry)
		if len(diagnostics) > 0 {
			t.Fatalf("src %q: compile failed: %v", src, diagnostics)
		}
		if len(prog.Code) == 0 {
			t.Fatalf("src %q: empty Code, expected a terminator opcode", src)
		}
		last := bytecode.OpCode(prog.Code[len(prog.Code)-1])
		if last != bytecode.OP_RETURN && last != bytecode.OP_RETURN_NONE {
			t.Fatalf("src %q: last opcode = %s, want OP_RETURN or OP_RETURN_NONE", src, last)
		}
	}
}
