package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestBooleanValuesCompareEqualForRelationalOrdering(t *testing.T) {
	tests := []struct {
		name  string
		left  types.Value
		right types.Value
	}{
		{name: "false before true", left: types.NewBool(false), right: types.NewBool(true)},
		{name: "true before false", left: types.NewBool(true), right: types.NewBool(false)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			comparison, err := compareValues(test.left, test.right, false)
			if err != nil {
				t.Fatalf("relational comparison failed: %v", err)
			}
			if comparison != 0 {
				t.Fatalf("relational comparison = %d, want 0", comparison)
			}
		})
	}
}

func TestBooleanIntegerEqualityAndMembershipUseMOOEquivalence(t *testing.T) {
	result := runBytecodeProgram(t, `return {
		0 == false,
		equal(0, false),
		false in {0, false},
		is_member(false, {0, false}),
		is_member(false, {0, false}, 0),
		1 == true,
		equal(1, true),
		true in {1, true},
		is_member(true, {1, true}),
		is_member(true, {1, true}, 0)
	};`, nil, strictCtx())
	if result.Flow != types.FlowReturn {
		t.Fatalf("flow = %v error = %v, want return", result.Flow, result.Error)
	}
	want := types.NewList([]types.Value{
		types.NewInt(1), types.NewInt(1), types.NewInt(1), types.NewInt(1), types.NewInt(1),
		types.NewInt(1), types.NewInt(1), types.NewInt(1), types.NewInt(1), types.NewInt(1),
	})
	if !result.Val.Equal(want) {
		t.Fatalf("result = %s, want %s", result.Val.String(), want.String())
	}
}
