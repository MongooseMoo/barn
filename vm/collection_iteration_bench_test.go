package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Fixtures and compilation are outside the timer. Each iteration creates a VM,
// executes a numeric filter, and checks the result. count measures loop/predicate
// work without constructing an output collection; it is not equivalent work.
var collectionIterationSink types.Value

func BenchmarkCollectionIteration(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		values := make([]types.Value, n)
		pairs := make([][2]types.Value, n)
		for i := range values {
			values[i] = types.NewInt(int64(i + 1))
			pairs[i] = [2]types.Value{types.NewInt(int64(i + 1)), values[i]}
		}
		list := types.NewList(values)
		mapping := types.NewMap(pairs)
		for _, mode := range []string{"list", "indexed", "map", "count"} {
			for _, reject := range []bool{false, true} {
				label := fmt.Sprintf("%s/n%d/reject%t", mode, n, reject)
				b.Run(label, func(b *testing.B) {
					input := list
					loop := "for x in (args[1])"
					if mode == "indexed" || mode == "map" {
						loop = "for x, k in (args[1])"
					}
					if mode == "map" {
						input = mapping
					}
					threshold := n / 2
					expected := n - n/2
					if reject {
						threshold = n
						expected = 0
					}
					init, body, ret := "l = {};", "l = {@l, x};", "length(l)"
					if mode == "count" {
						init = "l = 0;"
						body = "l = l + 1;"
						ret = "l"
					}
					code := fmt.Sprintf("%s %s if (x > %d) %s endif endfor return %s;", init, loop, threshold, body, ret)
					prog, registry := compileBench(b, code)
					store := newBenchStore(b)
					session := newTestSession(registry)
					args := []types.Value{input}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						ctx := kernel.NewTaskContext()
						ctx.Store = store
						ctx.TicksRemaining = 1 << 60
						m := NewVM(store, session)
						m.Context = ctx
						m.Task = task.NewTask(1, 0, ctx.TicksRemaining, 1)
						m.TickLimit = 1 << 60
						res := m.RunWithVerbContext(prog, 0, 0, 0, "filter_survey", 0, args)
						if res.Flow == types.FlowException || res.Val.Type() != types.TYPE_INT || res.Val.Int() != int64(expected) {
							b.Fatalf("unexpected result: %+v expected %d", res, expected)
						}
						collectionIterationSink = res.Val
					}
				})
			}
		}
	}
}
