package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A root VM reading a hot property its snapshot holds a superseded version of
// moves its task to a transaction at the current clock and reads there. A
// nested VM, whose caller still holds the transaction, reads the snapshot.
func TestRootVMReadsAStaleHotPropertyAtTheCurrentClock(t *testing.T) {
	for _, root := range []bool{true, false} {
		name := map[bool]string{true: "root", false: "nested"}[root]
		t.Run(name, func(t *testing.T) {
			store := dbstore.NewStore()
			system := dbstore.NewObjectBuilder(0)
			system.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
			system.SetProperty("handle", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			if err := store.Add(system.Build()); err != nil {
				t.Fatalf("store.Add: %v", err)
			}
			store.MarkHotProperty(0, "handle")

			registry := BuildVMRegistry()
			prog, diagnostics := registry.Compiler().CompileMOO([]string{"return #0.handle;"})
			if len(diagnostics) > 0 {
				t.Fatalf("CompileMOO: %v", diagnostics[0])
			}
			ctx := kernel.NewTaskContext()
			ctx.IsWizard = true
			ctx.Store = store
			began := store.BeginSnapshot(0)
			ctx.StoreTxn = began
			defer func() { ctx.StoreTxn.Release() }()

			other := store.BeginSnapshot(0)
			if ec := other.SetPropertyValue(0, "handle", types.NewInt(7)); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue: %v", ec)
			}
			if ec := other.Commit(); ec != types.E_NONE {
				t.Fatalf("Commit: %v", ec)
			}
			other.Release()

			machine := NewVM(store, newTestSession(registry))
			machine.Context = ctx
			machine.Task = task.NewTask(1, 0, 30000, 1)
			machine.Resumable = root
			machine.PrepareVerbFrame(prog, types.ObjNothing, 0, 0, "", types.ObjNothing, []types.Value{})
			result := machine.ExecuteLoop()
			if result.Flow != types.FlowReturn {
				t.Fatalf("flow %v: %v", result.Flow, result.Error)
			}

			want, renewed := int64(0), false
			if root {
				want, renewed = 7, true
			}
			if got := result.Val.Int(); got != want {
				t.Fatalf("#0.handle = %d, want %d", got, want)
			}
			if (ctx.StoreTxn != began) != renewed {
				t.Fatalf("transaction replaced = %v, want %v", ctx.StoreTxn != began, renewed)
			}
		})
	}
}
