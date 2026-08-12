package bytecode_test

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
)

func TestDisassembleDecodesCompiledProgram(t *testing.T) {
	program, diagnostics := compiler.New(nil).CompileMOO([]string{"x = 1;", "return x;"})
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

func TestDisassembleCompactCallVerbRetainsFollowingInstruction(t *testing.T) {
	program, diagnostics := compiler.New(nil).CompileMOO([]string{`return #0:foo();`})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}

	lines := bytecode.Disassemble(program)
	var callLine int
	for i, line := range lines {
		if strings.HasSuffix(line, "CALL_VERB") {
			callLine = i
			fields := strings.Fields(line)
			if len(fields) != 5 {
				t.Fatalf("CALL_VERB disassembly = %q, want opcode and two operands", line)
			}
			break
		}
	}
	if callLine == 0 {
		t.Fatalf("disassembly missing CALL_VERB:\n%s", strings.Join(lines, "\n"))
	}
	if callLine+1 >= len(lines) {
		t.Fatalf("CALL_VERB is the last disassembly row, want following RETURN\n%s", strings.Join(lines, "\n"))
	}
	if !strings.HasSuffix(lines[callLine+1], "RETURN") {
		t.Fatalf("instruction after CALL_VERB = %q, want RETURN\n%s", lines[callLine+1], strings.Join(lines, "\n"))
	}
}

func TestExtractCompiledForkRebasesHandlersAfterCompactCallVerb(t *testing.T) {
	program, diagnostics := compiler.New(nil).CompileMOO([]string{
		"fork (0)",
		"  x = #0:foo();",
		"  try",
		"    y = 1;",
		"  finally",
		"    y = 2;",
		"  endtry",
		"endfork",
		"return 0;",
	})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}

	forkIP := -1
	for _, line := range bytecode.Disassemble(program) {
		if strings.HasSuffix(line, "FORK_WIDE") {
			if _, err := fmt.Sscanf(line, "%d:", &forkIP); err != nil {
				t.Fatalf("parse fork IP from %q: %v", line, err)
			}
			break
		}
	}
	if forkIP < 0 {
		t.Fatalf("disassembly missing FORK_WIDE:\n%s", strings.Join(bytecode.Disassemble(program), "\n"))
	}
	const forkOperands = 5
	bodyIP := forkIP + 1 + forkOperands
	bodyLen := int(binary.BigEndian.Uint32(program.Code[forkIP+2 : forkIP+6]))
	child := program.ExtractForkBody(bodyIP, bodyLen)
	if child == nil {
		t.Fatal("ExtractForkBody rejected compiler-produced fork body")
	}

	var targets []int
	for _, line := range bytecode.Disassemble(child) {
		if !strings.HasSuffix(line, "TRY_FINALLY_WIDE") && !strings.HasSuffix(line, "END_FINALLY_WIDE") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 7 {
			t.Fatalf("handler disassembly = %q, want opcode and four operands", line)
		}
		var target int
		for _, field := range fields[2:6] {
			value, err := strconv.Atoi(field)
			if err != nil {
				t.Fatalf("parse handler operand %q: %v", field, err)
			}
			target = target<<8 | value
		}
		targets = append(targets, target)
	}
	if len(targets) < 2 {
		t.Fatalf("handler targets = %v, want TRY_FINALLY and END_FINALLY targets", targets)
	}
	for _, target := range targets[1:] {
		if target != targets[0] {
			t.Fatalf("handler targets = %v, want every target rebased to %d", targets, targets[0])
		}
	}
	if targets[0] < 0 || targets[0] >= bodyLen {
		t.Fatalf("rebased handler target = %d, want child coordinate within [0, %d)", targets[0], bodyLen)
	}
}

func TestDisassembleUsesToastOperatorMnemonics(t *testing.T) {
	program, diagnostics := compiler.New(nil).CompileMOO([]string{
		`"foobar"[^..$];`,
		`~123;`,
		`1 << 2;`,
		`1 >> 2;`,
	})
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
	program, diagnostics := compiler.New(nil).CompileMOO([]string{
		"try",
		"  x = 1;",
		"finally",
		"  x = 2;",
		"endtry",
		"return x;",
	})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}

	var tryOperands []string
	var endOperands [][]string
	for _, line := range bytecode.Disassemble(program) {
		fields := strings.Fields(line)
		if strings.HasSuffix(line, "TRY_FINALLY_WIDE") {
			if len(fields) != 7 {
				t.Fatalf("TRY_FINALLY_WIDE disassembly = %q, want opcode and four operands", line)
			}
			tryOperands = fields[2:6]
		}
		if strings.HasSuffix(line, "END_FINALLY_WIDE") {
			if len(fields) != 7 {
				t.Fatalf("END_FINALLY_WIDE disassembly = %q, want opcode and four operands", line)
			}
			endOperands = append(endOperands, fields[2:6])
		}
	}
	if len(tryOperands) != 4 {
		t.Fatalf("TRY_FINALLY_WIDE operands = %v, want four bytes", tryOperands)
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
