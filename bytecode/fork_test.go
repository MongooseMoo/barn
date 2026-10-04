package bytecode

import (
	"math"
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestForkBodyExtractsProgramAndReportsMetadata(t *testing.T) {
	parent := &Program{
		Code:      []byte{byte(OP_PUSH), 0, byte(OP_RETURN_NONE)},
		Constants: []types.Value{types.NewInt(1)},
		VarNames:  []string{"local"},
		NumLocals: 1,
		LineInfo:  []LineEntry{{StartIP: 0, Line: 5}, {StartIP: 2, Line: 17}},
	}
	body := &ForkBody{Parent: parent, Offset: 2, Length: 1}
	child := body.ExtractProgram()
	if child == nil || !reflect.DeepEqual(child.Code, []byte{byte(OP_RETURN_NONE), byte(OP_RETURN_NONE)}) {
		t.Fatalf("extracted program=%+v, want body and implicit return", child)
	}
	if !reflect.DeepEqual(body.VariableNames(), []string{"local"}) || body.FirstLine() != 17 || child.NumLocals != 1 {
		t.Fatalf("descriptor metadata: names=%v first line=%d locals=%d", body.VariableNames(), body.FirstLine(), child.NumLocals)
	}
	if !reflect.DeepEqual(child.LineInfo, []LineEntry{{StartIP: 0, Line: 17}}) {
		t.Fatalf("fork line mapping=%+v", child.LineInfo)
	}
	child.Code[0] = byte(OP_POP)
	if parent.Code[2] != byte(OP_RETURN_NONE) {
		t.Fatal("child code aliases parent storage")
	}
}

func TestForkBodyPreservesWideOffsetAndLength(t *testing.T) {
	const offset, length = 70000, 80000
	parent := &Program{Code: instructionPadding(offset + length), VarNames: []string{"wide"}, LineInfo: []LineEntry{{StartIP: offset, Line: 29}}}
	body := &ForkBody{Parent: parent, Offset: offset, Length: length}
	child := body.ExtractProgram()
	if child == nil || len(child.Code) != length+1 || body.FirstLine() != 29 || body.VariableNames()[0] != "wide" {
		t.Fatal("wide descriptor was truncated or lost metadata")
	}
}

func TestForkBodyAllowsEmptyInstructionRange(t *testing.T) {
	parent := &Program{Code: []byte{byte(OP_RETURN_NONE)}, VarNames: []string{"local"}, LineInfo: []LineEntry{{StartIP: 0, Line: 3}}}
	body := &ForkBody{Parent: parent, Offset: 0, Length: 0}
	child := body.ExtractProgram()
	if child == nil || !reflect.DeepEqual(child.Code, []byte{byte(OP_RETURN_NONE)}) || body.FirstLine() != 3 {
		t.Fatal("empty instruction range no longer creates an implicit-return body")
	}
}

func TestForkBodyRejectsInvalidParentsAndRanges(t *testing.T) {
	parent := &Program{Code: []byte{byte(OP_PUSH), 0, byte(OP_RETURN_NONE)}, Constants: []types.Value{types.NewInt(1)}, VarNames: []string{"local"}, LineInfo: []LineEntry{{StartIP: 0, Line: 9}}}
	for _, test := range []struct {
		name string
		body *ForkBody
	}{
		{"nil descriptor", nil},
		{"nil parent", &ForkBody{}},
		{"negative offset", &ForkBody{Parent: parent, Offset: -1, Length: 1}},
		{"negative length", &ForkBody{Parent: parent, Offset: 0, Length: -1}},
		{"offset past end", &ForkBody{Parent: parent, Offset: 4, Length: 0}},
		{"length past end", &ForkBody{Parent: parent, Offset: 2, Length: 2}},
		{"overflowing end", &ForkBody{Parent: parent, Offset: 2, Length: math.MaxInt}},
		{"start in operand", &ForkBody{Parent: parent, Offset: 1, Length: 1}},
		{"end in operand", &ForkBody{Parent: parent, Offset: 0, Length: 1}},
		{"unknown instruction", &ForkBody{Parent: &Program{Code: []byte{255}}, Offset: 0, Length: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if program := test.body.ExtractProgram(); program != nil {
				t.Fatalf("invalid descriptor extracted program %+v", program)
			}
			if names, line := test.body.VariableNames(), test.body.FirstLine(); names != nil || line != 0 {
				t.Fatalf("invalid descriptor exposed names=%v line=%d", names, line)
			}
		})
	}
}
