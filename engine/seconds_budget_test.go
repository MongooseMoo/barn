package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestSecondsBudgetStopsLoopAndCommitsPrefix(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagWizard)
	root.SetProperty("progress", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	s := NewRuntime(store)
	defer s.Stop()
	program := compileTestProgram(t, s.registry, `
#0.progress = 1;
start = ftime();
while (ftime() - start < 0.2)
endwhile
#0.progress = 2;
return 0;
`)
	running := task.NewTaskFull(99, 0, program, 1000000000, 0.01)
	running.Context.IsWizard = true
	if err := s.runTask(running); err != nil {
		t.Fatal(err)
	}
	if running.Result.Error != types.E_MAXREC || !resultValueContains(running.Result.Val, "seconds limit exceeded") {
		t.Fatalf("result = %+v, want seconds exhaustion", running.Result)
	}
	value, errCode := store.DirectTxn().PropertyValue(0, "progress")
	if errCode != types.E_NONE || value.Int() != 1 {
		t.Fatalf("progress = %v, %v; want committed prefix 1", value, errCode)
	}
}
