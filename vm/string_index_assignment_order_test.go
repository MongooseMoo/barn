package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Issue #1147: assigning into a string index checks the value's type before
// the index range, then the value's length. Expectations are the pinned Toast
// oracle's results for the same statements.
func TestStringIndexAssignmentErrorOrder(t *testing.T) {
	tests := []struct {
		code string
		want types.ErrorCode
	}{
		{`s = "%$a"; s[9223372036854775807] = #0;`, types.E_TYPE},
		{`s = "%$a"; s[5] = #0;`, types.E_TYPE},
		{`s = "%$a"; s[0] = #0;`, types.E_TYPE},
		{`s = "%$a"; s[1] = #0;`, types.E_TYPE},
		{`s = "abc"; s[5] = 1;`, types.E_TYPE},
		{`s = {"abc"}; s[1][5] = #0;`, types.E_TYPE},
		{`s = {"abc"}; s[1][2] = #0;`, types.E_TYPE},
		{`s = "%$a"; s[5] = "x";`, types.E_RANGE},
		{`s = "%$a"; s[0] = "x";`, types.E_RANGE},
		{`s = "abc"; s[5] = "xy";`, types.E_RANGE},
		{`s = "abc"; s[2] = "xy";`, types.E_INVARG},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			code := "try " + test.code + " return 0; except e (ANY) return e[1]; endtry"
			result := runBytecodeProgram(t, code, nil, nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_ERR || result.Val.ErrCode() != test.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %v", result.Flow, result.Val, result.Error, types.NewErr(test.want))
			}
		})
	}
}
