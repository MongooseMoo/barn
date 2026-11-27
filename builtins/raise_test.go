package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast's bf_raise defaults the message to value2str(code), which is tostr()
// formatting. Expected strings were observed on the WSL Toast oracle
// (ToastStunt 2.8.0_7, emergency mode) for raise(code) caught by try/except.
func TestRaiseDefaultMessageUsesTostrFormatting(t *testing.T) {
	ctx := newTestExecution()
	for _, tc := range []struct {
		name string
		code types.Value
		want string
	}{
		{"map", types.NewMap([][2]types.Value{{types.NewFloat(1.5), types.NewStr(`a" %""`)}}), "[map]"},
		{"list", types.NewList([]types.Value{types.NewInt(1), types.NewStr("x")}), "{list}"},
		{"string", types.NewStr(`\`), `\`},
		{"object", types.NewObj(5), "#5"},
		{"float", types.NewFloat(1.5), "1.5"},
		{"bool", types.NewBool(true), "true"},
		{"error", types.NewErr(types.E_PERM), "Permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := builtinRaise(ctx, []types.Value{tc.code})
			if result.Flow != types.FlowException {
				t.Fatalf("raise(%s) flow = %v, want exception", tc.code, result.Flow)
			}
			if got := result.Val.Get(2).Str(); got != tc.want {
				t.Fatalf("raise(%s) message = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}
