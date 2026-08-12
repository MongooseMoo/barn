package parser

import "testing"

func TestQuoteMOOStringPreservesNULByte(t *testing.T) {
	if got, want := quoteMOOString("nul\x00byte"), "\"nul\x00byte\""; got != want {
		t.Fatalf("quoteMOOString() = %q, want %q", got, want)
	}
}
