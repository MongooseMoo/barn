package vm

//   go test ./vm -run=^$ -bench=BenchmarkWaifProperty -benchmem

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

var waifPropertyBenchWorkloads = []struct {
	name string
	code string
}{
	// The WAIF holds its own foo; bar is read from the class.
	{"own_read_100k", "w = #0.benchwaif; x = 0; for i in [1..100000]; x = w.foo; endfor; return x;"},
	{"class_read_100k", "w = #0.benchwaif; x = 0; for i in [1..100000]; x = w.bar; endfor; return x;"},
	{"write_100k", "w = #0.benchwaif; for i in [1..100000]; w.foo = i; endfor; return w.foo;"},
}

// BenchmarkWaifProperty runs each workload in a task snapshot transaction, as
// a task does, over a class #0 defining :foo and :bar.
func BenchmarkWaifProperty(b *testing.B) {
	for _, w := range waifPropertyBenchWorkloads {
		b.Run(w.name, func(b *testing.B) {
			prog, registry := compileBench(b, w.code)
			store := newBenchStore(b)
			txn := store.DirectTxn()
			for _, name := range []string{":foo", ":bar"} {
				if errCode := txn.DefineProperty(0, name, dbstore.NewProperty(types.NewInt(7), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
					b.Fatalf("DefineProperty(%s): %v", name, errCode)
				}
			}
			waif := types.NewWaif(0, 0).SetProperty("foo", types.NewInt(5))
			store.RegisterWaif(0, waif)
			if errCode := txn.DefineProperty(0, "benchwaif", dbstore.NewProperty(waif, 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
				b.Fatalf("DefineProperty(benchwaif): %v", errCode)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := task.NewContext()
				ctx.Store = store
				ctx.TicksRemaining = 1 << 60
				ctx.IsWizard = true
				ctx.StoreTxn = store.BeginSnapshot(0)
				m := NewVM(store, newTestSession(registry))
				m.Context = ctx
				m.Task = task.NewState(1, types.ObjID(0), ctx.TicksRemaining, 1)
				m.TickLimit = 1 << 60
				res := m.Run(prog)
				ctx.StoreTxn.Release()
				if res.Flow == types.FlowException {
					b.Fatalf("%s raised: %v", w.name, res.Error)
				}
			}
		})
	}
}
