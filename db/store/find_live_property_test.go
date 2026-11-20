package store

import (
	"fmt"
	"maps"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// FindLiveProperty and FindLivePropertyRenewing fetch the object once for the
// existence check and the lookup. They must answer, and leave the read set,
// exactly as ObjectExists followed by FindProperty / FindPropertyRenewing.

func findLiveFixture(t *testing.T) *Store {
	t.Helper()
	s := testChainStore(t)
	if err := s.Add(NewObject(3, 0)); err != nil {
		t.Fatalf("Add #3: %v", err)
	}
	if ec := s.ChangeParents(3, []types.ObjID{0}); ec != types.E_NONE {
		t.Fatalf("ChangeParents #3: %v", ec)
	}
	for _, def := range []struct {
		obj  types.ObjID
		name string
	}{{0, "desc"}, {1, "mid"}, {2, "own"}} {
		if ec := s.DirectTxn().DefineProperty(def.obj, def.name, NewProperty(types.NewStr(def.name), 0, PropRead, false, true)); ec != types.E_NONE {
			t.Fatalf("DefineProperty %s: %v", def.name, ec)
		}
	}
	if err := s.Recycle(3); err != nil {
		t.Fatalf("Recycle #3: %v", err)
	}
	return s
}

var findLiveCases = []struct {
	obj  types.ObjID
	name string
}{
	{2, "desc"}, {2, "DESC"}, {2, "mid"}, {2, "own"}, {2, "nosuch"},
	{0, "desc"}, {0, "own"}, {3, "desc"}, {99, "desc"}, {types.ObjNothing, "desc"},
}

type liveLookup struct {
	value types.Value
	name  string
	ec    types.ErrorCode
}

func existsThenFind(tx *StoreTxn, objID types.ObjID, name string) liveLookup {
	if ec := tx.ObjectExists(objID); ec != types.E_NONE {
		return liveLookup{ec: ec}
	}
	view, ec := tx.FindProperty(objID, name)
	return liveLookup{value: view.Value, name: view.Name, ec: ec}
}

func requireSameLookup(t *testing.T, what string, want, got liveLookup) {
	t.Helper()
	if got.ec != want.ec || got.name != want.name || !got.value.Identical(want.value) {
		t.Fatalf("%s = %+v, want %+v", what, got, want)
	}
}

func requireSameScalarReads(t *testing.T, what string, want, got *StoreTxn) {
	t.Helper()
	if !maps.Equal(want.scalarReads, got.scalarReads) {
		t.Fatalf("%s: scalarReads = %v, want %v", what, got.scalarReads, want.scalarReads)
	}
}

// warmFindLive resolves objID.name once, so the store-level property memo
// answers both sides of a comparison alike.
func warmFindLive(s *Store, objID types.ObjID, name string) {
	warm := s.BeginSnapshot(0)
	existsThenFind(warm, objID, name)
	warm.Release()
}

func TestFindLivePropertyMatchesObjectExistsThenFindProperty(t *testing.T) {
	s := findLiveFixture(t)
	for _, tc := range findLiveCases {
		warmFindLive(s, tc.obj, tc.name)
		ref := s.BeginSnapshot(0)
		got := s.BeginSnapshot(0)
		// The second round hits the transaction's resolution memo.
		for round := range 2 {
			want := existsThenFind(ref, tc.obj, tc.name)
			view, ec := got.FindLiveProperty(tc.obj, tc.name)
			what := "FindLiveProperty(#" + fmt.Sprint(int64(tc.obj)) + "." + tc.name + ")"
			if round == 1 {
				what += " again"
			}
			requireSameLookup(t, what, want, liveLookup{value: view.Value, name: view.Name, ec: ec})
			requireSameReadSet(t, what, snapshotReadSet(ref), snapshotReadSet(got))
			requireSameScalarReads(t, what, ref, got)
		}
		ref.Release()
		got.Release()
	}
}

func TestFindLivePropertyOnTheDirectTransaction(t *testing.T) {
	s := findLiveFixture(t)
	tx := s.DirectTxn()
	for _, tc := range findLiveCases {
		want := existsThenFind(tx, tc.obj, tc.name)
		view, ec := tx.FindLiveProperty(tc.obj, tc.name)
		requireSameLookup(t, "direct FindLiveProperty(#"+fmt.Sprint(int64(tc.obj))+"."+tc.name+")", want, liveLookup{value: view.Value, name: view.Name, ec: ec})
	}
}

func TestFindLivePropertyRenewingMatchesObjectExistsThenRenewing(t *testing.T) {
	for _, tc := range findLiveCases {
		s := findLiveFixture(t)
		warmFindLive(s, tc.obj, tc.name)
		ref := s.BeginSnapshot(0)
		got := s.BeginSnapshot(0)
		var want liveLookup
		if ec := ref.ObjectExists(tc.obj); ec != types.E_NONE {
			want = liveLookup{ec: ec}
		} else {
			view, ec, next := ref.FindPropertyRenewing(tc.obj, tc.name)
			ref = next
			want = liveLookup{value: view.Value, name: view.Name, ec: ec}
		}
		view, ec, next := got.FindLivePropertyRenewing(tc.obj, tc.name)
		got = next
		what := "FindLivePropertyRenewing(#" + fmt.Sprint(int64(tc.obj)) + "." + tc.name + ")"
		requireSameLookup(t, what, want, liveLookup{value: view.Value, name: view.Name, ec: ec})
		requireSameReadSet(t, what, snapshotReadSet(ref), snapshotReadSet(got))
		requireSameScalarReads(t, what, ref, got)
		ref.Release()
		got.Release()
	}
}

// A stale read of a hot property renews the transaction through either path,
// and the renewed transaction still carries the existence read.
func TestFindLivePropertyRenewingRenewsAStaleHotRead(t *testing.T) {
	s, counter, _ := hotReadFixture(t)
	tx := s.BeginSnapshot(0)
	commitValue(t, s, counter, "handle", types.NewInt(7))

	view, ec, next := tx.FindLivePropertyRenewing(counter, "handle")
	if ec != types.E_NONE {
		t.Fatalf("FindLivePropertyRenewing: %v", ec)
	}
	defer next.Release()
	if next == tx {
		t.Fatalf("stale hot read did not renew the transaction")
	}
	if got := view.Value.Int(); got != 7 {
		t.Fatalf("hot read after renewal = %d, want the committed 7", got)
	}
	if _, ok := next.scalarReads[counter]; !ok {
		t.Fatalf("renewed transaction lost the existence read of #%d", counter)
	}
}
