package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func visitedWaifs(visit func(func(types.Value))) *types.WaifSet {
	set := types.NewWaifSet(nil)
	visit(func(value types.Value) { set.AddReachable(value, true) })
	return set
}

func definePropertyP(t *testing.T, s *Store, id types.ObjID, value types.Value) {
	t.Helper()
	if ec := s.DirectTxn().DefineProperty(id, "p", NewProperty(value, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
}

// A deferred sweep beside a running task reads that task's unpublished writes.
func TestVisitPrivatePropertyValuesSeesUnpublishedWrites(t *testing.T) {
	s := oneBodyStore(t)
	definePropertyP(t, s, 0, types.NewInt(0))
	written, defined := types.NewWaif(0, 0), types.NewWaif(0, 0)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.SetPropertyValue(0, "p", types.NewList([]types.Value{written})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.DefineProperty(0, "q", NewProperty(defined, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	got := visitedWaifs(tx.VisitPrivatePropertyValues)
	if !got.Has(written) || !got.Has(defined) {
		t.Fatalf("private values visited %v, want both staged WAIFs", got.Values)
	}
}

// An anonymous object is mutated in place, so a value a transaction read
// before the mutation survives only in that transaction's copy.
func TestVisitPrivatePropertyValuesSeesReadCopyOfAnonymous(t *testing.T) {
	s := oneBodyStore(t)
	definePropertyP(t, s, 0, types.NewInt(0))
	anon, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	old := types.NewWaif(0, 0)
	if ec := s.DirectTxn().SetPropertyValue(anon, "p", old); ec != types.E_NONE {
		t.Fatal(ec)
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if got, _ := tx.PropertyValue(anon, "p"); !got.Equal(old) {
		t.Fatalf("reader sees %v, want the WAIF", got)
	}
	if ec := s.DirectTxn().SetPropertyValue(anon, "p", types.NewInt(0)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if got := visitedWaifs(tx.VisitPrivatePropertyValues); !got.Has(old) {
		t.Fatalf("private values visited %v, want the WAIF in the read copy", got.Values)
	}
}

// A snapshot is served the version current at its readTS; a later snapshot is
// served the live image, so the superseded version is not its root.
func TestVisitSnapshotHistoryValuesSeesOnlyServedVersions(t *testing.T) {
	s := oneBodyStore(t)
	old := types.NewWaif(0, 0)
	definePropertyP(t, s, 0, old)
	before := s.BeginSnapshot(0)
	defer before.Release()
	if ec := s.DirectTxn().SetPropertyValue(0, "p", types.NewInt(0)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	after := s.BeginSnapshot(0)
	defer after.Release()
	if got, _ := before.PropertyValue(0, "p"); !got.Equal(old) {
		t.Fatalf("earlier reader sees %v, want the overwritten WAIF", got)
	}
	if got := visitedWaifs(before.VisitSnapshotHistoryValues); !got.Has(old) {
		t.Fatalf("earlier snapshot's history values %v, want the overwritten WAIF", got.Values)
	}
	if got := visitedWaifs(after.VisitSnapshotHistoryValues); got.Has(old) {
		t.Fatalf("later snapshot's history values %v include a version it is never served", got.Values)
	}
}
