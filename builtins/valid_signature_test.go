package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestValidSignatureFollowsValidIncarnation: stock Toast registers valid(ANY)
// (function_info {"valid", 1, 1, {-1}}; a second argument is E_ARGS with the
// expected/got message). The Mongoose fork registers valid(ANY [, STR]); Barn
// follows it only under VALID_INCARNATION. PROMOTE_NUMBERS does not affect it.
func TestValidSignatureFollowsValidIncarnation(t *testing.T) {
	stock := newTestExecution()
	stock.RuntimeOptions.PromoteNumbers = true
	fork := newTestExecution()
	fork.RuntimeOptions.ValidIncarnation = true
	obj := types.NewObj(types.ObjID(0))
	name := types.NewStr("valid")

	for _, tc := range []struct {
		label string
		ctx   *Execution
		want  string
	}{
		{"stock", stock, `{"valid", 1, 1, {-1}}`},
		{"fork", fork, `{"valid", 1, 2, {-1, 2}}`},
	} {
		info := admitTestBuiltin("function_info", tc.ctx, []types.Value{name})
		if !info.IsNormal() || info.Val.String() != tc.want {
			t.Fatalf("%s function_info(\"valid\") = %v (%v), want %s", tc.label, info.Val, info.Error, tc.want)
		}
		all := admitTestBuiltin("function_info", tc.ctx, nil)
		found := false
		for _, entry := range all.Val.Elements() {
			if entry.String() == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s function_info() lacks %s", tc.label, tc.want)
		}
	}

	two := []types.Value{obj, types.NewInt(2)}
	for label, call := range map[string]func(*Execution, []types.Value) types.Result{
		"by name": func(ctx *Execution, args []types.Value) types.Result {
			r, _ := ctx.Session.CallByNameWithExecution("valid", ctx, args)
			return r
		},
		"stored": func(ctx *Execution, args []types.Value) types.Result {
			return admitTestBuiltin("valid", ctx, args)
		},
	} {
		got := call(stock, two)
		if got.Error != types.E_ARGS {
			t.Fatalf("%s stock valid(#0, 2) error = %v, want E_ARGS", label, got.Error)
		}
		if msg := got.Val.Elements()[1].Str(); msg != "Incorrect number of arguments (expected 1; got 2)" {
			t.Fatalf("%s stock valid(#0, 2) message = %q", label, msg)
		}
		if got := call(fork, two); got.Error != types.E_TYPE {
			t.Fatalf("%s fork valid(#0, 2) error = %v, want E_TYPE", label, got.Error)
		}
		if got := call(fork, []types.Value{obj, types.NewStr("x"), types.NewStr("y")}); got.Error != types.E_ARGS {
			t.Fatalf("%s fork valid with three arguments error = %v, want E_ARGS", label, got.Error)
		}
	}
}
