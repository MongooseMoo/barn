package types

import "testing"

// TestErrResultValIsNone locks the None-sentinel contract for the error-raise
// path. Post-de-box the zero Value{} is integer 0 (tag TYPE_INT), NOT nil/None.
// Err(e) must set Val to the explicit None sentinel so that vm.HandleError's
// IsNone() branch fires and builds the standard {code,msg,value,traceback} list.
// Without this, a builtin-raised error surfaces as bare int 0.
func TestErrResultValIsNone(t *testing.T) {
	r := Err(E_TYPE)
	if r.Flow != FlowException || r.Error != E_TYPE {
		t.Fatalf("Err(E_TYPE) = {Flow:%v Error:%v}, want {FlowException E_TYPE}", r.Flow, r.Error)
	}
	if !r.Val.IsNone() {
		t.Fatalf("Err(E_TYPE).Val.IsNone() = false (Val=%v, type=%v); want true so HandleError's IsNone branch fires", r.Val, r.Val.Type())
	}
}
