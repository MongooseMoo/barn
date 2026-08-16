package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestDeepNestedRangeAssignment(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "variable depth two",
			source: `values = {{{1, 2, 3}}}; values[1][1][1..2] = {8, 9}; return values;`,
			want:   `{{{8, 9, 3}}}`,
		},
		{
			name:   "variable depth three",
			source: `values = {{{{1, 2, 3}}}}; values[1][1][1][2..3] = {8, 9}; return values;`,
			want:   `{{{{1, 8, 9}}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runBytecodeProgram(t, test.source, nil, nil)
			if result.Flow != types.FlowReturn || result.Error != types.E_NONE || result.Val.String() != test.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %s", result.Flow, result.Val, result.Error, test.want)
			}
		})
	}
}

func TestDeepNestedRangeAssignmentWithPropertyRoot(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add(#0) = %v", err)
	}
	initial := types.NewList([]types.Value{
		types.NewList([]types.Value{
			types.NewList([]types.Value{types.NewInt(1), types.NewInt(2), types.NewInt(3)}),
		}),
	})
	if errCode := store.DirectTxn().DefineProperty(0, "deep_range", dbstore.NewProperty(initial, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty() = %v", errCode)
	}

	result := runBytecodeProgram(t, `#0.deep_range[1][1][1..2] = {8, 9}; return #0.deep_range;`, store, nil)
	if result.Flow != types.FlowReturn || result.Error != types.E_NONE || result.Val.String() != `{{{8, 9, 3}}}` {
		t.Fatalf("result = flow %v, value %v, error %v; want {{{8, 9, 3}}}", result.Flow, result.Val, result.Error)
	}
}
