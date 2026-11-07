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
		{"truncated unwind jump", Program{Code: []byte{0xff}}, "truncated operands for UNWIND_JUMP"},
		{"truncated operand", Program{Code: []byte{byte(OP_PUSH)}}, "truncated"},
		{"constant index", Program{Code: []byte{byte(OP_PUSH), 0, byte(OP_RETURN)}}, "constant index"},
		{"local index", Program{Code: []byte{byte(OP_GET_VAR), 1, byte(OP_RETURN)}, VarNames: []string{"x"}, NumLocals: 1}, "local index"},
		{"column key slot", Program{Code: []byte{byte(OP_FOR_LIST_LOAD_COLUMNS), 0, 1, 2, 3, 5, byte(OP_RETURN_NONE)}, NumLocals: 5}, "local index"},
		{"column truncated keys", Program{Code: []byte{byte(OP_FOR_LIST_LOAD_COLUMNS), 0, 1, 2, 3}, NumLocals: 5}, "truncated"},
		{"value load slot", Program{Code: []byte{byte(OP_FOR_LIST_LOAD_VALUE), 0, 1, 3, byte(OP_RETURN_NONE)}, NumLocals: 3}, "local index"},
		{"jump into operand", Program{Code: []byte{byte(OP_LOOP), 0, 2, byte(OP_RETURN_NONE)}}, "instruction boundary"},
		{"finally handler at its own try", Program{Code: []byte{byte(OP_TRY_FINALLY), 0, 0, byte(OP_IMM_BASE), byte(OP_RETURN)}}, "does not follow"},
		{"finally handler before its try", Program{Code: []byte{byte(OP_RETURN_NONE), byte(OP_TRY_FINALLY_WIDE), 0, 0, 0, 0, byte(OP_RETURN_NONE)}}, "does not follow"},
		{"except handler at its own try", Program{Code: []byte{byte(OP_TRY_EXCEPT), 1, 0, 0, 0, 0, byte(OP_RETURN_NONE)}}, "does not follow"},
		{"falls off end", Program{Code: []byte{byte(OP_POP)}}, "terminal"},
		{"dead opcode", Program{Code: []byte{byte(OP_BREAK), byte(OP_RETURN_NONE)}}, "dead opcode"},
		{"fewer locals than names", Program{Code: []byte{byte(OP_RETURN_NONE)}, VarNames: []string{"x"}, NumLocals: 0}, "invalid local metadata"},
		{"more locals than addressable", Program{Code: []byte{byte(OP_RETURN_NONE)}, VarNames: []string{"x"}, NumLocals: MaxLocals + 1}, "invalid local metadata"},
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

// The compiler allocates unnamed temporaries from slot 255 downward, so any
// program using one carries NumLocals == MaxLocals alongside a handful of
// VarNames. Such programs must verify, or suspended tasks cannot be restored.
func TestVerifyProgramAcceptsCompilerTemporarySlots(t *testing.T) {
	program := &Program{
		Code: []byte{
			byte(OP_GET_VAR), 2,
			byte(OP_SET_VAR), 255,
			byte(OP_RETURN_NONE),
		},
		VarNames:  []string{"a", "b", "c"},
		NumLocals: MaxLocals,
	}
	if err := VerifyProgram(program); err != nil {
		t.Fatalf("VerifyProgram() error = %v, want temporaries above VarNames accepted", err)
	}
}

func immediate(value int) byte {
	op, _ := MakeImmediateOpcode(value)
	return byte(op)
}

func TestVerifyProgramRejectsStackUnsafeBytecode(t *testing.T) {
	tests := []struct {
		name string
		code []byte
		want string
	}{
		{"return from empty stack", []byte{byte(OP_RETURN)}, "stack underflow"},
		{"pop from empty stack", []byte{byte(OP_POP), byte(OP_RETURN_NONE)}, "stack underflow"},
		{"binary operator short one operand", []byte{immediate(1), byte(OP_ADD), byte(OP_RETURN)}, "stack underflow"},
		{"short-circuit on empty stack", []byte{byte(OP_AND), 0, 0, byte(OP_RETURN_NONE)}, "stack underflow"},
		{"call arguments missing", []byte{byte(OP_CALL_BUILTIN), 0, 2, byte(OP_RETURN)}, "stack underflow"},
		{"join at different heights", []byte{
			immediate(1),
			byte(OP_JUMP_IF_FALSE), 0, 1, // to the RETURN_NONE at height 0
			immediate(5), // fallthrough reaches it at height 1
			byte(OP_RETURN_NONE),
		}, "stack height"},
		{"loop grows the stack", []byte{
			immediate(1),
			byte(OP_LOOP), 0, 4, // back to the push at height 1
			byte(OP_RETURN_NONE),
		}, "stack height"},
		{"fork parent falls off the end", []byte{immediate(0), byte(OP_FORK), 0, 0, 1, byte(OP_RETURN_NONE)}, "end of the program"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyProgram(&Program{Code: tt.code})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("VerifyProgram() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestVerifyProgramRejectsTickFreeCycles(t *testing.T) {
	tests := []struct {
		name string
		code []byte
	}{
		{"wide loop to itself", []byte{byte(OP_LOOP_WIDE), 0, 0, 0, 5, byte(OP_RETURN_NONE)}},
		{"range next to itself", []byte{byte(OP_FOR_RANGE_NEXT), 0, 0, 0, 5, byte(OP_RETURN_NONE)}},
		{"loop through free pushes", []byte{
			immediate(1), byte(OP_POP),
			byte(OP_LOOP), 0, 5, // back to the push
			byte(OP_RETURN_NONE),
		}},
		{"zero tick prefix on the loop test", []byte{
			immediate(0),
			byte(OP_TICKS), 0,
			byte(OP_JUMP_IF_FALSE), 0, 3, // to the RETURN_NONE
			byte(OP_LOOP), 0, 9, // back to the push
			byte(OP_RETURN_NONE),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyProgram(&Program{Code: tt.code, NumLocals: 1})
			if err == nil || !strings.Contains(err.Error(), "charges no tick") {
				t.Fatalf("VerifyProgram() error = %v, want a tick-free cycle", err)
			}
		})
	}
}

func TestVerifyProgramAcceptsTickingLoops(t *testing.T) {
	for name, code := range map[string][]byte{
		"checked test": {
			immediate(0),
			byte(OP_JUMP_IF_FALSE), 0, 3,
			byte(OP_LOOP), 0, 7,
			byte(OP_RETURN_NONE),
		},
		"unchecked prefix": {
			immediate(0),
			byte(OP_TICKS), 1 | TicksUnchecked,
			byte(OP_JUMP_IF_FALSE), 0, 3,
			byte(OP_LOOP), 0, 9,
			byte(OP_RETURN_NONE),
		},
	} {
		if err := VerifyProgram(&Program{Code: code}); err != nil {
			t.Errorf("%s: VerifyProgram() error = %v", name, err)
		}
	}
}

func TestStackHeightsFollowsEveryEdge(t *testing.T) {
	program := &Program{Code: []byte{
		immediate(0),                // 0: h0
		byte(OP_TRY_FINALLY), 0, 10, // 1: h1, finally entry at 10 with h1
		immediate(2),      // 4: h1
		byte(OP_OR), 0, 1, // 5: h2, jump keeps the value, fallthrough pops it
		immediate(3),         // 8: h1
		byte(OP_ADD),         // 9: h2 on both paths
		byte(OP_RETURN),      // 10: h1
		byte(OP_RETURN_NONE), // 11: unreachable
	}}
	heights, err := StackHeights(program)
	if err != nil {
		t.Fatalf("StackHeights() error = %v", err)
	}
	want := map[int]int{0: 0, 1: 1, 4: 1, 5: 2, 8: 1, 9: 2, 10: 1, 11: -1}
	for ip, height := range want {
		if heights[ip] != height {
			t.Fatalf("height at %d = %d, want %d (all %v)", ip, heights[ip], height, heights)
		}
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
