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
	// Signature gained PromotedFloat, set on the 22 float math builtins whose
	// FLOAT positions admit INT under PROMOTE_NUMBERS.
	// Signature gained IncarnationArgs; valid's is STR, the Mongoose fork's
	// optional expected incarnation_id, admitted only under VALID_INCARNATION.
	// ToastStunt 61f2b7a registers the flag positions of isa, locate_by_name,
	// occupants, locations, explode, sort, queued_tasks, switch_player,
	// set_thread_mode, is_member, maphaskey, file_grep and pcre_match as
	// TYPE_FLAG (-3, INT or BOOL) instead of TYPE_INT.
	const expected = "7f7827abd7af3e16b643cc0d4adca5b600d50f2419798b07dc31dfc8372c6bda"
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
