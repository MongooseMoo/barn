package store

import (
	"barn/types"
	"testing"
	"unsafe"
)

func TestPropertyFitsCompactMapValue(t *testing.T) {
	if size := unsafe.Sizeof(Property{}); size > 40 {
		t.Fatalf("Property size = %d bytes, want at most 40", size)
	}
}

func TestResetPropertiesReusesResolvedValueMap(t *testing.T) {
	builder := NewObjectBuilder(1)
	properties := map[string]Property{
		"alpha": NewProperty(types.NewInt(1), 1, PropRead, false, true),
		"beta":  NewProperty(types.NewInt(2), 1, PropRead, false, true),
		"gamma": NewProperty(types.NewInt(3), 1, PropRead, false, true),
	}
	order := []string{"alpha", "beta", "gamma"}

	allocs := testing.AllocsPerRun(100, func() {
		builder.ResetProperties(properties, order)
	})
	if allocs != 0 {
		t.Fatalf("ResetProperties() allocations = %v, want 0", allocs)
	}
}
