package fuzzseed

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/vm"
)

func TestStructuredBuiltinNamesAreRegistered(t *testing.T) {
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, vm.Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range builtinNames {
		if !registry.Has(name) {
			t.Errorf("builtin %q is not registered", name)
		}
	}
}

func TestStructuredSourceIsDeterministic(t *testing.T) {
	choices := []byte{2, 3, 4, 200, 17, 9, 5, 6, 7, 8, 99, 1, 0, 12}
	first := StructuredSource(t, choices)
	if second := StructuredSource(t, choices); second != first {
		t.Fatalf("same choices gave different programs:\n%s\n---\n%s", first, second)
	}
	if empty := StructuredSource(t, nil); empty == first {
		t.Fatalf("different choices gave the same program:\n%s", first)
	}
}
