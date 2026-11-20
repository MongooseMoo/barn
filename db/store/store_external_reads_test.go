package store

import (
	"context"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestExternalReadValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writes bool
		bump   bool
		want   types.ErrorCode
	}{
		{"writer_unchanged", true, false, types.E_NONE},
		{"writer_after_external_write", true, true, types.E_INVARG},
		{"no_writes_unchanged", false, false, types.E_NONE},
		{"no_writes_after_external_write", false, true, types.E_INVARG},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newBoundaryRenewStore(t)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			tx.NoteExternalRead(s.ExternalWrites())
			if !tx.HasWrites() {
				t.Fatal("an external read must make the transaction commit")
			}
			if tc.writes {
				if ec := tx.SetPropertyValue(0, "written", types.NewInt(5)); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			if tc.bump {
				s.BumpExternalWrites()
			}
			ec := tx.Commit()
			if ec != tc.want || tx.ValidationFailed() != (tc.want != types.E_NONE) {
				t.Fatalf("commit = %v (validation failed %v), want %v", ec, tx.ValidationFailed(), tc.want)
			}
			if tc.want != types.E_NONE {
				if len(tx.conflicts) != 1 || tx.conflicts[0].Kind != ConflictExternal {
					t.Fatalf("conflicts = %+v, want one external conflict", tx.conflicts)
				}
			} else if tx.HasWrites() {
				t.Fatal("a committed transaction still reports work to commit")
			}
		})
	}
}

// A no-write transaction with an external read is validated on every store
// read, not only the external one: its external read was made after its
// snapshot.
func TestExternalReadValidatesStoreReadsOfNoWriteTransaction(t *testing.T) {
	s := newBoundaryRenewStore(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if _, ec := tx.PropertyValue(0, "seen"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	tx.NoteExternalRead(s.ExternalWrites())
	if ec := s.DirectTxn().SetPropertyValue(0, "seen", types.NewInt(9)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.Commit(); ec != types.E_INVARG || !tx.ValidationFailed() {
		t.Fatalf("commit = %v (validation failed %v), want a property conflict", ec, tx.ValidationFailed())
	}
}

// The boundary validates the external read; the renewed transaction, which
// runs under the exclusive gate, does not carry it.
func TestExternalReadAtBoundary(t *testing.T) {
	s := newBoundaryRenewStore(t)
	stale := s.BeginSnapshot(0)
	stale.NoteExternalRead(s.ExternalWrites())
	s.BumpExternalWrites()
	if _, _, ec := stale.CommitAndRenewCarryingReads(); ec != types.E_INVARG || !stale.ValidationFailed() {
		t.Fatalf("boundary after an external write = %v, want E_INVARG", ec)
	}
	stale.Release()

	tx := s.BeginSnapshot(0)
	tx.NoteExternalRead(s.ExternalWrites())
	next, published, ec := tx.CommitAndRenewCarryingReads()
	if ec != types.E_NONE || published {
		t.Fatalf("boundary = %v published %v, want success with nothing published", ec, published)
	}
	defer next.Release()
	if next.HasWrites() {
		t.Fatal("the renewed transaction carried the external read")
	}
}

// An attempt holding the exclusive gate records no external read: nothing can
// commit before it, and it must not lose its commit.
func TestExternalReadNotRecordedUnderExclusiveGate(t *testing.T) {
	s := newBoundaryRenewStore(t)
	grant, err := s.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Release()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	tx.BindExclusiveGrant(grant)
	tx.NoteExternalRead(s.ExternalWrites())
	s.BumpExternalWrites()
	if tx.HasWrites() {
		t.Fatal("a gate-holding transaction recorded an external read")
	}
}
