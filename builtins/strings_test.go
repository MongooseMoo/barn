package builtins

import (
	"regexp"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestMooPatternPercentEscapesAreLiteralByDefault(t *testing.T) {
	pattern, err := mooPatternToGoRegex("%.")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
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

	pattern, err = mooPatternToGoRegex("%d")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re = regexp.MustCompile(pattern)
	if !re.MatchString("d") || re.MatchString("1") {
		t.Fatalf("%%d translated to %q, want literal d", pattern)
	}
}

func TestMooPatternPercentWordBoundaryEscapes(t *testing.T) {
	pattern, err := mooPatternToGoRegex("%bbar")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compiled pattern %q failed: %v", pattern, err)
	}
	loc := re.FindStringIndex("foo bar")
	if loc == nil || loc[0] != 4 || loc[1] != 7 {
		t.Fatalf("%%bbar matched %v, want [4 7]", loc)
	}

	pattern, err = mooPatternToGoRegex("foo%>")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re = regexp.MustCompile(pattern)
	loc = re.FindStringIndex("foo bar")
	if loc == nil || loc[0] != 0 || loc[1] != 3 {
		t.Fatalf("foo%%> matched %v, want [0 3]", loc)
	}
	if re.MatchString("foobar") {
		t.Fatalf("foo%%> matched inside word")
	}
}

func TestSubstituteRejectsMalformedMatchData(t *testing.T) {
	ctx := kernel.NewTaskContext()
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
			res := builtinSubstitute(ctx, []types.Value{template, tc.match})
			if res.Flow != types.FlowException || res.Error != types.E_INVARG {
				t.Fatalf("builtinSubstitute = flow %v error %v value %v, want E_INVARG", res.Flow, res.Error, res.Val)
			}
		})
	}
}

func TestSubstituteUnusedCaptureSubstitutesEmptyString(t *testing.T) {
	ctx := kernel.NewTaskContext()
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

	res := builtinSubstitute(ctx, []types.Value{types.NewStr("group=[%1]"), match})
	if res.Flow != types.FlowNormal {
		t.Fatalf("builtinSubstitute flow = %v error = %v, want normal", res.Flow, res.Error)
	}
	got := res.Val.Str()
	if got != "group=[]" {
		t.Fatalf("substitute result = %q, want group=[]", got)
	}
}
