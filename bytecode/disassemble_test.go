package bytecode_test

import (
	"strings"
	"testing"

	"barn/bytecode"
	"barn/compiler"
)

func TestDisassembleDecodesCompiledProgram(t *testing.T) {
	program, diagnostics := compiler.CompileMOO([]string{"x = 1;", "return x;"}, stubRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}
	lines := bytecode.Disassemble(program)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Language version number: 17",
		"First line number: 1",
		"Main code vector:",
		"SET_VAR",
		"GET_VAR",
		"RETURN",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("disassembly missing %q:\n%s", want, joined)
		}
	}
}
