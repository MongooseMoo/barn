package store

import (
	"testing"

	"barn/types"
)

func TestStoreTxnFailedFlushPreservesPrivateViewAndReadTracking(t *testing.T) {
	s := newCoarseAtomicTestStore(t)
	if _, errCode := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb: %v", errCode)
	}
	tx := s.BeginReadOnly(0)
	if _, _, err := tx.FindVerb(0, "look"); err != nil {
		t.Fatalf("FindVerb memo setup: %v", err)
	}
	if errCode := tx.SetObjectName(0, "private"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName stage: %v", errCode)
	}
	lazySet(&tx.verbWrites, verbWriteKey{objID: 1, name: "missing"}, verbWrite{code: []string{"return 1;"}})

	cached := tx.objects[0]
	wantScalarRead := tx.scalarReads[0]
	if cached == nil || !tx.owned[0] || tx.resolveCacheActive() {
		t.Fatal("test setup did not establish an owned private view with the resolution memo disabled")
	}

	if errCode := tx.FlushStagedToLive(); errCode != types.E_VERBNF {
		t.Fatalf("FlushStagedToLive = %v, want E_VERBNF", errCode)
	}
	if tx.objects[0] != cached {
		t.Error("failed flush replaced the transaction's private object view")
	}
	if got := tx.scalarReads[0]; got != wantScalarRead {
		t.Errorf("failed flush scalar read = %d, want preserved %d", got, wantScalarRead)
	}
	if !tx.owned[0] || tx.resolveCacheActive() {
		t.Error("failed flush discarded ownership or re-enabled the resolution memo")
	}
	if got, errCode := tx.ObjectName(0); errCode != types.E_NONE || got != "private" {
		t.Errorf("failed flush private name = %q, %v; want private, E_NONE", got, errCode)
	}
}
