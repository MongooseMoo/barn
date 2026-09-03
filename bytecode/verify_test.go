package bytecode

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestVerifyProgramRejectsMalformedBytecode(t *testing.T) {
	tests := []struct {
		name    string
		program Program
		want    string
	}{
		{"empty", Program{}, "empty"},
		{"unknown opcode", Program{Code: []byte{0xff}}, "unknown opcode"},
		{"truncated operand", Program{Code: []byte{byte(OP_PUSH)}}, "truncated"},
		{"constant index", Program{Code: []byte{byte(OP_PUSH), 0, byte(OP_RETURN)}}, "constant index"},
		{"local index", Program{Code: []byte{byte(OP_GET_VAR), 1, byte(OP_RETURN)}, VarNames: []string{"x"}, NumLocals: 1}, "local index"},
		{"jump into operand", Program{Code: []byte{byte(OP_LOOP), 0, 2, byte(OP_RETURN_NONE)}}, "instruction boundary"},
		{"falls off end", Program{Code: []byte{byte(OP_POP)}}, "terminal"},
		{"dead opcode", Program{Code: []byte{byte(OP_BREAK), byte(OP_RETURN_NONE)}}, "dead opcode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyProgram(&tt.program)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("VerifyProgram() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestVerifyProgramAcceptsValidControlFlow(t *testing.T) {
	program := &Program{Code: []byte{
		byte(OP_JUMP), 0, 0,
		byte(OP_RETURN_NONE),
	}}
	if err := VerifyProgram(program); err != nil {
		t.Fatalf("VerifyProgram() error = %v", err)
	}
	if !program.IsInstructionBoundary(0) || !program.IsInstructionBoundary(3) || program.IsInstructionBoundary(1) {
		t.Fatal("IsInstructionBoundary reported incorrect boundaries")
	}
}

func FuzzVerifyProgram(f *testing.F) {
	f.Add([]byte{byte(OP_RETURN_NONE)}, 0, 0)
	f.Add([]byte{byte(OP_PUSH)}, 0, 0)
	f.Fuzz(func(t *testing.T, code []byte, constants, locals int) {
		if constants < 0 || constants > 256 || locals < 0 || locals > 256 {
			t.Skip()
		}
		program := &Program{
			Code:      append([]byte(nil), code...),
			Constants: make([]types.Value, constants),
			VarNames:  make([]string, locals),
			NumLocals: locals,
		}
		_ = VerifyProgram(program)
	})
}
