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
	const expected = "a524c2e564e4889ec0101d7df08910ea30e91df48828a52b76ffe9f90408646e"
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
