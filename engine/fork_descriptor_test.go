package engine

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestCreateForkedTaskRejectsNilParentProgram(t *testing.T) {
	s := NewRuntime(dbstore.NewStore())
	defer s.Stop()
	parent := NewTaskFull(8800, 0, nil, 1000, 1)
	if got := s.CreateForkedTask(&parent.State, &types.ForkInfo{
		Body: &bytecode.ForkBody{},
	}); got != 0 {
		t.Fatalf("invalid parent program created task %d, want zero", got)
	}
}

type metadataOnlyForkBody struct{}

func (metadataOnlyForkBody) VariableNames() []string { return []string{"local"} }
func (metadataOnlyForkBody) FirstLine() int          { return 1 }

func TestCreateForkedTaskRejectsInvalidExecutionDescriptors(t *testing.T) {
	s := NewRuntime(dbstore.NewStore())
	defer s.Stop()
	parent := NewTaskFull(8801, 0, nil, 1000, 1)
	var typedNil *bytecode.ForkBody
	for _, test := range []struct {
		name string
		info *types.ForkInfo
	}{
		{"nil info", nil},
		{"source only", &types.ForkInfo{SourceLines: []string{"return 1;"}}},
		{"typed nil", &types.ForkInfo{Body: typedNil}},
		{"foreign metadata", &types.ForkInfo{Body: metadataOnlyForkBody{}}},
		{"invalid range", &types.ForkInfo{Body: &bytecode.ForkBody{Parent: &bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}}, Offset: 0, Length: 2}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := s.CreateForkedTask(&parent.State, test.info); got != 0 {
				t.Fatalf("invalid execution descriptor created task %d", got)
			}
		})
	}
}

func TestLoadQueuedForkRetainsSourceOnlyExecutionAndSnapshotMetadata(t *testing.T) {
	s := NewRuntime(dbstore.NewStore())
	defer s.Stop()
	saved := &dbformat.QueuedTask{
		ID: 8810, This: 0, Player: 0, Programmer: 0, VerbLoc: 0, Verb: "restored",
		Code: []string{"return local;"}, Variables: map[string]types.Value{"local": types.NewInt(7)},
	}
	if err := s.loadQueuedTask(saved); err != nil {
		t.Fatal(err)
	}
	restored := s.taskManager.GetTask(saved.ID)
	if restored == nil || restored.Program == nil || restored.ForkInfo == nil || restored.ForkInfo.Body != nil {
		t.Fatal("queued fork did not retain its source-only representation")
	}
	defer s.taskManager.RemoveTask(restored.ID)
	snapshot := restored.PersistenceSnapshot()
	if snapshot.Fork == nil || snapshot.Fork.FirstLine != 1 || !reflect.DeepEqual(snapshot.Fork.VariableNames, restored.Program.VarNames) || !reflect.DeepEqual(snapshot.Fork.SourceLines, saved.Code) {
		t.Fatalf("restored snapshot metadata=%+v", snapshot.Fork)
	}
	if err := s.runTask(restored); err != nil {
		t.Fatal(err)
	}
	if restored.Result.Flow != types.FlowReturn || restored.Result.Val.Type() != types.TYPE_INT || restored.Result.Val.Int() != 7 {
		t.Fatalf("restored fork result=%+v, want return 7", restored.Result)
	}
}

// Issue #908: a queued fork's saved code is its body alone; the header's first
// line is the verb line the body began on, and the restored task numbers its
// lines from it, as Toast's read_task_queue sets first_lineno.
func TestLoadQueuedForkNumbersLinesFromHeaderFirstLine(t *testing.T) {
	s := NewRuntime(dbstore.NewStore())
	defer s.Stop()
	saved := &dbformat.QueuedTask{
		ID: 8811, This: 0, Player: 0, Programmer: 0, VerbLoc: 0, Verb: "restored",
		Code: []string{"x = 2;", "return x + \"a\";"}, Variables: map[string]types.Value{}, FirstLine: 7,
	}
	if err := s.loadQueuedTask(saved); err != nil {
		t.Fatal(err)
	}
	restored := s.taskManager.GetTask(saved.ID)
	if restored == nil || restored.Program == nil {
		t.Fatal("queued fork was not restored")
	}
	defer s.taskManager.RemoveTask(restored.ID)
	if got := restored.Program.LineForIP(0); got != 7 {
		t.Fatalf("restored fork first line = %d, want 7", got)
	}
	if snapshot := restored.PersistenceSnapshot(); snapshot.Fork == nil || snapshot.Fork.FirstLine != 7 {
		t.Fatalf("restored snapshot fork = %+v, want first line 7", snapshot.Fork)
	}
	if err := s.runTask(restored); err != nil {
		t.Fatal(err)
	}
	stack := restored.Result.CallStack
	if restored.Result.Flow != types.FlowException || len(stack) == 0 || stack[len(stack)-1].LineNumber != 8 {
		t.Fatalf("restored fork result=%+v, want E_TYPE on line 8", restored.Result)
	}
}
