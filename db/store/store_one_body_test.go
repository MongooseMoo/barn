package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Each store operation has one implementation, so the direct transaction and a
// snapshot transaction give the same answer. These cases are where the two
// used to disagree.

func oneBodyStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	if err := s.Add(NewObject(0, types.ObjNothing)); err != nil {
		t.Fatal(err)
	}
	return s
}

// A recycled anonymous object is not a recycled id: recycled_objects() lists
// numbered ids only (Toast bf_recycled_objects, objects.cc:1258-1276).
func TestIsRecycledExcludesRecycledAnonymous(t *testing.T) {
	s := oneBodyStore(t)
	anon, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if ec != types.E_NONE {
		t.Fatalf("create anonymous: %s", ec)
	}
	if err := s.Recycle(anon); err != nil {
		t.Fatalf("recycle anonymous: %v", err)
	}
	snap := s.BeginSnapshot(0)
	defer snap.Release()
	if s.DirectTxn().IsRecycled(anon) {
		t.Error("direct IsRecycled(recycled anonymous) = true, want false")
	}
	if snap.IsRecycled(anon) {
		t.Error("snapshot IsRecycled(recycled anonymous) = true, want false")
	}
}

// A move into a destination that is no longer valid fails and publishes
// nothing, whichever transaction stages it.
func TestMoveObjectIntoRecycledDestination(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	what1, _ := d.CreateObject([]types.ObjID{0}, 0)
	what2, _ := d.CreateObject([]types.ObjID{0}, 0)
	where, _ := d.CreateObject([]types.ObjID{0}, 0)
	if err := s.Recycle(where); err != nil {
		t.Fatal(err)
	}

	if ec := d.MoveObject(what1, where, 0); ec != types.E_INVARG {
		t.Errorf("direct MoveObject = %s, want E_INVARG", ec)
	}
	snap := s.BeginSnapshot(0)
	if ec := snap.MoveObject(what2, where, 0); ec != types.E_INVARG {
		t.Errorf("snapshot MoveObject = %s, want E_INVARG", ec)
	}
	if ec := snap.Commit(); ec != types.E_NONE {
		t.Errorf("snapshot Commit = %s", ec)
	}
	snap.Release()
	for _, id := range []types.ObjID{what1, what2} {
		if loc, _ := d.Location(id); loc != types.ObjNothing {
			t.Errorf("location of #%d = #%d, want #-1", id, loc)
		}
	}
}

// Verb lookups skip an object flagged invalid, as every other read does.
func TestVerbLookupsSkipInvalidObject(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	parent, _ := d.CreateObject([]types.ObjID{0}, 0)
	child, _ := d.CreateObject([]types.ObjID{parent}, 0)
	if _, ec := s.AddVerb(parent, NewVerb([]string{"greet"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 1;"})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := d.SetObjectFlag(parent, FlagInvalid, true); ec != types.E_NONE {
		t.Fatal(ec)
	}
	snap := s.BeginSnapshot(0)
	defer snap.Release()
	for name, tx := range map[string]*StoreTxn{"direct": d, "snapshot": snap} {
		if _, definer, err := tx.FindVerb(child, "greet"); err == nil {
			t.Errorf("%s FindVerb through invalid parent found it on #%d", name, definer)
		}
		if _, err := tx.FindVerbOnObject(parent, "greet"); err == nil {
			t.Errorf("%s FindVerbOnObject on invalid object found the verb", name)
		}
	}
}

// Setting a verb's code on an invalid object is E_INVIND, before any verb
// lookup, as SetVerbCodeByIndex already is.
func TestSetVerbCodeOnRecycledObject(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	obj, _ := d.CreateObject([]types.ObjID{0}, 0)
	if _, ec := s.AddVerb(obj, NewVerb([]string{"v"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if err := s.Recycle(obj); err != nil {
		t.Fatal(err)
	}
	if ec := d.SetVerbCode(obj, "v", []string{"return 2;"}); ec != types.E_INVIND {
		t.Errorf("direct SetVerbCode = %s, want E_INVIND", ec)
	}
	snap := s.BeginSnapshot(0)
	defer snap.Release()
	if ec := snap.SetVerbCode(obj, "v", []string{"return 2;"}); ec != types.E_INVIND {
		t.Errorf("snapshot SetVerbCode = %s, want E_INVIND", ec)
	}
}

// A direct write that fails publishes nothing: the clock, the object's history
// and the WAIF-roots epoch stay where they were.
func TestFailedDirectWriteLeavesNoTrace(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	obj, _ := d.CreateObject([]types.ObjID{0}, 0)
	clock := s.readTimestamp()
	epoch := s.waifRoots.epoch.Load()
	if ec := d.SetPropertyValue(obj, "missing", types.NewInt(1)); ec != types.E_PROPNF {
		t.Fatalf("SetPropertyValue = %s, want E_PROPNF", ec)
	}
	if got := s.readTimestamp(); got != clock {
		t.Errorf("clock moved %d -> %d", clock, got)
	}
	s.history.mu.Lock()
	entries := len(s.history.byID[obj])
	s.history.mu.Unlock()
	if entries != 0 {
		t.Errorf("history entries for #%d = %d, want 0", obj, entries)
	}
	if got := s.waifRoots.epoch.Load(); got != epoch {
		t.Errorf("WAIF-roots epoch moved %d -> %d", epoch, got)
	}
}
