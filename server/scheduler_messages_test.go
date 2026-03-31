package server

import (
	"barn/types"
	"testing"
)

func TestNormalizeExceptionMessage_BareCodeString(t *testing.T) {
	msg := normalizeExceptionMessage("E_INVARG", types.E_INVARG)
	if msg != types.E_INVARG.Message() {
		t.Fatalf("expected %q, got %q", types.E_INVARG.Message(), msg)
	}
}

func TestNormalizeExceptionMessage_CodePrefixWithText(t *testing.T) {
	msg := normalizeExceptionMessage("E_INVARG: Invalid mode string", types.E_INVARG)
	if msg != "Invalid mode string" {
		t.Fatalf("expected trimmed message, got %q", msg)
	}
}
