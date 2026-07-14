package vm

import (
	"testing"

	"barn/types"
)

// TestBuiltinRaisedErrorValueIsErrorList reproduces the de-box regression where
// a builtin that raises via types.Err(...) surfaced its error as bare int 0
// instead of the standard {code, message, value, traceback} list.
//
// length(5) raises E_TYPE inside builtin dispatch. Caught via try/except, the
// bound value must be the 4-element error list whose first element is the
// E_TYPE error code — NOT integer 0.
func TestBuiltinRaisedErrorValueIsErrorList(t *testing.T) {
	code := `try; x = length(5); except e (ANY); return e; endtry; return -1;`
	res := runBytecodeProgram(t, code, nil, nil)
	if res.Flow == types.FlowException {
		t.Fatalf("unexpected uncaught exception %s (val=%v)", res.Error, res.Val)
	}
	if res.Val.Type() != types.TYPE_LIST {
		t.Fatalf("caught value = %v (type %v), want LIST {code,msg,value,traceback}", res.Val, res.Val.Type())
	}
	first := res.Val.Get(1)
	if first.Type() != types.TYPE_ERR || first.Code() != types.E_TYPE {
		t.Fatalf("caught[1] = %v (type %v), want E_TYPE error code", first, first.Type())
	}
}

func TestUncaughtRaisePreservesExceptionValue(t *testing.T) {
	res := runBytecodeProgram(t, `raise(E_INVARG, "custom uncaught message", {7, 8});`, nil, nil)
	if res.Flow != types.FlowException || res.Error != types.E_INVARG {
		t.Fatalf("result = {Flow:%v Error:%v Val:%v}, want uncaught E_INVARG", res.Flow, res.Error, res.Val)
	}
	if res.Val.Type() != types.TYPE_LIST || res.Val.Len() != 4 {
		t.Fatalf("uncaught value = %v, want four-element exception list", res.Val)
	}
	if got := res.Val.Get(2); got.Type() != types.TYPE_STR || got.Str() != "custom uncaught message" {
		t.Errorf("uncaught message = %v, want custom uncaught message", got)
	}
	wantValue := types.NewList([]types.Value{types.NewInt(7), types.NewInt(8)})
	if got := res.Val.Get(3); !got.Equal(wantValue) {
		t.Errorf("uncaught custom value = %v, want %v", got, wantValue)
	}
}
