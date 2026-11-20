package compiler

import (
	"strings"
	"testing"
)

func TestCompilerDoesNotAllocateUnusedGapBeforeTemporaries(t *testing.T) {
	for _, source := range []string{
		"{a, ?b = 2, @rest} = {1, 2, 3}; return rest;",
		"for i in [1..3] endfor",
		"return `1 / 0 ! ANY';",
		"m = [1 -> 2]; return m;",
	} {
		program, diagnostics := New(nil).CompileMOO([]string{source})
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		if program.NumLocals > 12 {
			t.Errorf("%s: %d locals for %d named variables", source, program.NumLocals, len(program.VarNames))
		}
	}
}

// Sequential scatters reuse their bookkeeping slots, so a verb with many of
// them compiles (issue #687); a nested scatter still gets its own slots.
func TestSequentialScattersReuseTemporaries(t *testing.T) {
	source := strings.Repeat("{a, ?b = ({c, ?d = 1} = {2}), @r} = {3};\n", 100) + "return {a, b, c, d, r};"
	program, diagnostics := New(nil).CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if program.NumLocals > len(program.VarNames)+8 {
		t.Errorf("%d locals for %d named variables", program.NumLocals, len(program.VarNames))
	}
}
