package store

import (
	"context"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// renumberUnderGate runs Store.Renumber the way the renumber() builtin does,
// under the exclusive commit gate, while other transactions stay open.
func renumberUnderGate(t *testing.T, s *Store, oldID, newID types.ObjID) {
	t.Helper()
	grant, err := s.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Release()
	if err := s.Renumber(oldID, newID); err != nil {
		t.Fatalf("Renumber(#%d, #%d): %v", oldID, newID, err)
	}
}

// hasConflict reports whether conflicts holds a stale read of kind on objID
// (and, for a property, on name).
func hasConflict(conflicts []ReadConflict, kind ConflictKind, objID types.ObjID, name string) bool {
	for _, c := range conflicts {
		if c.Kind == kind && c.ObjID == objID && (name == "" || c.Name == name) {
			return true
		}
	}
	return false
}

// TestRenumberPropertyOwnerFixInvalidatesConcurrentSlotRead is the history
// FuzzStoreCommitsMatchSerialReplay found (rapid seed 6666356451387711135):
// #0 is recycled, a transaction sets #1.a, renumber(#2) takes the freed #0 and
// clears every owner equal to #0 to #-1 (Toast db_renumber_object), and then the
// transaction commits. Serially the renumber comes first or the commit fails,
// so #1.a must end owned by #-1. The transaction's staged slot carries the
// owner it read (#0); it must fail validation instead of writing that back.
func TestRenumberPropertyOwnerFixInvalidatesConcurrentSlotRead(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, types.ObjNothing)); err != nil {
		t.Fatal(err)
	}
	d := s.DirectTxn()
	for _, parent := range []types.ObjID{0, 1, 0} {
		if _, ec := d.CreateObject([]types.ObjID{parent}, types.ObjNothing); ec != types.E_NONE {
			t.Fatalf("create under #%d: %s", parent, ec)
		}
	}
	for id, name := range []string{"a", "b"} {
		id := types.ObjID(id)
		if ec := d.DefineProperty(id, name, NewProperty(types.NewInt(0), id, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatalf("define #%d.%s: %s", id, name, ec)
		}
	}
	if err := s.Recycle(0); err != nil {
		t.Fatal(err)
	}

	tx := s.BeginSnapshot(0)
	if ec := tx.SetPropertyValue(1, "a", types.NewInt(0)); ec != types.E_NONE {
		t.Fatalf("set #1.a: %s", ec)
	}
	renumberUnderGate(t, s, 2, 0)

	next, _, ec := tx.CommitAndRenewCarryingReads()
	if ec == types.E_NONE {
		next.Release()
		t.Errorf("commit after renumber rewrote #1.a's owner validated; it read the slot before the rewrite")
	} else {
		if !hasConflict(tx.Conflicts(), ConflictProperty, 1, "a") {
			t.Errorf("conflicts = %+v, want a stale read of #1.a", tx.Conflicts())
		}
		tx.Release()
	}

	// The slot itself: with the commit refused it is still clear, and #1 has no
	// parent left to resolve its value through.
	prop, _, ec := s.DirectTxn().LocalProperty(1, "a")
	if ec != types.E_NONE {
		t.Fatalf("find #1.a: %s", ec)
	}
	if prop.Owner != types.ObjNothing {
		t.Errorf("#1.a owner = #%d, want #-1 (renumber cleared the owner equal to the new id)", prop.Owner)
	}
}

// TestRenumberVerbOwnerFixInvalidatesConcurrentVerbRead checks that renumber's
// rewrite of a verbdef owner gives that verb a new version, as an ordinary
// committed write of the verb does, so a transaction's read of the verb is
// stale at validation.
func TestRenumberVerbOwnerFixInvalidatesConcurrentVerbRead(t *testing.T) {
	s := NewStore()
	for id := types.ObjID(0); id <= 2; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if _, ec := s.AddVerb(0, NewVerb([]string{"greet"}, 2, VerbRead|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "none"}, []string{"return 0;"})); ec != types.E_NONE {
		t.Fatalf("add verb: %s", ec)
	}
	if ec := s.DirectTxn().DefineProperty(1, "x", NewProperty(types.NewInt(0), 1, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("define #1.x: %s", ec)
	}

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	verb, err := tx.FindVerbOnObject(0, "greet")
	if err != nil {
		t.Fatal(err)
	}
	if verb.Owner != 2 {
		t.Fatalf("verb owner before renumber = #%d, want #2", verb.Owner)
	}
	if ec := tx.SetPropertyValue(1, "x", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("set #1.x: %s", ec)
	}
	renumberUnderGate(t, s, 2, 3)

	if ec := tx.Commit(); ec == types.E_NONE {
		t.Fatalf("commit validated after renumber rewrote the owner of a verb it read")
	}
	if !hasConflict(tx.Conflicts(), ConflictVerb, 0, "") {
		t.Errorf("conflicts = %+v, want a stale read of the verb #0:greet", tx.Conflicts())
	}
}

// TestRenumberAnonymousRewritesInvalidateConcurrentReads checks that the
// rewrites renumber makes on an anonymous object (its parent, its owner and its
// slot owners) move the versions a transaction's reads of them recorded.
func TestRenumberAnonymousRewritesInvalidateConcurrentReads(t *testing.T) {
	cases := []struct {
		name string
		kind ConflictKind
		prop string
		read func(tx *StoreTxn, anon types.ObjID) types.ErrorCode
	}{
		{"parents", ConflictRelationship, "", func(tx *StoreTxn, anon types.ObjID) types.ErrorCode {
			_, ec := tx.Parents(anon)
			return ec
		}},
		{"owner", ConflictScalar, "", func(tx *StoreTxn, anon types.ObjID) types.ErrorCode {
			_, ec := tx.ObjectOwner(anon)
			return ec
		}},
		{"property owner", ConflictProperty, "p", func(tx *StoreTxn, anon types.ObjID) types.ErrorCode {
			_, ec := tx.FindProperty(anon, "p")
			return ec
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			for id := types.ObjID(0); id <= 2; id++ {
				if err := s.Add(NewObject(id, 0)); err != nil {
					t.Fatal(err)
				}
			}
			// #2 is the free id renumber(#1) moves to; the anonymous object
			// takes a fresh id above it.
			if err := s.Recycle(2); err != nil {
				t.Fatal(err)
			}
			d := s.DirectTxn()
			// #1 defines p owned by #1; the anonymous child is owned by #1.
			if ec := d.DefineProperty(1, "p", NewProperty(types.NewInt(0), 1, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatalf("define #1.p: %s", ec)
			}
			anon, ec := d.CreateObject([]types.ObjID{1}, 1, true)
			if ec != types.E_NONE {
				t.Fatalf("create anonymous child: %s", ec)
			}
			if ec := d.DefineProperty(0, "x", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatalf("define #0.x: %s", ec)
			}

			tx := s.BeginSnapshot(0)
			defer tx.Release()
			if ec := tc.read(tx, anon); ec != types.E_NONE {
				t.Fatalf("read: %s", ec)
			}
			if ec := tx.SetPropertyValue(0, "x", types.NewInt(1)); ec != types.E_NONE {
				t.Fatalf("set #0.x: %s", ec)
			}
			renumberUnderGate(t, s, 1, 2)

			if ec := tx.Commit(); ec == types.E_NONE {
				t.Fatalf("commit validated after renumber rewrote the anonymous object's %s", tc.name)
			}
			if !hasConflict(tx.Conflicts(), tc.kind, anon, tc.prop) {
				t.Errorf("conflicts = %+v, want a stale %s read of the anonymous object", tx.Conflicts(), tc.name)
			}
		})
	}
}
