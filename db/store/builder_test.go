package store

import (
	"barn/types"
	"testing"
)

func TestResetPropertiesReusesResolvedValueMap(t *testing.T) {
	builder := NewObjectBuilder(1)
	properties := map[string]Property{
		"alpha": NewProperty("alpha", types.NewInt(1), 1, PropRead, false, true),
		"beta":  NewProperty("beta", types.NewInt(2), 1, PropRead, false, true),
		"gamma": NewProperty("gamma", types.NewInt(3), 1, PropRead, false, true),
	}
	order := []string{"alpha", "beta", "gamma"}

	allocs := testing.AllocsPerRun(100, func() {
		builder.ResetProperties(properties, order)
	})
	if allocs != 0 {
		t.Fatalf("ResetProperties() allocations = %v, want 0", allocs)
	}
}
