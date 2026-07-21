package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestTofloatRejectsNonFiniteStringValues(t *testing.T) {
	ctx := kernel.NewTaskContext()

	for _, input := range []string{"inf", "-inf", "nan", "Infinity", "1e999"} {
		t.Run(input, func(t *testing.T) {
			res := builtinTofloat(ctx, []types.Value{types.NewStr(input)})
			if res.Flow != types.FlowException || res.Error != types.E_INVARG {
				t.Fatalf("tofloat(%q) = flow %v error %v value %v, want E_INVARG", input, res.Flow, res.Error, res.Val)
			}
		})
	}

	res := builtinTofloat(ctx, []types.Value{types.NewStr("3.5")})
	if res.Flow != types.FlowNormal {
		t.Fatalf("tofloat finite flow = %v error = %v, want normal", res.Flow, res.Error)
	}
	got := res.Val.Float()
	if got != 3.5 {
		t.Fatalf("tofloat finite = %v, want 3.5", got)
	}
}

func TestTointStringOverflowClamps(t *testing.T) {
	ctx := kernel.NewTaskContext()

	tests := []struct {
		input string
		want  int64
	}{
		{"99999999999999999999", 9223372036854775807},
		{"9223372036854775808", 9223372036854775807},
		{"-99999999999999999999", -9223372036854775808},
		{"9223372036854775807", 9223372036854775807},
		{"-9223372036854775808", -9223372036854775808},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			res := builtinToint(ctx, []types.Value{types.NewStr(tc.input)})
			if res.Flow != types.FlowNormal {
				t.Fatalf("toint(%q) flow = %v error = %v, want normal", tc.input, res.Flow, res.Error)
			}
			got := res.Val.Int()
			if got != tc.want {
				t.Fatalf("toint(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestToliteralHidesAnonymousObjectIdentity(t *testing.T) {
	ctx := kernel.NewTaskContext()
	result := builtinToliteral(ctx, []types.Value{types.NewAnon(12)})
	if result.IsError() {
		t.Fatalf("toliteral failed: %v", result.Error)
	}
	if got := result.Val.Str(); got != "*anonymous*" {
		t.Fatalf("toliteral(anonymous) = %q, want %q", got, "*anonymous*")
	}
}

func TestAnonymousObjectNumericConversionsReturnTypeError(t *testing.T) {
	ctx := kernel.NewTaskContext()
	for name, convert := range map[string]func(*kernel.TaskContext, []types.Value) types.Result{
		"toint":   builtinToint,
		"toobj":   builtinToobj,
		"tofloat": builtinTofloat,
	} {
		t.Run(name, func(t *testing.T) {
			result := convert(ctx, []types.Value{types.NewAnon(12)})
			if !result.IsError() || result.Error != types.E_TYPE {
				t.Fatalf("%s(anonymous) = %+v, want E_TYPE", name, result)
			}
		})
	}
}

func TestTostrHidesAnonymousObjectIdentity(t *testing.T) {
	ctx := kernel.NewTaskContext()
	result := builtinTostr(ctx, []types.Value{types.NewAnon(12)})
	if result.IsError() {
		t.Fatalf("tostr failed: %v", result.Error)
	}
	if got := result.Val.Str(); got != "*anonymous*" {
		t.Fatalf("tostr(anonymous) = %q, want %q", got, "*anonymous*")
	}
}
