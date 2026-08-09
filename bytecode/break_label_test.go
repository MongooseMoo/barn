package bytecode_test

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/compiler"
)

type stubRegistry struct{}

func (stubRegistry) GetID(string) (int, bool) { return 0, false }

// F21: `break ID;` must resolve/validate its loop name exactly like `continue ID;`.
// ToastStunt's check_loop_name (parser.y:1187-1209) raises
// "Invalid loop name in `break' statement" for an unknown name, identical to
// the continue path. Barn previously stuffed the identifier into BreakStmt.Value
// and silently compiled `break nonexistent;` as a break-with-value, never erroring.

// TestBreakUnknownLoopNameIsCompileError proves `break nonexistent;` is now a
// compile error, mirroring continue.
func TestBreakUnknownLoopNameIsCompileError(t *testing.T) {
	src := []string{
		"while (1)",
		"break nonexistent;",
		"endwhile",
		"return 0;",
	}
	_, diagnostics := compiler.CompileMOO(src, stubRegistry{})
	if len(diagnostics) == 0 {
		t.Fatalf("BUG: `break nonexistent;` compiled without error; want \"Invalid loop name\"")
	}
	if !strings.Contains(diagnostics[0].Message, "invalid loop name") {
		t.Fatalf("got diagnostic %q, want it to contain \"invalid loop name\"", diagnostics[0].Message)
	}
}

// TestContinueUnknownLoopNameIsCompileError is the parity baseline: continue
// already errored on an unknown loop name. break must match this.
func TestContinueUnknownLoopNameIsCompileError(t *testing.T) {
	src := []string{
		"while (1)",
		"continue nonexistent;",
		"endwhile",
		"return 0;",
	}
	_, diagnostics := compiler.CompileMOO(src, stubRegistry{})
	if len(diagnostics) == 0 {
		t.Fatalf("`continue nonexistent;` compiled without error; want \"Invalid loop name\"")
	}
	if !strings.Contains(diagnostics[0].Message, "invalid loop name") {
		t.Fatalf("got diagnostic %q, want it to contain \"invalid loop name\"", diagnostics[0].Message)
	}
}

// TestLabeledBreakAndContinueCompile confirms valid labeled break/continue
// (targeting the enclosing loop variable name) still compile cleanly.
func TestLabeledBreakAndContinueCompile(t *testing.T) {
	for _, src := range [][]string{
		{"for i in ({1, 2})", "break i;", "endfor", "return 0;"},
		{"for i in ({1, 2})", "continue i;", "endfor", "return 0;"},
		{"while (1)", "break;", "endwhile", "return 0;"},
	} {
		if _, diagnostics := compiler.CompileMOO(src, stubRegistry{}); len(diagnostics) > 0 {
			t.Fatalf("valid loop %v failed to compile: %v", src, diagnostics)
		}
	}
}
