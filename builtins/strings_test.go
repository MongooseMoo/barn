package builtins

import (
	"regexp"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestMooPatternPercentEscapesAreLiteralByDefault(t *testing.T) {
	pattern, _, err := translateMOOPattern("%.")
	if err != nil {
		t.Fatalf("translateMOOPattern failed: %v", err)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compiled pattern %q failed: %v", pattern, err)
	}
	if !re.MatchString(".") {
		t.Fatalf("%q did not match literal dot", pattern)
	}
	if re.MatchString("x") {
		t.Fatalf("%q matched non-dot character", pattern)
	}

	pattern, _, err = translateMOOPattern("%d")
	if err != nil {
		t.Fatalf("translateMOOPattern failed: %v", err)
	}
	re = regexp.MustCompile(pattern)
	if !re.MatchString("d") || re.MatchString("1") {
		t.Fatalf("%%d translated to %q, want literal d", pattern)
	}
}

func TestMooPatternPercentWordBoundaryEscapes(t *testing.T) {
	pattern, _, err := translateMOOPattern("%bbar")
	if err != nil {
		t.Fatalf("translateMOOPattern failed: %v", err)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compiled pattern %q failed: %v", pattern, err)
	}
	loc := re.FindStringIndex("foo bar")
	if loc == nil || loc[0] != 4 || loc[1] != 7 {
		t.Fatalf("%%bbar matched %v, want [4 7]", loc)
	}
}

func TestSubstituteRejectsMalformedMatchData(t *testing.T) {
	ctx := newTestExecution()
	template := types.NewStr("x")

	tests := []struct {
		name  string
		match types.Value
	}{
		{
			name:  "empty match result",
			match: types.NewList(nil),
		},
		{
			name: "out of range full match",
			match: types.NewList([]types.Value{
				types.NewInt(1),
				types.NewInt(9),
				types.NewList([]types.Value{
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
					types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)}),
				}),
				types.NewStr("abc"),
			}),
		},
		{
			name: "wrong group count",
			match: types.NewList([]types.Value{
				types.NewInt(1),
				types.NewInt(3),
				types.NewList(nil),
				types.NewStr("abc"),
			}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := admitTestBuiltin("substitute", ctx, []types.Value{template, tc.match})
			if res.Flow != types.FlowException || res.Error != types.E_INVARG {
				t.Fatalf("builtinSubstitute = flow %v error %v value %v, want E_INVARG", res.Flow, res.Error, res.Val)
			}
		})
	}
}

func TestSubstituteUnusedCaptureSubstitutesEmptyString(t *testing.T) {
	ctx := newTestExecution()
	groups := make([]types.Value, 9)
	for i := range groups {
		groups[i] = types.NewList([]types.Value{types.NewInt(0), types.NewInt(-1)})
	}
	match := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewInt(3),
		types.NewList(groups),
		types.NewStr("abc"),
	})

	res := admitTestBuiltin("substitute", ctx, []types.Value{types.NewStr("group=[%1]"), match})
	if res.Flow != types.FlowNormal {
		t.Fatalf("builtinSubstitute flow = %v error = %v, want normal", res.Flow, res.Error)
	}
	got := res.Val.Str()
	if got != "group=[]" {
		t.Fatalf("substitute result = %q, want group=[]", got)
	}
}

func TestRindexNegativeOffsetBeforeNeedleReturnsZero(t *testing.T) {
	result := admitTestBuiltin("rindex", newTestExecution(), []types.Value{
		types.NewStr("aaaa"),
		types.NewStr("aa"),
		types.NewInt(0),
		types.NewInt(-4),
	})
	if !result.IsNormal() {
		t.Fatalf("builtinRindex flow = %v error = %v, want normal", result.Flow, result.Error)
	}
	if got := result.Val.Int(); got != 0 {
		t.Fatalf("builtinRindex = %d, want 0", got)
	}
}

// Issue #1152: Toast's regex compiler rejects a pattern ending in a lone
// escape, so match() and rmatch() raise E_INVARG. "%%" is a quoted '%' and
// a '%' inside a class is literal; both still compile.
func TestMatchTrailingPercentRaisesInvarg(t *testing.T) {
	for _, pattern := range []string{"%", "a%", "x%", "%%%"} {
		for name, fn := range map[string]func(*Execution, []types.Value) types.Result{
			"match": builtinMatch, "rmatch": builtinRmatch,
		} {
			for _, subject := range []string{"", "x", "a%"} {
				r := fn(nil, []types.Value{types.NewStr(subject), types.NewStr(pattern)})
				if r.Flow != types.FlowException || r.Error != types.E_INVARG {
					t.Errorf("%s(%q, %q) = %+v, want E_INVARG", name, subject, pattern, r)
				}
			}
		}
	}
	for _, pattern := range []string{"%%", "[%]", "a%%"} {
		r := builtinMatch(nil, []types.Value{types.NewStr("a%"), types.NewStr(pattern)})
		if r.Flow == types.FlowException {
			t.Errorf("match(%q, %q) raised %v, want a match", "a%", pattern, r.Error)
		}
	}
}

// Toast reads the index/rindex offset into a C int, so only its low 32 bits
// count (issue #1145). Expected values are from the ToastStunt oracle.
func TestIndexOffsetKeepsLow32Bits(t *testing.T) {
	cases := []struct {
		name      string
		haystack  string
		needle    string
		offset    int64
		want      int64
		wantError types.ErrorCode
	}{
		{"rindex", "b", "%b\tb$\t", 9223372036854775807, 0, types.E_NONE},
		{"rindex", "abcabc", "abc", 4294967295, 1, types.E_NONE},
		{"rindex", "abcabc", "abc", 4294967296, 4, types.E_NONE},
		{"rindex", "abcabc", "abc", 2147483648, 0, types.E_NONE},
		{"rindex", "abcabc", "abc", -4294967296, 4, types.E_NONE},
		{"index", "b", "x", 9223372036854775807, 0, types.E_INVARG},
		{"index", "abcabc", "abc", 4294967299, 1, types.E_NONE},
		{"index", "abcabc", "abc", -4294967296, 1, types.E_NONE},
	}
	for _, tc := range cases {
		result := admitTestBuiltin(tc.name, newTestExecution(), []types.Value{
			types.NewStr(tc.haystack),
			types.NewStr(tc.needle),
			types.NewInt(0),
			types.NewInt(tc.offset),
		})
		if tc.wantError != types.E_NONE {
			if result.Flow != types.FlowException || result.Error != tc.wantError {
				t.Errorf("%s(%q, %q, 0, %d) flow = %v error = %v, want %v",
					tc.name, tc.haystack, tc.needle, tc.offset, result.Flow, result.Error, tc.wantError)
			}
			continue
		}
		if !result.IsNormal() {
			t.Errorf("%s(%q, %q, 0, %d) flow = %v error = %v, want normal",
				tc.name, tc.haystack, tc.needle, tc.offset, result.Flow, result.Error)
			continue
		}
		if got := result.Val.Int(); got != tc.want {
			t.Errorf("%s(%q, %q, 0, %d) = %d, want %d", tc.name, tc.haystack, tc.needle, tc.offset, got, tc.want)
		}
	}
}
