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
	got := res.Val.(types.FloatValue).Val
	if got != 3.5 {
		t.Fatalf("tofloat finite = %v, want 3.5", got)
	}
}
