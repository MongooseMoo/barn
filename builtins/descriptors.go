package builtins

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/config"
)

type Visibility uint8

const (
	Public Visibility = iota + 1
	Hidden
)

// EffectPolicy describes who owns the irreversible-effect boundary.
type EffectPolicy uint8

const (
	Transactional EffectPolicy = iota + 1
	Irreversible
	CommitGateReentrant
	ImplementationOwned
)

// Signature describes public argument admission. ArgTypes checks the positional
// prefix; an omitted variadic tail admits any type, as MOO registration does.
// Alternatives express exceptional positional unions without a name switch.
type Signature struct {
	MinArgs, MaxArgs int64
	ArgTypes         []int64
	Alternatives     map[int][]int64
	VariadicType     *int64
	PlainArityError  bool
}

// Descriptor is the complete construction input for one builtin. Construction
// copies metadata; callers cannot mutate the resulting registry through it.
type Descriptor struct {
	Name           string
	Implementation BuiltinFunc
	Signature      *Signature
	Visibility     Visibility
	Effect         EffectPolicy
	LineSync       bool
	Capability     config.Capabilities
}

func (d Descriptor) validate() error {
	bad := func(why string) error { return fmt.Errorf("builtin %q: %s", d.Name, why) }
	if d.Name == "" || strings.ToLower(d.Name) != d.Name || strings.ContainsAny(d.Name, " \t\r\n") {
		return bad("invalid name")
	}
	for i, c := range d.Name {
		if c != '_' && (c < 'a' || c > 'z') && (i == 0 || c < '0' || c > '9') {
			return bad("invalid identifier")
		}
	}
	if d.Implementation == nil {
		return bad("missing implementation")
	}
	if d.Signature == nil {
		return bad("missing signature")
	}
	if d.Visibility != Public && d.Visibility != Hidden {
		return bad("invalid visibility")
	}
	if d.Effect < Transactional || d.Effect > ImplementationOwned {
		return bad("invalid effect policy")
	}
	if d.Capability == 0 || d.Capability & ^config.DefaultCapabilities() != 0 {
		return bad("invalid capability")
	}
	s := d.Signature
	if s.MinArgs < 0 || s.MaxArgs < -1 || (s.MaxArgs >= 0 && s.MaxArgs < s.MinArgs) {
		return bad("invalid arity")
	}
	if s.MaxArgs >= 0 && int64(len(s.ArgTypes)) != s.MaxArgs {
		return bad("incomplete positional signature")
	}
	if s.MaxArgs == -1 && int64(len(s.ArgTypes)) > s.MinArgs {
		return bad("variadic prefix exceeds required arity")
	}
	for _, typ := range s.ArgTypes {
		if !validArgType(typ) {
			return bad("invalid argument type")
		}
	}
	for index, alternatives := range s.Alternatives {
		if index < 0 || index >= len(s.ArgTypes) || len(alternatives) == 0 {
			return bad("invalid positional alternatives")
		}
		for _, typ := range alternatives {
			if !validArgType(typ) {
				return bad("invalid alternative type")
			}
		}
	}
	if s.VariadicType != nil && (s.MaxArgs != -1 || !validArgType(*s.VariadicType)) {
		return bad("invalid variadic type")
	}
	return nil
}

func validArgType(t int64) bool {
	switch t {
	case -2, -1, 0, 1, 2, 3, 4, 9, 10, 12, 13, 14:
		return true
	}
	return false
}

func cloneSignature(s *Signature) Signature {
	copy := *s
	copy.ArgTypes = slices.Clone(s.ArgTypes)
	copy.Alternatives = make(map[int][]int64, len(s.Alternatives))
	for i, alternatives := range s.Alternatives {
		copy.Alternatives[i] = slices.Clone(alternatives)
	}
	if s.VariadicType != nil {
		tail := *s.VariadicType
		copy.VariadicType = &tail
	}
	return copy
}

// NewRegistryFromDescriptors validates all descriptors, including disabled
// families. Descriptor order defines deterministic IDs; disabled entries leave
// holes so capability filtering cannot reinterpret previously compiled IDs.
func NewRegistryFromDescriptors(capabilities config.Capabilities, descriptors []Descriptor) (*Registry, error) {
	if capabilities & ^config.DefaultCapabilities() != 0 {
		return nil, fmt.Errorf("unknown builtin capabilities")
	}
	seen := make(map[string]bool, len(descriptors))
	for _, d := range descriptors {
		if err := d.validate(); err != nil {
			return nil, err
		}
		if seen[d.Name] {
			return nil, fmt.Errorf("duplicate builtin %q", d.Name)
		}
		seen[d.Name] = true
	}
	if len(descriptors) > 1<<16 {
		return nil, fmt.Errorf("builtin ID layout has %d entries; limit is 65536", len(descriptors))
	}
	r := &Registry{funcs: make(map[string]BuiltinFunc), nameToID: make(map[string]int)}
	for _, d := range descriptors {
		if capabilities&d.Capability == d.Capability {
			r.install(d)
		} else {
			r.entries = append(r.entries, nil)
		}
	}
	r.sourceCompiler = compiler.New(r.nameToID)
	return r, nil
}

// Presence reports availability from the actual constructed registry, including
// hidden callable extensions. It does not describe runtime permissions.
func (r *Registry) Presence() map[string]any {
	features := make(map[string]any, len(r.entries))
	for _, entry := range r.entries {
		if entry != nil {
			features["builtin."+entry.name] = "present"
		}
	}
	return features
}
