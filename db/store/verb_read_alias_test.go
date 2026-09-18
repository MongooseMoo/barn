package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestAliasedVerbReadCommits: a txn that calls (reads) a verb whose name is a
// multi-alias string ("look l*ook glance") and stages any write must commit
// cleanly. Regression for the retry storm found on mongoose.db: the loader
// stores Verb.name as the full space-separated alias string, but Object.verbs
// is keyed by names[0]; markVerbRead used verb.name as the read-set key, so
// validateVerbReadsLocked could never find the verb again and every commit
// failed E_INVARG — each affected task then re-ran to the 64-attempt retry cap
// (~300ms and ~100MB of allocations per command on the real database).
func TestAliasedVerbReadCommits(t *testing.T) {
	s, ids := immutFixture(t, 1)
	obj := ids[0]

	full := "look l*ook glance"
	v := NewVerb(full, []string{"look", "l*ook", "glance"}, 0,
		VerbRead|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"return 1;"})
	if _, ec := s.AddVerb(obj, v); ec != types.E_NONE {
		t.Fatalf("AddVerb: %v", ec)
	}
	p := NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)
	if ec := s.DirectTxn().DefineProperty(obj, "counter", p); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()
	// Resolve the verb through an alias — this records a verb read.
	if _, _, err := tx.FindCallableVerb(obj, "glance"); err != nil {
		t.Fatalf("FindCallableVerb(glance): %v", err)
	}
	// Stage a write so Commit actually validates the read set.
	if ec := tx.SetPropertyValue(obj, "counter", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit failed: %v (validationFail=%v) — aliased verb read-set key "+
			"does not match the live verbs map key", ec, tx.ValidationFailed())
	}
}
