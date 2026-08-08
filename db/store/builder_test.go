package store

import (
	"github.com/MongooseMoo/barn/types"
	"testing"
	"unsafe"
)

func TestPropertyFitsCompactMapValue(t *testing.T) {
	// 40 bytes was the compacted size before MVCC; the version uint64 that makes
	// a property snapshot-visible adds 8, so the compact target is now 48.
	if size := unsafe.Sizeof(Property{}); size > 48 {
		t.Fatalf("Property size = %d bytes, want at most 48", size)
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
