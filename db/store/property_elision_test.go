package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func strList(items ...string) types.Value {
	vals := make([]types.Value, len(items))
	for i, s := range items {
		vals[i] = types.NewStr(s)
	}
	return types.NewList(vals)
}

func elisionFixture(t *testing.T) (*Store, types.ObjID, types.ObjID) {
	t.Helper()
	s, ids := immutFixture(t, 1)
	parent := ids[0]
	if ec := s.DirectTxn().DefineProperty(parent, "aliases", NewProperty(strList("a", "b"), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	child, ec := s.DirectTxn().CreateObject([]types.ObjID{parent}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}
	return s, parent, child
}

func liveSlotVersion(t *testing.T, s *Store, id types.ObjID, name string) uint64 {
	t.Helper()
	_, prop, ok := propertyByName(s.load(id).properties, name)
	if !ok {
		t.Fatalf("#%d.%s: no live slot", id, name)
	}
	return prop.version
}

// A write of an Identical value to a non-clear slot stages nothing, clones
// nothing, commits as a read-only txn, and leaves the slot version alone.
func TestSetPropertyValueSameValueIsElided(t *testing.T) {
	s, parent, _ := elisionFixture(t)
	v0 := liveSlotVersion(t, s, parent, "aliases")
	e0 := s.PropertyWriteElisions()

	tx := s.BeginReadOnly(0)
	defer tx.Release()
	if ec := tx.SetPropertyValue(parent, "aliases", strList("a", "b")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if len(tx.propertyWrites) != 0 {
		t.Fatalf("same-value write was staged: %v", tx.propertyWrites)
	}
	if tx.owned[parent] {
		t.Fatalf("same-value write privatized the object")
	}
	if _, read := tx.propertyReads[propertyReadKey{objID: parent, name: "aliases"}]; !read {
		t.Fatalf("elided write did not record a read dependency")
	}
	got, ec := tx.PropertyValue(parent, "aliases")
	if ec != types.E_NONE || !got.Identical(strList("a", "b")) {
		t.Fatalf("read-your-writes after elision = %v (%v)", got, ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	if v1 := liveSlotVersion(t, s, parent, "aliases"); v1 != v0 {
		t.Fatalf("slot version moved %d -> %d on an elided write", v0, v1)
	}
	if s.PropertyWriteElisions() != e0+1 {
		t.Fatalf("elision counter = %d, want %d", s.PropertyWriteElisions(), e0+1)
	}
}

// Two concurrent same-value writers (the Mongoose #6:title shape) no longer
// conflict; a real change still invalidates a concurrent reader.
func TestSameValueWritersDoNotConflict(t *testing.T) {
	s, parent, child := elisionFixture(t)

	reader := s.BeginReadOnly(0)
	defer reader.Release()
	if _, ec := reader.PropertyValue(parent, "aliases"); ec != types.E_NONE {
		t.Fatalf("reader PropertyValue: %v", ec)
	}

	writer := s.BeginReadOnly(0)
	if ec := writer.SetPropertyValue(parent, "aliases", strList("a", "b")); ec != types.E_NONE {
		t.Fatalf("writer SetPropertyValue: %v", ec)
	}
	if ec := writer.Commit(); ec != types.E_NONE {
		t.Fatalf("writer Commit: %v", ec)
	}
	writer.Release()

	// The reader writes elsewhere and commits: must not see a conflict.
	if ec := reader.SetPropertyValue(child, "aliases", strList("c")); ec != types.E_NONE {
		t.Fatalf("reader SetPropertyValue child: %v", ec)
	}
	if ec := reader.Commit(); ec != types.E_NONE {
		t.Fatalf("reader Commit after same-value writer: %v (want E_NONE)", ec)
	}

	// Control: a different value still conflicts.
	reader2 := s.BeginReadOnly(0)
	defer reader2.Release()
	if _, ec := reader2.PropertyValue(parent, "aliases"); ec != types.E_NONE {
		t.Fatalf("reader2 PropertyValue: %v", ec)
	}
	writer2 := s.BeginReadOnly(0)
	if ec := writer2.SetPropertyValue(parent, "aliases", strList("a", "b", "z")); ec != types.E_NONE {
		t.Fatalf("writer2 SetPropertyValue: %v", ec)
	}
	if ec := writer2.Commit(); ec != types.E_NONE {
		t.Fatalf("writer2 Commit: %v", ec)
	}
	writer2.Release()
	if ec := reader2.SetPropertyValue(child, "aliases", strList("d")); ec != types.E_NONE {
		t.Fatalf("reader2 SetPropertyValue child: %v", ec)
	}
	if ec := reader2.Commit(); ec != types.E_INVARG {
		t.Fatalf("reader2 Commit after real change = %v, want E_INVARG", ec)
	}
}

// The elision decision depends on the value seen at the time of the write, so
// an elided writer must still lose validation if the slot changed underneath it.
func TestElidedWriteKeepsReadDependency(t *testing.T) {
	s, parent, child := elisionFixture(t)

	elider := s.BeginReadOnly(0)
	defer elider.Release()
	if ec := elider.SetPropertyValue(parent, "aliases", strList("a", "b")); ec != types.E_NONE {
		t.Fatalf("elider SetPropertyValue: %v", ec)
	}

	other := s.BeginReadOnly(0)
	if ec := other.SetPropertyValue(parent, "aliases", strList("x")); ec != types.E_NONE {
		t.Fatalf("other SetPropertyValue: %v", ec)
	}
	if ec := other.Commit(); ec != types.E_NONE {
		t.Fatalf("other Commit: %v", ec)
	}
	other.Release()

	if ec := elider.SetPropertyValue(child, "aliases", strList("c")); ec != types.E_NONE {
		t.Fatalf("elider SetPropertyValue child: %v", ec)
	}
	if ec := elider.Commit(); ec != types.E_INVARG {
		t.Fatalf("elider Commit = %v, want E_INVARG (slot changed after elision)", ec)
	}
}

// Writes that change something observable are never elided: a clear slot
// becoming an own value, and a case-different string (Equal but not Identical).
func TestSetPropertyValueNotElidedWhenObservable(t *testing.T) {
	s, parent, child := elisionFixture(t)

	// Child inherits aliases (clear). Writing the parent's exact value must
	// still make the child's slot non-clear.
	if clear, ec := s.DirectTxn().PropertyClearState(child, "aliases"); ec != types.E_NONE || !clear {
		t.Fatalf("child aliases clear state before = %v (%v), want clear", clear, ec)
	}
	tx := s.BeginReadOnly(0)
	if ec := tx.SetPropertyValue(child, "aliases", strList("a", "b")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue child: %v", ec)
	}
	if len(tx.propertyWrites) != 1 {
		t.Fatalf("clear-slot write was elided: staged %d writes", len(tx.propertyWrites))
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	tx.Release()
	if clear, ec := s.DirectTxn().PropertyClearState(child, "aliases"); ec != types.E_NONE || clear {
		t.Fatalf("child aliases clear state after = %v (%v), want not clear", clear, ec)
	}

	// Case differs: MOO == says equal, but the stored bytes are observable.
	v0 := liveSlotVersion(t, s, parent, "aliases")
	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()
	if ec := tx2.SetPropertyValue(parent, "aliases", strList("A", "b")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue case-different: %v", ec)
	}
	if len(tx2.propertyWrites) != 1 {
		t.Fatalf("case-different write was elided")
	}
	if ec := tx2.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	if v1 := liveSlotVersion(t, s, parent, "aliases"); v1 == v0 {
		t.Fatalf("slot version did not move on a real write")
	}
	got, _ := s.DirectTxn().PropertyValue(parent, "aliases")
	if got.Get(1).Str() != "A" {
		t.Fatalf("stored value = %v, want case preserved", got)
	}
}

// title's `setadd(x, "The")` then `setremove(x, "the")` stages an intermediate
// value and then restores the original: the net write must be dropped.
func TestRoundTripWriteIsElided(t *testing.T) {
	s, parent, _ := elisionFixture(t)
	v0 := liveSlotVersion(t, s, parent, "aliases")

	tx := s.BeginReadOnly(0)
	defer tx.Release()
	if ec := tx.SetPropertyValue(parent, "aliases", strList("a", "b", "The")); ec != types.E_NONE {
		t.Fatalf("first write: %v", ec)
	}
	if len(tx.propertyWrites) != 1 {
		t.Fatalf("intermediate write not staged")
	}
	if ec := tx.SetPropertyValue(parent, "aliases", strList("a", "b")); ec != types.E_NONE {
		t.Fatalf("restoring write: %v", ec)
	}
	if len(tx.propertyWrites) != 0 {
		t.Fatalf("round trip left a staged write: %v", tx.propertyWrites)
	}
	got, _ := tx.PropertyValue(parent, "aliases")
	if !got.Identical(strList("a", "b")) {
		t.Fatalf("read-your-writes after round trip = %v", got)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	if v1 := liveSlotVersion(t, s, parent, "aliases"); v1 != v0 {
		t.Fatalf("slot version moved %d -> %d on a round-trip write", v0, v1)
	}

	// A round trip that lands on a DIFFERENT value still publishes.
	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()
	if ec := tx2.SetPropertyValue(parent, "aliases", strList("a", "b", "c")); ec != types.E_NONE {
		t.Fatalf("tx2 first write: %v", ec)
	}
	if ec := tx2.SetPropertyValue(parent, "aliases", strList("a", "c")); ec != types.E_NONE {
		t.Fatalf("tx2 second write: %v", ec)
	}
	if len(tx2.propertyWrites) != 1 {
		t.Fatalf("real change was elided")
	}
}
