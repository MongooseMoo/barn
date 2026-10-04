package task

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

func TestPersistenceSnapshotCopiesLiveForkDescriptorMetadata(t *testing.T) {
	parent := &bytecode.Program{
		Code:     []byte{byte(bytecode.OP_RETURN_NONE), byte(bytecode.OP_RETURN_NONE)},
		VarNames: []string{"first", "second"},
		LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 2}, {StartIP: 1, Line: 17}},
	}
	tk := NewTaskFull(8100, 0, nil, 1000, 1)
	tk.ForkInfo = &types.ForkInfo{Body: &bytecode.ForkBody{Parent: parent, Offset: 1, Length: 1}}
	saved := tk.PersistenceSnapshot()
	if saved.Fork == nil || saved.Fork.FirstLine != 17 || !reflect.DeepEqual(saved.Fork.VariableNames, []string{"first", "second"}) {
		t.Fatalf("live fork metadata=%+v", saved.Fork)
	}
	parent.VarNames[0] = "parent edit"
	if saved.Fork.VariableNames[0] != "first" {
		t.Fatal("snapshot variable names alias descriptor metadata")
	}
	saved.Fork.VariableNames[1] = "snapshot edit"
	if parent.VarNames[1] != "second" {
		t.Fatal("descriptor metadata aliases snapshot variable names")
	}
}

func TestPersistenceSnapshotUsesSourceRestoredForkProgramMetadata(t *testing.T) {
	program := &bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}, VarNames: []string{"saved"}, LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 9}}}
	tk := NewTaskFull(8101, 0, program, 1000, 1)
	tk.ForkInfo = &types.ForkInfo{SourceLines: []string{"return saved;"}, Variables: map[string]types.Value{"saved": types.NewInt(7)}}
	saved := tk.PersistenceSnapshot()
	if saved.Fork == nil || saved.Fork.FirstLine != 9 || !reflect.DeepEqual(saved.Fork.VariableNames, []string{"saved"}) {
		t.Fatalf("source-restored fork metadata=%+v", saved.Fork)
	}
	if tk.ForkInfo.Body != nil {
		t.Fatal("source-restored fork unexpectedly acquired a live descriptor")
	}
}

func TestPersistenceSnapshotHandlesTypedNilForkDescriptor(t *testing.T) {
	tk := NewTaskFull(8102, 0, nil, 1000, 1)
	var body *bytecode.ForkBody
	tk.ForkInfo = &types.ForkInfo{Body: body}
	saved := tk.PersistenceSnapshot()
	if saved.Fork == nil || saved.Fork.FirstLine != 0 || len(saved.Fork.VariableNames) != 0 {
		t.Fatalf("typed-nil descriptor metadata=%+v", saved.Fork)
	}
}
