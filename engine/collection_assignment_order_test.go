package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestCollectionAssignmentEvaluatesTargetBeforeValue(t *testing.T) {
	tests := []struct {
		name string
		code string
		want []int64
	}{
		{
			name: "index",
			code: `i = 0; values = {0, 0}; values[(i = i + 1)] = (i = i + 1); return {i, values[1], values[2]};`,
			want: []int64{2, 2, 0},
		},
		{
			name: "range",
			code: `i = 0; values = {0, 0, 0}; values[(i = i + 1)..(i = i + 1)] = {(i = i + 1)}; return {i, values[1], values[2]};`,
			want: []int64{3, 3, 0},
		},
		{
			name: "property backed index",
			code: `objects = {#4, #5}; i = 0; objects[(i = i + 1)].p[1] = 99; return {i, #4.p[1], #5.p[1]};`,
			want: []int64{1, 99, 0},
		},
	}

	for testIndex, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := buildConcurrencyStore(t, 0, 0)
			for _, id := range []types.ObjID{4, 5} {
				object := dbstore.NewObjectBuilder(id)
				object.SetOwner(3)
				object.SetProperty("p", dbstore.NewProperty(types.NewList([]types.Value{types.NewInt(0)}), 3, dbstore.PropRead|dbstore.PropWrite, false, true))
				if err := store.Add(object.Build()); err != nil {
					t.Fatalf("add property target #%d: %v", id, err)
				}
			}
			runtime := newRuntimeWithWorkerCount(store, config.Options{}, 1)
			defer runtime.Stop()
			defer removeTasksForOwner(runtime, 3)

			task := runtime.buildBenchTask(t, int64(99800+testIndex), test.code)
			if err := runtime.runTask(task); err != nil {
				t.Fatalf("runTask: %v", err)
			}
			if task.Result.Flow != types.FlowReturn {
				t.Fatalf("result flow = %v, error = %v", task.Result.Flow, task.Result.Error)
			}
			got := task.Result.Val
			if got.Type() != types.TYPE_LIST || got.Len() != len(test.want) {
				t.Fatalf("result = %v, want %v", got, test.want)
			}
			for i, want := range test.want {
				if value := got.Get(i + 1); value.Type() != types.TYPE_INT || value.Int() != want {
					t.Fatalf("result[%d] = %v, want %d (full result %v)", i+1, value, want, got)
				}
			}
		})
	}
}
