package builtins

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

func descriptorStub(_ *Execution, _ []types.Value) types.Result {
	return types.Ok(types.NewInt(42))
}

func testDescriptor(name string) Descriptor {
	return Descriptor{Name: name, Implementation: descriptorStub, Signature: &Signature{MinArgs: 0, MaxArgs: 0, ArgTypes: []int64{}}, Visibility: Public, Effect: Transactional, Capability: config.Core}
}

func TestDescriptorConstructionRejectsInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Descriptor)
	}{
		{"name", func(d *Descriptor) { d.Name = "" }},
		{"implementation", func(d *Descriptor) { d.Implementation = nil }},
		{"signature", func(d *Descriptor) { d.Signature = nil }},
		{"arity", func(d *Descriptor) { d.Signature.MinArgs = 2 }},
		{"missing types", func(d *Descriptor) { d.Signature.MaxArgs = 1 }},
		{"type", func(d *Descriptor) { d.Signature.MaxArgs = 1; d.Signature.ArgTypes = []int64{999} }},
		{"visibility", func(d *Descriptor) { d.Visibility = 0 }},
		{"effect", func(d *Descriptor) { d.Effect = 0 }},
		{"capability", func(d *Descriptor) { d.Capability = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDescriptor("probe")
			tc.change(&d)
			if _, err := NewRegistryFromDescriptors(config.DefaultCapabilities(), []Descriptor{d}); err == nil {
				t.Fatal("invalid descriptor accepted")
			}
		})
	}
	d := testDescriptor("duplicate")
	if _, err := NewRegistryFromDescriptors(config.DefaultCapabilities(), []Descriptor{d, d}); err == nil {
		t.Fatal("duplicate accepted")
	}
	tooMany := make([]Descriptor, 1<<16+1)
	for i := range tooMany {
		tooMany[i] = testDescriptor(fmt.Sprintf("builtin_%d", i))
	}
	if _, err := NewRegistryFromDescriptors(config.DefaultCapabilities(), tooMany); err == nil {
		t.Fatal("bytecode ID overflow accepted")
	}
}

func TestDescriptorCapabilitiesAndImmutableLayout(t *testing.T) {
	a, b := testDescriptor("a"), testDescriptor("b")
	b.Capability = config.BarnExtensions
	for _, caps := range []config.Capabilities{config.Core, config.DefaultCapabilities()} {
		r, err := NewRegistryFromDescriptors(caps, []Descriptor{b, a})
		if err != nil {
			t.Fatal(err)
		}
		if r.Has("b") != (caps&config.BarnExtensions != 0) {
			t.Fatal("capability filter disagrees")
		}
		if id, ok := r.GetID("a"); !ok || id != 1 {
			t.Fatalf("a ID = %d, %v", id, ok)
		}
		_, diagnostics := r.Compiler().CompileMOO([]string{"return b();"})
		if (len(diagnostics) == 0) != r.Has("b") {
			t.Fatal("compiler capability filter disagrees")
		}
	}
	d := testDescriptor("snapshot")
	r, err := NewRegistryFromDescriptors(config.Core, []Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	d.Signature.MaxArgs = 1
	fn, _ := r.Get("snapshot")
	if got := fn(nil, []types.Value{types.NewInt(1)}); got.Error != types.E_ARGS {
		t.Fatal("caller mutated constructed registry")
	}
}

func TestBaseDescriptorsComplete(t *testing.T) {
	descriptors := BaseDescriptors()
	if len(descriptors) < 240 {
		t.Fatalf("only %d base descriptors", len(descriptors))
	}
	if _, err := NewRegistryFromDescriptors(config.DefaultCapabilities(), descriptors); err != nil {
		t.Fatal(err)
	}
}
