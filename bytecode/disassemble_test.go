package bytecode_test

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
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

func TestDisassembleUsesToastOperatorMnemonics(t *testing.T) {
	program, diagnostics := compiler.CompileMOO([]string{
		`"foobar"[^..$];`,
		`~123;`,
		`1 << 2;`,
		`1 >> 2;`,
	}, stubRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}

	joined := strings.Join(bytecode.Disassemble(program), "\n")
	for _, want := range []string{"FIRST", "LAST", "COMPLEMENT", "BITSHL", "BITSHR"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("disassembly missing %q:\n%s", want, joined)
		}
	}
}

func TestDisassembleDecodesEndFinallyHandlerOperands(t *testing.T) {
	program, diagnostics := compiler.CompileMOO([]string{
		"try",
		"  x = 1;",
		"finally",
		"  x = 2;",
		"endtry",
		"return x;",
	}, stubRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}

	var tryOperands []string
	var endOperands [][]string
	for _, line := range bytecode.Disassemble(program) {
		fields := strings.Fields(line)
		if strings.HasSuffix(line, "TRY_FINALLY") {
			if len(fields) != 5 {
				t.Fatalf("TRY_FINALLY disassembly = %q, want opcode and two operands", line)
			}
			tryOperands = fields[2:4]
		}
		if strings.HasSuffix(line, "END_FINALLY") {
			if len(fields) != 5 {
				t.Fatalf("END_FINALLY disassembly = %q, want opcode and two operands", line)
			}
			endOperands = append(endOperands, fields[2:4])
		}
	}
	if len(tryOperands) != 2 {
		t.Fatalf("TRY_FINALLY operands = %v, want two bytes", tryOperands)
	}
	if len(endOperands) != 2 {
		t.Fatalf("END_FINALLY instructions = %d, want 2", len(endOperands))
	}
	for i, operands := range endOperands {
		if strings.Join(operands, " ") != strings.Join(tryOperands, " ") {
			t.Fatalf("END_FINALLY %d operands = %v, want matching handler %v", i, operands, tryOperands)
		}
	}
}
