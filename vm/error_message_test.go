package vm

import (
	"barn/types"
	"testing"
)

func TestExtractErrorMessage_VMExceptionStringPayload(t *testing.T) {
	err := VMException{
		Code:  types.E_ARGS,
		Value: types.NewStr("Incorrect number of arguments (expected 1; got 0)"),
	}

	msg := extractErrorMessage(err, types.E_ARGS)
	if msg != "Incorrect number of arguments (expected 1; got 0)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestExtractErrorMessage_BareCodeStringUsesDefaultMessage(t *testing.T) {
	err := MooError{Code: types.E_INVARG, Message: "E_INVARG"}
	msg := extractErrorMessage(err, types.E_INVARG)
	if msg != types.E_INVARG.Message() {
		t.Fatalf("expected %q, got %q", types.E_INVARG.Message(), msg)
	}
}
