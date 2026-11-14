package bytecode

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// widePushProgram pushes the constant at index and returns it, from a pool of
// poolSize string constants.
func widePushProgram(poolSize, index int) *Program {
	constants := make([]types.Value, poolSize)
	for i := range constants {
		constants[i] = types.NewStr("k")
	}
	return &Program{
		Code:      []byte{byte(OP_PUSH_WIDE), byte(index >> 8), byte(index), byte(OP_RETURN)},
		Constants: constants,
	}
}

func TestPushWideVerifiesAgainstTheWholePool(t *testing.T) {
	program := widePushProgram(257, 256)
	heights, err := StackHeights(program)
	if err != nil {
		t.Fatalf("StackHeights() error = %v", err)
	}
	if heights[0] != 0 || heights[3] != 1 {
		t.Fatalf("stack heights = %v, want 0 before the push and 1 after it", heights)
	}
	if !program.IsInstructionBoundary(0) || program.IsInstructionBoundary(1) || program.IsInstructionBoundary(2) || !program.IsInstructionBoundary(3) {
		t.Fatal("PUSH_WIDE does not occupy exactly its opcode and two operand bytes")
	}
	if checked, unchecked := InstructionTicks(OP_PUSH_WIDE, program.Code, 0); checked != 0 || unchecked != 0 {
		t.Fatalf("PUSH_WIDE ticks = %d checked, %d unchecked; want none, like PUSH", checked, unchecked)
	}
	if disassembly := strings.Join(Disassemble(program), "\n"); !strings.Contains(disassembly, "PUSH_WIDE") {
		t.Fatalf("Disassemble() omitted PUSH_WIDE:\n%s", disassembly)
	}
}

func TestPushWideRejectsMalformedOperands(t *testing.T) {
	tests := []struct {
		name    string
		program *Program
		want    string
	}{
		{"index past the pool", widePushProgram(257, 257), "constant index 257"},
		{"truncated operand", &Program{Code: []byte{byte(OP_PUSH_WIDE), 0}}, "truncated operands for PUSH_WIDE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyProgram(tt.program)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("VerifyProgram() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestForkBodyKeepsPushWideOperands(t *testing.T) {
	parent := widePushProgram(257, 256)
	parent.Code = []byte{byte(OP_RETURN_NONE), byte(OP_PUSH_WIDE), 1, 0, byte(OP_POP)}
	child := parent.ExtractForkBody(1, 4)
	if child == nil {
		t.Fatal("ExtractForkBody rejected a body holding PUSH_WIDE")
	}
	want := []byte{byte(OP_PUSH_WIDE), 1, 0, byte(OP_POP), byte(OP_RETURN_NONE)}
	if string(child.Code) != string(want) {
		t.Fatalf("fork body code = %v, want %v", child.Code, want)
	}
	if parent.ExtractForkBody(2, 3) != nil {
		t.Fatal("ExtractForkBody accepted a body starting inside PUSH_WIDE's operand")
	}
}
