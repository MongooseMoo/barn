package vm

// Micro-benchmarks for the VM hot loop, mirroring the bench/bench.py workloads
// used to compare barn against ToastStunt. Drives the VM directly (no socket) so
// profiles isolate interpreter cost from network/protocol.
//
//   go test ./vm -run=^$ -bench=BenchmarkVM -benchmem
//   go test ./vm -run=^$ -bench=BenchmarkVM/int_arith -cpuprofile=/tmp/cpu.prof -memprofile=/tmp/mem.prof
//   go tool pprof -top -nodecount=25 /tmp/cpu.prof

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

var vmBenchWorkloads = []struct {
	name string
	code string
}{
	{"int_arith_1M", "x = 0; for i in [1..1000000]; x = x + i; endfor; return x;"},
	{"float_arith_1M", "x = 0.0; for i in [1..1000000]; x = x + 1.5; endfor; return x;"},
	{"string_concat_10k", `s = ""; for i in [1..10000]; s = s + "x"; endfor; return length(s);`},
	{"list_append_10k", "l = {}; for i in [1..10000]; l = {@l, i}; endfor; return length(l);"},
	{"list_index_1M", "l = {}; for i in [1..1000]; l = {@l, i}; endfor; x = 0; for i in [1..1000000]; x = l[1 + (i % 1000)]; endfor; return x;"},
	{"builtin_abs_200k", "x = 0; for i in [1..200000]; x = x + abs(-i); endfor; return x;"},
	{"tostr_200k", "n = 0; for i in [1..200000]; n = n + length(tostr(i)); endfor; return n;"},
	{"nested_1k", "c = 0; for i in [1..1000]; for j in [1..1000]; c = c + 1; endfor; endfor; return c;"},
	{"list_iter_1M", "l = {1..1000000}; s = 0; for x in (l); s = s + x; endfor; return s;"},
	{"while_lt_1M", "i = 0; while (i < 1000000) i = i + 1; endwhile; return i;"},
	{"if_chain_1M", "c = 0; for i in [1..1000000]; if (i % 2 == 0) c = c + 1; elseif (i > 500000) c = c - 1; else c = c * 2; endif; endfor; return c;"},
	// prop_access_1M mirrors bench_differ's workload; the next three split it
	// into its parts (builtin call alone, built-in property alone, defined
	// property alone) so a regression can be attributed.
	{"prop_access_1M", "n = #0; x = 0; for i in [1..1000000]; x = typeof(n.name); endfor; return x;"},
	{"typeof_1M", "x = 0; for i in [1..1000000]; x = typeof(i); endfor; return x;"},
	{"prop_name_1M", "n = #0; x = 0; for i in [1..1000000]; x = n.name; endfor; return x;"},
	{"prop_defined_1M", "n = #0; x = 0; for i in [1..1000000]; x = n.benchprop; endfor; return x;"},
}

// newBenchStore returns a store holding #0 with one defined property so the
// property workloads have something to read.
func newBenchStore(b *testing.B) *dbstore.Store {
	b.Helper()
	store := dbstore.NewStore()
	txn := store.DirectTxn()
	obj, errCode := txn.CreateObject(nil, 0)
	if errCode != types.E_NONE || obj != 0 {
		b.Fatalf("CreateObject = #%d, %v; want #0", obj, errCode)
	}
	if errCode := txn.SetObjectName(obj, "bench"); errCode != types.E_NONE {
		b.Fatalf("SetObjectName: %v", errCode)
	}
	if errCode := txn.DefineProperty(obj, "benchprop", dbstore.NewProperty(types.NewInt(42), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		b.Fatalf("DefineProperty: %v", errCode)
	}
	return store
}

func compileBench(b *testing.B, code string) (*bytecode.Program, *builtins.Registry) {
	b.Helper()
	registry := BuildVMRegistry()
	prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) > 0 {
		b.Fatalf("compile failed: %v", diagnostics)
	}
	return prog, registry
}

func BenchmarkVM(b *testing.B) {
	for _, w := range vmBenchWorkloads {
		w := w
		b.Run(w.name, func(b *testing.B) {
			prog, registry := compileBench(b, w.code)
			store := newBenchStore(b)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := kernel.NewTaskContext()
				ctx.Store = store
				ctx.TicksRemaining = 1 << 60
				m := NewVM(store, newTestSession(registry))
				m.Context = ctx
				m.Task = task.NewTask(1, types.ObjID(0), ctx.TicksRemaining, 1)
				m.TickLimit = 1 << 60
				res := m.Run(prog)
				if res.Flow == types.FlowException {
					b.Fatalf("%s raised: %v", w.name, res.Error)
				}
			}
		})
	}
}
