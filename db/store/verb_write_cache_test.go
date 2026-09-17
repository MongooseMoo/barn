package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestVerbMemoAfterUnrelatedStagedWrite(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if err := s.Add(NewObject(3, 0)); err != nil {
		t.Fatal(err)
	}
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	if ec := tx.SetObjectName(3, "changed"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	want := referenceVerbReadSet(t, s, 2, "look")
	for range 2 {
		_, definer, err := tx.findVerb(2, "look", false)
		if err != nil || definer != 0 {
			t.Fatalf("lookup = #%d, %v", definer, err)
		}
		requireSameReadSet(t, "untouched ancestry", want, snapshotReadSet(tx))
	}
	if n, _ := tx.resolveCacheLenForTest(); n != 1 {
		t.Fatalf("verb cache size = %d, want 1", n)
	}
	// Privatizing an ancestor invalidates the entry. Subsequent writes to that
	// already-owned ancestor must never leave an entry eligible for reuse.
	if ec := tx.SetObjectName(1, "owned ancestor"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if _, _, err := tx.findVerb(2, "look", false); err != nil {
		t.Fatal(err)
	}
	if n, _ := tx.resolveCacheLenForTest(); n != 0 {
		t.Fatalf("owned path was cached: %d", n)
	}
}

func BenchmarkVerbLookupAfterUnrelatedWrite(b *testing.B) {
	s := NewStore()
	for id := types.ObjID(0); id < 4; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			b.Fatal(err)
		}
		if id > 0 && id < 3 {
			s.ChangeParents(id, []types.ObjID{id - 1})
		}
	}
	v := NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)
	s.AddVerb(0, v)
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	tx.SetObjectName(3, "changed")
	for b.Loop() {
		if _, _, err := tx.findVerb(2, "look", false); err != nil {
			b.Fatal(err)
		}
	}
}
