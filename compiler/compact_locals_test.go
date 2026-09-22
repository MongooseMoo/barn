package compiler

import "testing"

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
