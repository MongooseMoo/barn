package vm

// Micro-benchmarks for the VM hot loop, mirroring the bench/bench.py workloads
// used to compare barn against ToastStunt. Drives the VM directly (no socket) so
// profiles isolate interpreter cost from network/protocol.
//
//   go test ./vm -run=^$ -bench=BenchmarkVM -benchmem
//   go test ./vm -run=^$ -bench=BenchmarkVM/int_arith -cpuprofile=/tmp/cpu.prof -memprofile=/tmp/mem.prof
//   go tool pprof -top -nodecount=25 /tmp/cpu.prof

import (
	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/task"
	"barn/types"
	"testing"
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
	{"tostr_200k", "n = 0; for i in [1..200000]; n = n + length(tostr(i)); endfor; return n;"},
	{"nested_1k", "c = 0; for i in [1..1000]; for j in [1..1000]; c = c + 1; endfor; endfor; return c;"},
}

func compileBench(b *testing.B, code string) (*Program, *builtins.Registry) {
	b.Helper()
	registry := BuildVMRegistry()
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		b.Fatalf("parse failed: %v", err)
	}
	compiler := NewCompilerWithRegistry(registry)
	prog, err := compiler.CompileStatements(stmts)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}
	return prog, registry
}

func BenchmarkVM(b *testing.B) {
	for _, w := range vmBenchWorkloads {
		w := w
		b.Run(w.name, func(b *testing.B) {
			prog, registry := compileBench(b, w.code)
			store := dbstore.NewStore()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := kernel.NewTaskContext()
				ctx.Store = store
				ctx.Registry = registry
				ctx.TicksRemaining = 1 << 60
				ctx.Task = task.NewTask(1, types.ObjID(0), ctx.TicksRemaining, 1)
				m := NewVM(store, registry)
				m.Context = ctx
				m.TickLimit = 1 << 60
				res := m.Run(prog)
				if res.Flow == types.FlowException {
					b.Fatalf("%s raised: %v", w.name, res.Error)
				}
			}
		})
	}
}
