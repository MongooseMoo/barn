package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestTxnCreateDecentralized: tx.CreateObject stages a create that commits on the
// decentralized path (not liveMutated); the new object exists post-commit with the
// right parent, the parent's children includes it, and max_object reflects it.
func TestTxnCreateDecentralized(t *testing.T) {
	s, ids := immutFixture(t, 1)
	parent := ids[0]

	tx := s.BeginReadOnly(0)
	newID, ec := tx.CreateObject([]types.ObjID{parent}, 3)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %v", ec)
	}
	if tx.liveMutated {
		t.Fatalf("create marked liveMutated — took the coarse path")
	}
	// Read-your-writes: the new object is visible inside the txn before commit.
	if !validLiveObject(tx.object(newID)) {
		t.Fatalf("new object not visible within the creating txn")
	}
	if tx.MaxObject() < newID {
		t.Errorf("tx.MaxObject() = %v, want >= newID %v (read-your-writes)", tx.MaxObject(), newID)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit: %v", ec)
	}
	tx.Release()

	obj := s.load(newID)
	if !validLiveObject(obj) {
		t.Fatalf("new object #%d not live post-commit", newID)
	}
	if len(obj.parents) != 1 || obj.parents[0] != parent {
		t.Errorf("new object parents = %v, want [%v]", obj.parents, parent)
	}
	if !containsObjID(s.load(parent).children, newID) {
		t.Errorf("parent #%d children = %v, missing new object #%d", parent, s.load(parent).children, newID)
	}
	if s.MaxObject() < newID {
		t.Errorf("store MaxObject() = %v, want >= %v", s.MaxObject(), newID)
	}
}

// TestTxnRecycleSimpleDecentralized: recycling a simple object (no children/contents)
// commits decentralized; the object becomes a recycled tombstone and is removed from
// its parent's children.
func TestTxnRecycleSimpleDecentralized(t *testing.T) {
	s, ids := immutFixture(t, 1)
	parent := ids[0]

	tx0 := s.BeginReadOnly(0)
	obj, ec := tx0.CreateObject([]types.ObjID{parent}, 3)
	if ec != types.E_NONE {
		t.Fatalf("setup create: %v", ec)
	}
	if ec := tx0.Commit(); ec != types.E_NONE {
		t.Fatalf("setup commit: %v", ec)
	}
	tx0.Release()
	if !containsObjID(s.load(parent).children, obj) {
		t.Fatalf("setup: parent should contain obj")
	}

	tx := s.BeginReadOnly(0)
	handled, ec := tx.RecycleObject(obj)
	if !handled || ec != types.E_NONE {
		t.Fatalf("RecycleObject handled=%v ec=%v", handled, ec)
	}
	if tx.liveMutated {
		t.Fatalf("recycle marked liveMutated — took the coarse path")
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit: %v", ec)
	}
	tx.Release()

	o := s.load(obj)
	if o == nil || !o.recycled {
		t.Errorf("obj #%d should be a recycled tombstone, got %v", obj, o)
	}
	if containsObjID(s.load(parent).children, obj) {
		t.Errorf("parent children still contains recycled obj #%d", obj)
	}
}

// TestTxnCreateThenRecycleSameTxn is the build-task shape: create an object then recycle
// it in ONE decentralized txn. The object commits as a recycled tombstone and the
// parent's children is net-unchanged (the create's add and the recycle's remove cancel).
func TestTxnCreateThenRecycleSameTxn(t *testing.T) {
	s, ids := immutFixture(t, 1)
	parent := ids[0]
	childrenBefore := len(s.load(parent).children)

	tx := s.BeginReadOnly(0)
	o, ec := tx.CreateObject([]types.ObjID{parent}, 3)
	if ec != types.E_NONE {
		t.Fatalf("create: %v", ec)
	}
	handled, ec := tx.RecycleObject(o)
	if !handled || ec != types.E_NONE {
		t.Fatalf("recycle handled=%v ec=%v", handled, ec)
	}
	if tx.liveMutated {
		t.Fatalf("create+recycle marked liveMutated — took the coarse path")
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit: %v", ec)
	}
	tx.Release()

	o2 := s.load(o)
	if o2 == nil || !o2.recycled {
		t.Errorf("#%d should be a recycled tombstone, got %v", o, o2)
	}
	if got := len(s.load(parent).children); got != childrenBefore {
		t.Errorf("parent children count %d -> %d; create+recycle should net zero", childrenBefore, got)
	}
}

// TestTxnCreateConcurrentSameParentBothCommit: two creates under the SAME parent, at
// the same snapshot, allocate DISTINCT ids and both commit (children is a commutative
// setadd; neither records a conflicting write on the parent). Both children land.
func TestTxnCreateConcurrentSameParentBothCommit(t *testing.T) {
	s, ids := immutFixture(t, 1)
	parent := ids[0]

	tx1 := s.BeginReadOnly(0)
	defer tx1.Release()
	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()

	id1, ec := tx1.CreateObject([]types.ObjID{parent}, 3)
	if ec != types.E_NONE {
		t.Fatalf("tx1 create: %v", ec)
	}
	id2, ec := tx2.CreateObject([]types.ObjID{parent}, 3)
	if ec != types.E_NONE {
		t.Fatalf("tx2 create: %v", ec)
	}
	if id1 == id2 {
		t.Fatalf("concurrent creates allocated the same id #%d", id1)
	}
	if ec := tx1.Commit(); ec != types.E_NONE {
		t.Fatalf("tx1 commit: %v", ec)
	}
	if ec := tx2.Commit(); ec != types.E_NONE {
		t.Errorf("tx2 conflicted creating under the same parent: %v (children adds should commute)", ec)
	}

	c := s.load(parent).children
	if !containsObjID(c, id1) || !containsObjID(c, id2) {
		t.Errorf("parent children = %v, want both #%d and #%d", c, id1, id2)
	}
	if !validLiveObject(s.load(id1)) || !validLiveObject(s.load(id2)) {
		t.Errorf("both created objects should be live: #%d live=%v, #%d live=%v",
			id1, validLiveObject(s.load(id1)), id2, validLiveObject(s.load(id2)))
	}
}
