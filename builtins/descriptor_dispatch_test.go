package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

func TestDescriptorDispatchAdmissionParity(t *testing.T) {
	// Controlled implementations isolate argument admission from permissions,
	// object validity, external resources, and implementation-owned checks.
	descriptors := BaseDescriptors()
	for i := range descriptors {
		descriptors[i].Implementation = descriptorStub
		descriptors[i].Effect = Transactional
	}
	r, err := NewRegistryFromDescriptors(config.DefaultCapabilities(), descriptors)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(r, NoHost())
	ctx := newTestExecutionForSession(s)
	for _, d := range descriptors {
		t.Run(d.Name, func(t *testing.T) {
			id, _ := r.GetID(d.Name)
			args := make([]types.Value, d.Signature.MinArgs)
			for i := range args {
				args[i] = types.NewInt(0)
			}
			for i, typ := range d.Signature.ArgTypes {
				if i >= len(args) {
					break
				}
				switch typ {
				case 1:
					args[i] = types.NewObj(0)
				case 2:
					args[i] = types.NewStr("")
				case 3:
					args[i] = types.NewErr(types.E_NONE)
				case 4:
					args[i] = types.NewList(nil)
				case 9:
					args[i] = types.NewFloat(0)
				case 10:
					args[i] = types.NewMap(nil)
				}
			}
			check := func(args []types.Value, want types.ErrorCode) {
				t.Helper()
				byID := s.CallByIDWithExecution(id, ctx, args)
				byName, found := s.CallByNameWithExecution(d.Name, ctx, args)
				dynamic := builtinCallFunction(ctx, append([]types.Value{types.NewStr(d.Name)}, args...))
				if !found || byID.Error != want || byName.Error != want || dynamic.Error != want {
					t.Fatalf("admission mismatch: ID=%+v name=%+v dynamic=%+v; want %v", byID, byName, dynamic, want)
				}
				if want != types.E_NONE && (!byID.Val.Equal(byName.Val) || !byID.Val.Equal(dynamic.Val)) {
					t.Fatal("argument error payload differs by dispatch path")
				}
				if want == types.E_NONE && byID.Val.Int() != 42 {
					t.Fatal("controlled implementation not called")
				}
			}
			check(args, types.E_NONE)
			if len(args) > 0 {
				check(args[:len(args)-1], types.E_ARGS)
			}
			if d.Signature.MaxArgs >= 0 {
				check(make([]types.Value, d.Signature.MaxArgs+1), types.E_ARGS)
			}
			for i, typ := range d.Signature.ArgTypes {
				if i >= len(args) {
					break
				}
				if typ == -1 {
					continue
				}
				bad := append([]types.Value(nil), args...)
				bad[i] = types.NewMap(nil)
				if typ == 10 {
					bad[i] = types.NewStr("")
				}
				check(bad, types.E_TYPE)
			}
		})
	}
}

func TestConnectionOptionPreservesSemanticErrorOrder(t *testing.T) {
	s := NewSession(NewRegistry(), NoHost())
	ctx := newTestExecutionForSession(s)
	args := []types.Value{types.NewObj(123), types.NewInt(0)}
	want := builtinConnectionOption(ctx, args)
	if want.Error != types.E_INVARG {
		t.Fatalf("fixture result: %+v", want)
	}
	got, _ := s.CallByNameWithExecution("connection_option", ctx, args)
	if got.Error != want.Error {
		t.Fatalf("descriptor changed missing-connection error: got %v, want %v", got.Error, want.Error)
	}
}

func TestDescriptorExceptionalAndVariadicAdmission(t *testing.T) {
	d := testDescriptor("union")
	d.Signature = &Signature{MinArgs: 1, MaxArgs: -1, ArgTypes: []int64{1}, Alternatives: map[int][]int64{0: {0}}}
	str := int64(types.TYPE_STR)
	d.Signature.VariadicType = &str
	r, err := NewRegistryFromDescriptors(config.Core, []Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	fn, _ := r.Get(d.Name)
	for _, first := range []types.Value{types.NewObj(0), types.NewInt(0)} {
		if result := fn(nil, []types.Value{first, types.NewStr("tail")}); !result.IsNormal() {
			t.Fatal(result)
		}
		if result := fn(nil, []types.Value{first, types.NewInt(0)}); result.Error != types.E_TYPE {
			t.Fatal(result)
		}
	}
}

func TestDescriptorCapabilityCombinations(t *testing.T) {
	descriptors := BaseDescriptors()
	for caps := config.Capabilities(0); caps <= config.DefaultCapabilities(); caps++ {
		r, err := NewRegistryFromDescriptors(caps, descriptors)
		if err != nil {
			t.Fatal(err)
		}
		s := NewSession(r, NoHost())
		ctx := newTestExecutionForSession(s)
		presence := r.Presence()
		for _, d := range descriptors {
			want := caps&d.Capability == d.Capability
			if r.Has(d.Name) != want {
				t.Fatalf("%d: %s availability", caps, d.Name)
			}
			_, present := presence["builtin."+d.Name]
			if present != want {
				t.Fatalf("%d: %s presence", caps, d.Name)
			}
			info := builtinFunctionInfo(ctx, []types.Value{types.NewStr(d.Name)})
			if info.IsNormal() != (want && d.Visibility == Public) {
				t.Fatalf("%d: %s visibility", caps, d.Name)
			}
			if !want {
				if _, ok := r.GetID(d.Name); ok {
					t.Fatal("disabled ID")
				}
				if result := builtinCallFunction(ctx, []types.Value{types.NewStr(d.Name)}); result.Error != types.E_INVARG {
					t.Fatal("disabled dynamic call")
				}
			}
		}
	}
}
