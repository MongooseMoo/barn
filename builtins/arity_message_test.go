package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestArityErrorMessageNamesBounds: functions.cc call_bi_func raises E_ARGS
// with "(expected MIN-MAX; got N)" when the registered bounds differ, a
// variadic maximum printing as -1, and "(expected N; got M)" when they match.
func TestArityErrorMessageNamesBounds(t *testing.T) {
	ctx := newTestExecution()
	for _, tc := range []struct {
		name string
		args []types.Value
		want string
	}{
		{"notify", []types.Value{types.NewStr("SAe:")}, "Incorrect number of arguments (expected 2-4; got 1)"},
		{"notify", make([]types.Value, 5), "Incorrect number of arguments (expected 2-4; got 5)"},
		{"call_function", nil, "Incorrect number of arguments (expected 1--1; got 0)"},
		{"typeof", nil, "Incorrect number of arguments (expected 1; got 0)"},
	} {
		got := admitTestBuiltin(tc.name, ctx, tc.args)
		if got.Flow != types.FlowException || got.Error != types.E_ARGS {
			t.Fatalf("%s with %d args = %v (%v), want E_ARGS", tc.name, len(tc.args), got.Val, got.Error)
		}
		want := types.NewList([]types.Value{types.NewErr(types.E_ARGS), types.NewStr(tc.want), types.NewInt(0)})
		if !got.Val.Equal(want) {
			t.Fatalf("%s with %d args raised %v, want %v", tc.name, len(tc.args), got.Val, want)
		}
	}
}
