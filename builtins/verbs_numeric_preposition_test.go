package builtins

import (
	"strconv"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func numericPrepositionArgs(prep string) []types.Value {
	return []types.Value{
		types.NewObj(0),
		types.NewStr("look"),
		types.NewList([]types.Value{types.NewStr("this"), types.NewStr(prep), types.NewStr("this")}),
	}
}

// The canonical managed Toast witness accepts 0..14 and raises E_INVARG at
// 15. Mongoose #59:_fix_preps relies on exactly this rejection to end its loop.
func TestSetVerbArgsNumericPrepositionBounds(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)
	firstError := -1
	for code := 0; code <= 32; code++ {
		result := admitTestBuiltin("set_verb_args", ctx, numericPrepositionArgs(strconv.Itoa(code)))
		if result.IsError() {
			if result.Error != types.E_INVARG {
				t.Fatalf("numeric prep %d: error %v, want E_INVARG", code, result.Error)
			}
			if firstError < 0 {
				firstError = code
			}
		} else {
			if firstError >= 0 {
				t.Fatalf("numeric prep %d succeeded after first rejection %d", code, firstError)
			}
			if result.Val.Type() != types.TYPE_INT || result.Val.Int() != 0 {
				t.Fatalf("numeric prep %d returned %v, want integer zero", code, result)
			}
		}
	}
	if firstError != 15 {
		t.Fatalf("first rejected numeric prep = %d, want 15", firstError)
	}
}

func TestSetVerbArgsNumericPrepositionCanonicalReadback(t *testing.T) {
	for _, tc := range []struct {
		code      string
		canonical string
	}{
		{"0", "with/using"},
		{"1", "at/to"},
		{"2", "in front of"},
		{"3", "in/inside/into"},
		{"4", "on top of/on/onto/upon"},
		{"5", "out of/from inside/from"},
		{"6", "over"},
		{"7", "through"},
		{"8", "under/underneath/beneath"},
		{"9", "behind"},
		{"10", "beside"},
		{"11", "for/about"},
		{"12", "is"},
		{"13", "as"},
		{"14", "off/off of"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			ctx, _ := verbMetadataTxnTestContext(t)
			result := admitTestBuiltin("set_verb_args", ctx, numericPrepositionArgs(tc.code))
			if result.IsError() {
				t.Fatalf("numeric prep %q: %v", tc.code, result)
			}
			got := admitTestBuiltin("verb_args", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
			want := types.NewList([]types.Value{types.NewStr("this"), types.NewStr(tc.canonical), types.NewStr("this")})
			if got.IsError() || !got.Val.Equal(want) {
				t.Fatalf("numeric prep %q readback = %v, want %v", tc.code, got, want)
			}
		})
	}
}

func TestSetVerbArgsNumericPrepositionRefusalPreservesState(t *testing.T) {
	for number := 15; number <= 32; number++ {
		code := strconv.Itoa(number)
		t.Run(code, func(t *testing.T) {
			ctx, store := verbMetadataTxnTestContext(t)
			original, ec := store.DirectTxn().VerbByIndex(0, 0)
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			result := admitTestBuiltin("set_verb_args", ctx, numericPrepositionArgs(code))
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("numeric prep %q returned %v, want E_INVARG", code, result)
			}
			current, ec := store.DirectTxn().VerbByIndex(0, 0)
			if ec != types.E_NONE || current.ArgSpec != original.ArgSpec {
				t.Fatalf("rejected prep changed live args: %v / %v, error %v", original.ArgSpec, current.ArgSpec, ec)
			}
			if ctx.LiveStoreMutated {
				t.Fatal("rejected prep marked live store mutated")
			}
			got := admitTestBuiltin("verb_args", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
			want := types.NewList([]types.Value{types.NewStr("none"), types.NewStr("none"), types.NewStr("none")})
			if got.IsError() || !got.Val.Equal(want) {
				t.Fatalf("rejected prep changed transaction readback: %v, want %v", got, want)
			}
		})
	}
}

func TestSetVerbArgsNumericPrepositionLexicalCases(t *testing.T) {
	// These exact inputs and outputs are from the canonical managed lexical
	// supplement; signs, whitespace and trailing characters are not normalized.
	for _, tc := range []struct {
		input     string
		canonical string // empty means E_INVARG
	}{
		{"-1", ""},
		{"-2", ""},
		{"-3", ""},
		{"+1", ""},
		{"01", "at/to"},
		{"00015", ""},
		{" 1", ""},
		{"1 ", ""},
		{"15 ", ""},
		{"+15", ""},
		{"999999999999999999999999999999", ""},
		{"1x", ""},
		{"0x1", ""},
		{"with", "with/using"},
		{"any", "any"},
		{"none", "none"},
		{"", ""},
		{"99", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, _ := verbMetadataTxnTestContext(t)
			result := admitTestBuiltin("set_verb_args", ctx, numericPrepositionArgs(tc.input))
			wantPrep := tc.canonical
			if tc.canonical == "" {
				if !result.IsError() || result.Error != types.E_INVARG {
					t.Fatalf("prep %q returned %v, want E_INVARG", tc.input, result)
				}
				if ctx.LiveStoreMutated {
					t.Fatal("rejected lexical prep marked live store mutated")
				}
				wantPrep = "none"
			} else if result.IsError() || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 0 {
				t.Fatalf("prep %q returned %v, want integer zero", tc.input, result)
			}
			got := admitTestBuiltin("verb_args", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
			wantDobj, wantIobj := "this", "this"
			if tc.canonical == "" {
				wantDobj, wantIobj = "none", "none"
			}
			want := types.NewList([]types.Value{types.NewStr(wantDobj), types.NewStr(wantPrep), types.NewStr(wantIobj)})
			if got.IsError() || !got.Val.Equal(want) {
				t.Fatalf("prep %q readback = %v, want %v", tc.input, got, want)
			}
		})
	}
}

// Toast bf_set_verb_args checks the object (E_INVARG when invalid) and then the
// args list before E_VERBNF and E_PERM (src/verbs.cc:431-449).
func TestSetVerbArgsNumericPrepositionAdmissionOrder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		object     types.ObjID
		verb       string
		programmer types.ObjID
		want       types.ErrorCode
	}{
		{"nonowner", 0, "look", 123, types.E_INVARG},
		{"missing_verb", 0, "absent", 0, types.E_INVARG},
		{"missing_object", 999, "look", 0, types.E_INVARG},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := verbMetadataTxnTestContext(t)
			ctx.IsWizard = false
			ctx.Programmer = tc.programmer
			args := numericPrepositionArgs("15")
			args[0], args[1] = types.NewObj(tc.object), types.NewStr(tc.verb)
			result := admitTestBuiltin("set_verb_args", ctx, args)
			if !result.IsError() || result.Error != tc.want {
				t.Fatalf("admission returned %v, want %v for invalid prep", result, tc.want)
			}
			if ctx.LiveStoreMutated {
				t.Fatal("failed admission marked live store mutated")
			}
		})
	}
}
