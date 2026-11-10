package builtins

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

func TestNativeCatalogPreservesHistoricalContract(t *testing.T) {
	// Captured independently from d6d396e's centralized table. Pin both the
	// whole native order (including its appended tail) and every policy column.
	// Since then kill_task (ImplementationOwned) and resume (Irreversible) became
	// Transactional: they stage the target task's store row in the caller's
	// transaction. dump_database (CommitGateReentrant) became Transactional too:
	// it queues its checkpoint request as a commit-deferred effect.
	// file_count_lines and file_grep became Irreversible because their scans
	// move the shared file cursor, just like file_readline and file_seek.
	// exec became ImplementationOwned: it crosses the boundary itself, after its
	// argument, permission and path checks and just before starting the process.
	const expected = "afb7076529a2aa4d0ae0021d9ff4c061c6bab47adf7bf2e6bf74cdba8a84da0d"
	type contract struct {
		Name       string
		Signature  *Signature
		Visibility Visibility
		Effect     EffectPolicy
		LineSync   bool
		Capability uint64
	}
	var contracts []contract
	for _, d := range Descriptors()[:258] {
		sig := *d.Signature
		if len(sig.ArgTypes) == 0 {
			sig.ArgTypes = nil
		}
		if len(sig.Alternatives) == 0 {
			sig.Alternatives = nil
		}
		contracts = append(contracts, contract{d.Name, &sig, d.Visibility, d.Effect, d.LineSync, uint64(d.Capability)})
	}
	data, err := json.Marshal(contracts)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected {
		t.Fatalf("historical native catalog changed: %s", got)
	}
}

func TestCatalogRequiresCompleteVMContribution(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("partial VM contribution accepted")
		}
	}()
	Descriptors(testDescriptor("eval"))
}
