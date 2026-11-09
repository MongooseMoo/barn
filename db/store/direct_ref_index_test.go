package store

import (
	"maps"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// fullScanAnonymousReachability is the walk persistentAnonymousReachability made
// before the direct-reference index: it seeds from every property of every live
// non-anonymous directory object. It is the oracle the index-seeded walk must equal.
func fullScanAnonymousReachability(s *Store, retained bool) map[types.ObjID]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reachable := make(map[types.ObjID]struct{})
	var queue []types.ObjID
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if !validLiveObject(obj) || obj.anonymous {
			return true
		}
		for _, prop := range obj.properties.all() {
			refs := make(map[types.ObjID]struct{})
			types.CollectAnonymousRefs(prop.value, refs, retained)
			for id := range refs {
				queue = append(queue, id)
			}
		}
		return true
	})
	s.expandAnonymousReachabilityLocked(reachable, queue, retained)
	return reachable
}

// fullScanWaifTop is the top-level scan persistentWaifRootSet made before the
// index: every WAIF held directly by any live directory object's property values.
func fullScanWaifTop(s *Store) []types.Value {
	s.mu.RLock()
	defer s.mu.RUnlock()
	set := types.NewWaifSet(nil)
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if !validLiveObject(obj) {
			return true
		}
		for _, prop := range obj.properties.all() {
			if prop.value.MayHoldFinalizable() {
				prop.value.VisitDirectFinalizationRefs(func(ref types.Value) { set.Add(ref) })
			}
		}
		return true
	})
	return set.Values
}

func sameWaif(a, b types.Value) bool { return a.WaifIdentity() == b.WaifIdentity() }

// indexCoverage records whether the drive ever gave the walks something to find,
// so a vacuous run cannot pass.
type indexCoverage struct{ anons, waifs bool }

func (c *indexCoverage) require(t *testing.T, s *Store, step string) {
	t.Helper()
	for _, retained := range []bool{true, false} {
		got, want := s.persistentAnonymousReachability(retained), fullScanAnonymousReachability(s, retained)
		if !maps.Equal(got, want) {
			t.Fatalf("%s: anonymous reachability (retained=%v) = %v, full scan %v", step, retained, got, want)
		}
		c.anons = c.anons || len(want) > 0
	}
	top, wantTop := s.indexedWaifTop(), fullScanWaifTop(s)
	if !slices.EqualFunc(top, wantTop, sameWaif) {
		t.Fatalf("%s: indexed WAIF top = %v, full scan %v", step, top, wantTop)
	}
	c.waifs = c.waifs || len(wantTop) > 0
	for _, retained := range []bool{true, false} {
		want := types.NewWaifSet(nil)
		for _, waif := range wantTop {
			want.AddReachable(waif, retained)
		}
		if got := s.persistentWaifRootSet(retained).Values; !slices.EqualFunc(got, want.Values, sameWaif) {
			t.Fatalf("%s: WAIF root set (retained=%v) = %v, full scan %v", step, retained, got, want.Values)
		}
	}
}

func requireNoError(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", step, err)
	}
}

func requireNone(t *testing.T, step string, ec types.ErrorCode) {
	t.Helper()
	if ec != types.E_NONE {
		t.Fatalf("%s: %v", step, ec)
	}
}

// The index-seeded walks equal the full scans after every kind of store write
// that calls noteWaifRootsChanged: commits (value set, override clear, define,
// definition delete, rename, recycle), direct recycle, recreate, chparent of
// numbered and anonymous objects, renumber, load (Add, AddAnonymous), anonymous
// creation, and in-place WAIF property changes.
func TestDirectRefIndexMatchesFullScan(t *testing.T) {
	s, ids := immutFixture(t, 2)
	a, b := ids[0], ids[1]
	d := s.DirectTxn()
	var cov indexCoverage
	step := func(name string) { t.Helper(); cov.require(t, s, name) }

	c, ec := d.CreateObject([]types.ObjID{a}, 0)
	requireNone(t, "create child", ec)
	slot := func(v types.Value) Property { return NewProperty(v, 0, PropRead|PropWrite, false, true) }
	requireNone(t, "define a.p", d.DefineProperty(a, "p", slot(types.NewInt(0))))
	requireNone(t, "define b.p", d.DefineProperty(b, "p", slot(types.NewInt(0))))
	step("define")

	anon := func() types.ObjID {
		t.Helper()
		id, ec := d.CreateObject([]types.ObjID{a}, 0, true)
		requireNone(t, "create anonymous", ec)
		return id
	}
	x1, x2, x3 := anon(), anon(), anon()
	step("create anonymous")

	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(a, "p", types.NewAnon(x1)) })
	step("numbered holds anonymous")
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(x1, "p", types.NewAnon(x2)) })
	step("anonymous holds anonymous")

	w := types.NewWaif(a, 0)
	w.SetProperty("child", types.NewAnon(x3))
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetPropertyValue(b, "p", types.NewList([]types.Value{types.NewInt(1), w}))
	})
	step("waif in a list")
	w.SetProperty("child", types.NewAnon(x2))
	step("waif property in place")

	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(c, "p", types.NewAnon(x3)) })
	step("override on child")
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.ClearPropertyOverride(c, "p") })
	step("clear override")

	requireNone(t, "define with waif value", d.DefineProperty(a, "d", slot(types.NewWaif(a, 0))))
	step("direct define")
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.DeleteDefinedProperty(a, "d") })
	step("committed definition delete")
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.DefineProperty(a, "e", slot(types.NewAnon(x3))) })
	step("committed define")
	requireNone(t, "direct definition delete", d.DeleteDefinedProperty(a, "e"))
	step("direct definition delete")
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.RenameProperty(a, "p", "q") })
	step("rename")

	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(c, "q", types.NewAnon(x3)) })
	requireNone(t, "chparent numbered", s.ChangeParents(c, []types.ObjID{b}))
	step("chparent drops an override")
	requireNone(t, "chparent anonymous", s.ChangeParents(x1, []types.ObjID{b}))
	step("chparent anonymous")

	spare, ec := d.CreateObject(nil, 0)
	requireNone(t, "create spare", ec)
	requireNoError(t, "direct recycle", s.Recycle(spare))
	step("direct recycle")
	requireNoError(t, "renumber", s.Renumber(a, spare))
	step("renumber")
	requireNoError(t, "recreate", s.Recreate(a, types.ObjNothing, 0))
	step("recreate")
	requireNoError(t, "direct recycle of a holder", s.Recycle(spare))
	step("direct recycle of a holder")
	requireNoError(t, "recycle anonymous", s.Recycle(x2))
	step("recycle anonymous")
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { _, ec := tx.RecycleObject(b); return ec })
	step("committed recycle")

	x4 := anon()
	loaded := func(id types.ObjID, anonymous bool, v types.Value) *Object {
		builder := NewObjectBuilder(id)
		builder.SetAnonymous(anonymous)
		builder.SetProperty("held", slot(v))
		return builder.Build()
	}
	requireNoError(t, "load numbered", s.Add(loaded(s.NextID(), false, types.NewList([]types.Value{types.NewAnon(x4), types.NewWaif(spare, 0)}))))
	step("load numbered")
	requireNoError(t, "load anonymous in directory", s.Add(loaded(s.NextID(), true, types.NewWaif(spare, 0))))
	step("load anonymous in directory")
	s.AddAnonymous(loaded(s.NextID(), true, types.NewWaif(spare, 0)))
	step("load anonymous")

	if !cov.anons || !cov.waifs {
		t.Fatalf("vacuous drive: anonymous refs seen %v, WAIF refs seen %v", cov.anons, cov.waifs)
	}
}
