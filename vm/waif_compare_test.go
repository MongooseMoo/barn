package vm

import (
	"testing"

	"barn/types"
)

func TestWaifValuesCompareEqualForRelationalOrdering(t *testing.T) {
	first := types.NewWaif(10, 1)
	second := types.NewWaif(10, 1)
	if first.Equal(second) {
		t.Fatal("distinct waifs compare equal by identity")
	}
	comparison, err := compareValues(first, second, false)
	if err != nil {
		t.Fatalf("relational comparison failed: %v", err)
	}
	if comparison != 0 {
		t.Fatalf("relational comparison = %d, want 0", comparison)
	}
}
