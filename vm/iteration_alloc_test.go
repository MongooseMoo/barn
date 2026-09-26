package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Iteration setup must not allocate one pair/list header per input element.
// Compare sizes instead of pinning VM construction's unrelated fixed costs.
func TestIndexedIterationAllocationGrowth(t *testing.T) {
	for _, kind := range []string{"list", "map"} {
		t.Run(kind, func(t *testing.T) {
			registry := BuildVMRegistry()
			program, diagnostics := registry.Compiler().CompileMOO([]string{
				"total = 0; for value, key in (args[1]) total = total + value; endfor return total;",
			})
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			store := dbstore.NewStore()
			session := newTestSession(registry)
			allocs := func(n int) float64 {
				values := make([]types.Value, n)
				pairs := make([][2]types.Value, n)
				for i := range values {
					values[i] = types.NewInt(int64(i + 1))
					pairs[i] = [2]types.Value{types.NewInt(int64(i)), values[i]}
				}
				input := types.NewList(values)
				if kind == "map" {
					input = types.NewMap(pairs)
				}
				args := []types.Value{input}
				return testing.AllocsPerRun(5, func() {
					ctx := kernel.NewTaskContext()
					ctx.Store = store
					ctx.TicksRemaining = 1 << 60
					machine := NewVM(store, session)
					machine.Context = ctx
					machine.TickLimit = 1 << 60
					machine.Task = task.NewTask(1, 0, ctx.TicksRemaining, 1)
					result := machine.RunWithVerbContext(program, 0, 0, 0, "iteration", 0, args)
					if result.Flow != types.FlowReturn || !result.Val.Equal(types.NewInt(int64(n*(n+1)/2))) {
						t.Fatalf("result = %+v", result)
					}
				})
			}
			small, large := allocs(10), allocs(1000)
			t.Logf("allocations for 10/1000 elements: %.0f / %.0f", small, large)
			if large-small > 10 {
				t.Fatalf("iteration allocations grow with input: %.0f -> %.0f", small, large)
			}
		})
	}
}
