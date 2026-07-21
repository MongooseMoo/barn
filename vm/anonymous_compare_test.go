package vm

import (
	"testing"

	"barn/types"
)

func TestAnonymousValuesCompareEqualForRelationalOrdering(t *testing.T) {
	first := types.NewAnon(10)
	second := types.NewAnon(11)
	if first.Equal(second) {
		t.Fatal("distinct anonymous values compare equal by identity")
	}
	comparison, err := compareValues(first, second, false)
	if err != nil {
		t.Fatalf("relational comparison failed: %v", err)
	}
	if comparison != 0 {
		t.Fatalf("relational comparison = %d, want 0", comparison)
	}
}
