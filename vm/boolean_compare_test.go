package vm

import (
	"testing"

	"barn/types"
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
