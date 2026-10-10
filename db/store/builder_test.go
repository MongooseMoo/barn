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

func TestBuildKeepsOnlyDefinedNamesInPropOrder(t *testing.T) {
	builder := NewObjectBuilder(2)
	builder.SetPropDefsCount(2)
	builder.ResetProperties(map[string]Property{
		"Mine":      NewProperty(types.NewInt(1), 1, PropRead, false, true),
		"also_mine": NewProperty(types.NewInt(2), 1, PropRead, false, true),
		"inherited": NewProperty(types.None, 1, PropRead, true, false),
		"override":  NewProperty(types.NewInt(3), 1, PropRead, false, false),
		"late":      NewProperty(types.NewInt(4), 1, PropRead, false, true),
	}, []string{"Mine", "also_mine", "inherited", "override", "late"})

	obj := builder.Build()

	want := []string{"Mine", "also_mine", "late"}
	if !stringsEqual(obj.propOrder, want) {
		t.Fatalf("propOrder = %v, want %v", obj.propOrder, want)
	}
	if got := obj.properties.count(); got != 5 {
		t.Fatalf("property slots = %d, want 5 (dropping a name must not drop its slot)", got)
	}

	s := NewStore()
	if err := s.Add(obj); err != nil {
		t.Fatal(err)
	}
	names, code := s.definedPropertyNames(2)
	if code != types.E_NONE || !stringsEqual(names, want) {
		t.Fatalf("definedPropertyNames = %v (%v), want %v", names, code, want)
	}
}

func TestBuildLeavesAFullyDefinedPropOrderInPlace(t *testing.T) {
	builder := NewObjectBuilder(3)
	builder.SetPropDefsCount(1)
	order := []string{"only"}
	builder.ResetProperties(map[string]Property{
		"only": NewProperty(types.NewInt(1), 1, PropRead, false, true),
	}, order)

	obj := builder.Build()

	if len(obj.propOrder) != 1 || &obj.propOrder[0] != &order[0] {
		t.Fatalf("propOrder was copied although nothing was dropped: %v", obj.propOrder)
	}
}
