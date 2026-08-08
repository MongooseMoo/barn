package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
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

func TestToobjStringOverflowClamps(t *testing.T) {
	ctx := kernel.NewTaskContext()
	tests := []struct {
		input string
		want  types.ObjID
	}{
		{input: "-9223372036854775807", want: -9223372036854775807},
		{input: "9223372036854775807", want: 9223372036854775807},
		{input: "-9223372036854775808", want: -9223372036854775808},
		{input: "9223372036854775808", want: 9223372036854775807},
		{input: "-9223372036854775809", want: -9223372036854775808},
		{input: "9223372036854775809", want: 9223372036854775807},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := builtinToobj(ctx, []types.Value{types.NewStr(tc.input)})
			if result.Flow != types.FlowNormal {
				t.Fatalf("toobj(%q) flow = %v error = %v, want normal", tc.input, result.Flow, result.Error)
			}
			if got := result.Val.Obj(); got != tc.want {
				t.Fatalf("toobj(%q) = %d, want %d", tc.input, got, tc.want)
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

func TestToliteralHidesNestedAnonymousMapKeyIdentity(t *testing.T) {
	ctx := kernel.NewTaskContext()
	first := types.NewAnon(1)
	second := types.NewAnon(2)
	mapping := types.NewMap([][2]types.Value{
		{first, types.NewInt(1)},
		{second, types.NewInt(2)},
	})

	result := builtinToliteral(ctx, []types.Value{mapping})
	if result.IsError() {
		t.Fatalf("toliteral failed: %v", result.Error)
	}
	if got := result.Val.Str(); got != "[*anonymous* -> 2, *anonymous* -> 1]" {
		t.Fatalf("toliteral(anonymous-key map) = %q, want nested identities hidden in tree order", got)
	}
}

func TestToliteralSortsMixedMapKeysCanonically(t *testing.T) {
	ctx := kernel.NewTaskContext()
	mapping := types.NewMap([][2]types.Value{
		{types.NewObj(-1), types.NewObj(-1)},
		{types.NewStr("2"), types.NewEmptyList()},
		{types.NewStr("1"), types.NewEmptyList()},
		{types.NewInt(5), types.NewInt(5)},
		{types.NewFloat(3.14), types.NewFloat(3.14)},
	})

	result := builtinToliteral(ctx, []types.Value{mapping})
	if result.IsError() {
		t.Fatalf("toliteral failed: %v", result.Error)
	}
	if got, want := result.Val.Str(), `[5 -> 5, #-1 -> #-1, 3.14 -> 3.14, "1" -> {}, "2" -> {}]`; got != want {
		t.Fatalf("toliteral(mixed-key map) = %q, want %q", got, want)
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

func TestWaifStringAndLiteralForms(t *testing.T) {
	ctx := kernel.NewTaskContext()
	waif := types.NewWaif(12, 34)

	t.Run("tostr", func(t *testing.T) {
		result := builtinTostr(ctx, []types.Value{waif})
		if result.IsError() {
			t.Fatalf("tostr failed: %v", result.Error)
		}
		if got := result.Val.Str(); got != "[[waif]]" {
			t.Fatalf("tostr(waif) = %q, want %q", got, "[[waif]]")
		}
	})

	t.Run("toliteral", func(t *testing.T) {
		result := builtinToliteral(ctx, []types.Value{waif})
		if result.IsError() {
			t.Fatalf("toliteral failed: %v", result.Error)
		}
		if got := result.Val.Str(); got != "[[class = #12, owner = #34]]" {
			t.Fatalf("toliteral(waif) = %q, want %q", got, "[[class = #12, owner = #34]]")
		}
	})
}

func TestStrictEqualIgnoresErrorMapInsertionOrder(t *testing.T) {
	forward := types.NewMap([][2]types.Value{
		{types.NewErr(types.E_TYPE), types.NewStr("type")},
		{types.NewErr(types.E_DIV), types.NewStr("div")},
	})
	reversed := types.NewMap([][2]types.Value{
		{types.NewErr(types.E_DIV), types.NewStr("div")},
		{types.NewErr(types.E_TYPE), types.NewStr("type")},
	})
	if !strictEqual(forward, reversed) {
		t.Fatal("reversed error-key maps are not strictly equal")
	}
}
