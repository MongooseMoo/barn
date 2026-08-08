package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func containsObjID(s []types.ObjID, id types.ObjID) bool {
	for _, x := range s {
		if x == id {
			return true
		}
	}
	return false
}

// commitMove stages what->where through a fresh txn and commits it, returning the
// commit error code. It asserts the move did not take the coarse live-mutation path.
func commitMove(t *testing.T, s *Store, what, where types.ObjID, pos int64) types.ErrorCode {
	t.Helper()
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	if ec := tx.MoveObject(what, where, pos); ec != types.E_NONE {
		t.Fatalf("MoveObject(%v,%v): %v", what, where, ec)
	}
	if tx.liveMutated {
		t.Fatalf("txn move marked liveMutated — took the coarse stop-the-world path")
	}
	return tx.Commit()
}

func TestTxnMoveDecentralizedMaintainsContents(t *testing.T) {
	s, ids := immutFixture(t, 3)
	roomA, roomB, x := ids[0], ids[1], ids[2]

	if ec := commitMove(t, s, x, roomA, 0); ec != types.E_NONE {
		t.Fatalf("commit move to A: %v", ec)
	}
	if !containsObjID(s.load(roomA).contents, x) {
		t.Fatalf("A.contents missing x after first move")
	}

	if ec := commitMove(t, s, x, roomB, 0); ec != types.E_NONE {
		t.Fatalf("commit move to B: %v", ec)
	}
	if loc := s.load(x).location; loc != roomB {
		t.Errorf("x.location = %v, want %v", loc, roomB)
	}
	if !containsObjID(s.load(roomB).contents, x) {
		t.Errorf("B.contents missing x")
	}
	if containsObjID(s.load(roomA).contents, x) {
		t.Errorf("A.contents still has x after move away")
	}
}

// TestTxnMoveDisjointRoomsCommitInParallel: two moves between disjoint room pairs,
// opened at the same snapshot, must BOTH commit — proving disjoint moves no longer
// serialize (the whole point of taking move off the coarse path).
func TestTxnMoveDisjointRoomsCommitInParallel(t *testing.T) {
	s, ids := immutFixture(t, 6)
	roomA, roomB, roomC, roomD, x, y := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]
	if ec := commitMove(t, s, x, roomA, 0); ec != types.E_NONE {
		t.Fatalf("seed x->A: %v", ec)
	}
	if ec := commitMove(t, s, y, roomC, 0); ec != types.E_NONE {
		t.Fatalf("seed y->C: %v", ec)
	}

	// Two concurrent snapshots.
	tx1 := s.BeginReadOnly(0)
	defer tx1.Release()
	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()

	if ec := tx1.MoveObject(x, roomB, 0); ec != types.E_NONE {
		t.Fatalf("tx1 move x->B: %v", ec)
	}
	if ec := tx2.MoveObject(y, roomD, 0); ec != types.E_NONE {
		t.Fatalf("tx2 move y->D: %v", ec)
	}
	if ec := tx1.Commit(); ec != types.E_NONE {
		t.Fatalf("tx1 commit: %v", ec)
	}
	if ec := tx2.Commit(); ec != types.E_NONE {
		t.Errorf("tx2 commit conflicted with a DISJOINT move: %v (false conflict)", ec)
	}
	if s.load(x).location != roomB || s.load(y).location != roomD {
		t.Errorf("moves not applied: x@%v y@%v", s.load(x).location, s.load(y).location)
	}
}

// TestTxnMoveSameRoomCommutes: two moves of DIFFERENT objects INTO the same room,
// opened at the same snapshot, must BOTH commit — the contents edits are commutative
// setadds, so neither records a read dep on the room and there is no conflict; both
// objects end up in the room (no lost update). This is the whole point of the
// commutative-contents design: same-room moves serialize cheaply instead of aborting.
func TestTxnMoveSameRoomCommutes(t *testing.T) {
	s, ids := immutFixture(t, 5)
	roomA, roomB, dest, x, y := ids[0], ids[1], ids[2], ids[3], ids[4]
	if ec := commitMove(t, s, x, roomA, 0); ec != types.E_NONE {
		t.Fatalf("seed x->A: %v", ec)
	}
	if ec := commitMove(t, s, y, roomB, 0); ec != types.E_NONE {
		t.Fatalf("seed y->B: %v", ec)
	}

	tx1 := s.BeginReadOnly(0)
	defer tx1.Release()
	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()

	if ec := tx1.MoveObject(x, dest, 0); ec != types.E_NONE {
		t.Fatalf("tx1 move x->dest: %v", ec)
	}
	if ec := tx2.MoveObject(y, dest, 0); ec != types.E_NONE {
		t.Fatalf("tx2 move y->dest: %v", ec)
	}
	if ec := tx1.Commit(); ec != types.E_NONE {
		t.Fatalf("tx1 commit: %v", ec)
	}
	if ec := tx2.Commit(); ec != types.E_NONE {
		t.Errorf("tx2 conflicted on a commutative same-room move: %v (should commute)", ec)
	}
	// Both objects landed in the room — no lost update.
	c := s.load(dest).contents
	if !containsObjID(c, x) || !containsObjID(c, y) {
		t.Errorf("dest.contents = %v, want both x=%v and y=%v", c, x, y)
	}
}
