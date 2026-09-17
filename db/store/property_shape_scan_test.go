package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// shapeFixture: parent defines aliases and notifies; child inherits both (clear
// slots) and has an own value for notifies. Neither has a slot for "nosuch",
// so a child.nosuch lookup falls through child and parent to E_PROPNF — the
// `x.p ! E_PROPNF` idiom, which is what records scan dependencies on hot
// objects like Mongoose's #330.
func shapeFixture(t *testing.T) (*Store, types.ObjID, types.ObjID) {
	t.Helper()
	s, parent, child := elisionFixture(t)
	if ec := s.DirectTxn().DefineProperty(parent, "notifies", NewProperty(strList(), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty notifies: %v", ec)
	}
	if ec := s.DirectTxn().SetPropertyValue(child, "notifies", strList("x")); ec != types.E_NONE {
		t.Fatalf("seed child.notifies: %v", ec)
	}
	return s, parent, child
}

// walkThenCommit looks up child.nosuch (falling through child and parent),
// lets a concurrent writer run, then writes parent.aliases and commits;
// returns the commit code.
func walkThenCommit(t *testing.T, s *Store, parent, child types.ObjID, concurrent func()) types.ErrorCode {
	t.Helper()
	walker := s.BeginReadOnly(0)
	defer walker.Release()
	if _, ec := walker.PropertyValue(child, "nosuch"); ec != types.E_PROPNF {
		t.Fatalf("child.nosuch = %v, want E_PROPNF", ec)
	}
	if len(walker.propertyScans) != 0 {
		t.Fatalf("fall-through walk recorded a coarse property scan: %v", walker.propertyScans)
	}
	for _, id := range []types.ObjID{child, parent} {
		if _, ok := walker.propertyShapeScans[id]; !ok {
			t.Fatalf("fall-through walk did not record a shape scan on #%d: %v", id, walker.propertyShapeScans)
		}
	}
	concurrent()
	if ec := walker.SetPropertyValue(parent, "aliases", strList("w")); ec != types.E_NONE {
		t.Fatalf("walker SetPropertyValue: %v", ec)
	}
	return walker.Commit()
}

func commitWrite(t *testing.T, s *Store, f func(tx *StoreTxn) types.ErrorCode) {
	t.Helper()
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	if ec := f(tx); ec != types.E_NONE {
		t.Fatalf("stage: %v", ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit: %v", ec)
	}
}

// A value write to an existing slot on a fallen-through object is not a
// conflict for the walker: it cannot change where the walk lands.
func TestValueWriteDoesNotInvalidateFallThroughWalk(t *testing.T) {
	s, parent, child := shapeFixture(t)
	ec := walkThenCommit(t, s, parent, child, func() {
		commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode {
			return tx.SetPropertyValue(child, "notifies", strList("y"))
		})
	})
	if ec != types.E_NONE {
		t.Fatalf("walker commit after value write on fallen-through object = %v, want E_NONE", ec)
	}
}

// Defining the looked-up name on the child changes the walk's result and must
// still conflict.
func TestDefineOnChildInvalidatesFallThroughWalk(t *testing.T) {
	s, parent, child := shapeFixture(t)
	ec := walkThenCommit(t, s, parent, child, func() {
		commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode {
			return tx.DefineProperty(child, "nosuch", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true))
		})
	})
	if ec != types.E_INVARG {
		t.Fatalf("walker commit after define on child = %v, want E_INVARG", ec)
	}
}

// Defining it on the parent (which propagates a clear slot to the child) must
// still conflict too.
func TestDefineOnParentInvalidatesFallThroughWalk(t *testing.T) {
	s, parent, child := shapeFixture(t)
	ec := walkThenCommit(t, s, parent, child, func() {
		commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode {
			return tx.DefineProperty(parent, "nosuch", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true))
		})
	})
	if ec != types.E_INVARG {
		t.Fatalf("walker commit after define on parent = %v, want E_INVARG", ec)
	}
}

// Reparenting the fallen-through object changes what the walk reaches next and
// must still conflict.
func TestChparentInvalidatesFallThroughWalk(t *testing.T) {
	s, parent, child := shapeFixture(t)
	other, ec := s.DirectTxn().CreateObject(nil, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject other: %v", ec)
	}
	if ec := s.DirectTxn().DefineProperty(other, "nosuch", NewProperty(strList("q"), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty other.nosuch: %v", ec)
	}
	ec = walkThenCommit(t, s, parent, child, func() {
		if ec := s.ChangeParents(child, []types.ObjID{other}); ec != types.E_NONE {
			t.Fatalf("ChangeParents: %v", ec)
		}
	})
	if ec != types.E_INVARG {
		t.Fatalf("walker commit after chparent of fallen-through object = %v, want E_INVARG", ec)
	}
}
