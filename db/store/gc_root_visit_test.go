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

// A deferred sweep beside a running task reads that task's unpublished writes.
func TestVisitStagedPropertyValuesSeesUnpublishedWrites(t *testing.T) {
	s := oneBodyStore(t)
	if ec := s.DirectTxn().DefineProperty(0, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	written, defined := types.NewWaif(0, 0), types.NewWaif(0, 0)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.SetPropertyValue(0, "p", types.NewList([]types.Value{written})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.DefineProperty(0, "q", NewProperty(defined, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	got := visitedWaifs(tx.VisitStagedPropertyValues)
	if !got.Has(written) || !got.Has(defined) {
		t.Fatalf("staged values visited %v, want both staged WAIFs", got.Values)
	}
}

// A value overwritten after a reader's snapshot stays visible to that reader
// through history, so it must stay a root while the history entry is kept.
func TestVisitHistoryPropertyValuesSeesRetainedVersions(t *testing.T) {
	s := oneBodyStore(t)
	old := types.NewWaif(0, 0)
	if ec := s.DirectTxn().DefineProperty(0, "p", NewProperty(old, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	reader := s.BeginSnapshot(0)
	defer reader.Release()
	if ec := s.DirectTxn().SetPropertyValue(0, "p", types.NewInt(0)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if got, _ := reader.PropertyValue(0, "p"); !got.Equal(old) {
		t.Fatalf("reader sees %v, want the overwritten WAIF", got)
	}
	if got := visitedWaifs(s.VisitHistoryPropertyValues); !got.Has(old) {
		t.Fatalf("history values visited %v, want the overwritten WAIF", got.Values)
	}
}
