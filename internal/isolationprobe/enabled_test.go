//go:build elle

package isolationprobe

import "testing"

func TestCommitExclusionFaultInactiveOutsideHeldPair(t *testing.T) {
	if ActiveFault() != "commit-exclusion" {
		t.Skip("run with BARN_ELLE_FAULT=commit-exclusion")
	}
	if Fault("commit-exclusion") {
		t.Fatal("commit-exclusion active outside its held write-skew pair; concurrent WAIF waves can panic before the witness")
	}
	SetCommitExclusionPair(true)
	defer SetCommitExclusionPair(false)
	if !Fault("commit-exclusion") {
		t.Fatal("selected fault is not active in the held pair")
	}
	SetCommitExclusionPair(false)
	if Fault("commit-exclusion") {
		t.Fatal("fault leaked beyond the held pair")
	}
}
