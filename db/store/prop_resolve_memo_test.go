package store

import (
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// resetPropResolveMemoForTest empties the store-level property memo so a test
// can observe a first-lookup walk.
func (s *Store) resetPropResolveMemoForTest() {
	s.props.memo.Store(&sync.Map{})
	s.props.size.Store(0)
}

func propMemoHas(s *Store, objID types.ObjID, name string) bool {
	_, ok := s.props.entries().Load(propResolveKey{objID: objID, name: propertyNameKey(name)})
	return ok
}

// propMemoStore is the chain #0 <- #1 <- #2 with "p" (value "root") and "x"
// defined on #0, so #1 and #2 hold clear inherited slots for both; "w" defined
// on #2; and #3, outside the chain, defining "w" and "v", which transactions
// write to become writers without touching any walk of "p".
func propMemoStore(t *testing.T) *Store {
	t.Helper()
	s := testChainStore(t)
	if err := s.Add(NewObject(3, 0)); err != nil {
		t.Fatalf("Add #3: %v", err)
	}
	for _, def := range []struct {
		obj   types.ObjID
		name  string
		value types.Value
	}{{0, "p", types.NewStr("root")}, {0, "x", types.NewInt(0)}, {2, "w", types.NewInt(0)}, {3, "w", types.NewInt(0)}, {3, "v", types.NewInt(0)}} {
		if ec := s.DirectTxn().DefineProperty(def.obj, def.name, NewProperty(def.value, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatalf("DefineProperty #%d.%s: %v", def.obj, def.name, ec)
		}
	}
	return s
}

// walkReference resolves objID.name with a real walk on a fresh snapshot and
// returns the answer and the read set the walk left.
func walkReference(t *testing.T, s *Store, objID types.ObjID, name string, prepare func(*StoreTxn)) (Property, string, types.ErrorCode, readSetSnapshot) {
	t.Helper()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if prepare != nil {
		prepare(tx)
	}
	prop, actual, ec, _ := tx.walkProperty(objID, nil, propertyNameKey(name))
	return prop, actual, ec, snapshotReadSet(tx)
}

// sameProp compares two resolutions field by field, values by identity of
// content: the same staged write in two transactions builds two values.
func sameProp(a, b Property) bool {
	av, bv := a, b
	av.value, bv.value = types.None, types.None
	return av == bv && a.value.Identical(b.value)
}

func warmPropMemo(t *testing.T, s *Store, objID types.ObjID, name string) {
	t.Helper()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	tx.findProperty(objID, name)
	if !propMemoHas(s, objID, name) {
		t.Fatalf("walk of #%d.%s did not populate the store memo", objID, name)
	}
}

// writeOff stages a write to #3.name, off every walk of "p".
func writeOff(t *testing.T, tx *StoreTxn, name string, v int64) {
	t.Helper()
	if ec := tx.SetPropertyValue(3, name, types.NewInt(v)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue #3.%s: %v", name, ec)
	}
}

// writeW stages a write to #3.w.
func writeW(t *testing.T, tx *StoreTxn, v int64) {
	t.Helper()
	writeOff(t, tx, "w", v)
}

// A reading transaction resolves through the store memo: the walk's answer,
// read marks on the slot that supplied the metadata and the slot that supplied
// the value, and a transaction-level dependency instead of per-ancestor marks.
func TestPropResolveMemoReaderHitMatchesWalk(t *testing.T) {
	s := propMemoStore(t)
	if ec := s.DirectTxn().SetPropertyInfo(2, "p", nil, &[]PropertyPerms{PropRead}[0]); ec != types.E_NONE {
		t.Fatalf("SetPropertyInfo: %v", ec)
	}
	wantProp, wantName, wantEC, _ := walkReference(t, s, 2, "p", nil)
	warmPropMemo(t, s, 2, "p")

	user := s.BeginSnapshot(0)
	defer user.Release()
	prop, name, ec := user.findProperty(2, "p")
	if !user.usedPropMemo {
		t.Fatal("second transaction did not resolve through the store memo")
	}
	if prop != wantProp || name != wantName || ec != wantEC {
		t.Fatalf("memo hit = (%+v, %q, %v), want the walk's (%+v, %q, %v)", prop, name, ec, wantProp, wantName, wantEC)
	}
	if prop.value.Str() != "root" || prop.perms != PropRead {
		t.Fatalf("memo hit value %v perms %v, want root and #2's perms", prop.value, prop.perms)
	}
	got := snapshotReadSet(user)
	if len(got.propertyReads) != 2 || len(got.propertyShapeScans) != 0 || len(got.propertyScans) != 0 {
		t.Fatalf("reader hit read set = %+v, want reads of #2.p and #0.p only", got)
	}
	for _, id := range []types.ObjID{2, 0} {
		if _, ok := got.propertyReads[propertyReadKey{objID: id, name: "p"}]; !ok {
			t.Fatalf("reader hit did not mark #%d.p: %v", id, got.propertyReads)
		}
	}
	// Repeat lookups are answered by the transaction's own record.
	for i := 0; i < 10; i++ {
		if again, _, _ := user.findProperty(2, "p"); again != prop {
			t.Fatalf("repeat lookup %d = %+v, want %+v", i, again, prop)
		}
	}
	if len(user.propMemoHits) != 1 {
		t.Fatalf("repeat lookups noted %d memo hits, want 1", len(user.propMemoHits))
	}
	requireSameReadSet(t, "repeat lookups", got, snapshotReadSet(user))
}

// A start object with no slot for the name (as in a database whose inherited
// slots are missing) is skipped by the walk; a reader hit marks only the slot
// that answered, and its record checks the start object without marking it.
func TestPropResolveMemoReaderHitPastSlotlessStart(t *testing.T) {
	s := propMemoStore(t)
	bare := NewObject(4, 0)
	bare.parents = []types.ObjID{0}
	if err := s.Add(bare); err != nil {
		t.Fatalf("Add #4: %v", err)
	}
	wantProp, wantName, wantEC, _ := walkReference(t, s, 4, "p", nil)
	if wantEC != types.E_NONE || wantProp.value.Str() != "root" {
		t.Fatalf("walk from #4 = (%v, %v), want root", wantProp.value, wantEC)
	}
	warmPropMemo(t, s, 4, "p")
	user := s.BeginSnapshot(0)
	defer user.Release()
	for i := 0; i < 3; i++ {
		prop, name, ec := user.findProperty(4, "p")
		if !sameProp(prop, wantProp) || name != wantName || ec != wantEC {
			t.Fatalf("lookup %d = (%+v, %q, %v), want (%+v, %q, %v)", i, prop, name, ec, wantProp, wantName, wantEC)
		}
	}
	got := snapshotReadSet(user)
	if !user.usedPropMemo || len(user.propMemoHits) != 1 || len(got.propertyReads) != 1 || len(got.propertyShapeScans) != 0 {
		t.Fatalf("used=%v hits=%v reads=%v shapes=%v, want one hit marking #0.p only", user.usedPropMemo, user.propMemoHits, got.propertyReads, got.propertyShapeScans)
	}
}

// A writing transaction gets the walk's exact read set from a memo hit, also
// when it has privatized objects on the path without changing where the walk
// stops there. When its own write did change that, it walks.
func TestPropResolveMemoWriterHitMatchesWalk(t *testing.T) {
	writes := map[string]func(t *testing.T, tx *StoreTxn){
		"off path": func(t *testing.T, tx *StoreTxn) { writeW(t, tx, 1) },
		"path object privatized": func(t *testing.T, tx *StoreTxn) {
			if ec := tx.SetPropertyValue(2, "w", types.NewInt(1)); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue #2.w: %v", ec)
			}
		},
		"own override on the path": func(t *testing.T, tx *StoreTxn) {
			if ec := tx.SetPropertyValue(1, "p", types.NewStr("own")); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue #1.p: %v", ec)
			}
		},
	}
	for what, write := range writes {
		for _, name := range []string{"p", "nosuch"} {
			t.Run(what+"/"+name, func(t *testing.T) {
				s := propMemoStore(t)
				wantProp, wantName, wantEC, want := walkReference(t, s, 2, name, func(tx *StoreTxn) { write(t, tx) })
				warmPropMemo(t, s, 2, name)

				user := s.BeginSnapshot(0)
				defer user.Release()
				write(t, user)
				user.propWalk.steps = user.propWalk.steps[:0]
				prop, actual, ec := user.findProperty(2, name)
				walked := len(user.propWalk.steps) != 0
				if wantWalk := what == "own override on the path" && name == "p"; walked != wantWalk {
					t.Fatalf("writer walked=%v, want %v", walked, wantWalk)
				}
				if user.usedPropMemo {
					t.Fatal("writer took a clock-validated memo hit")
				}
				if !sameProp(prop, wantProp) || actual != wantName || ec != wantEC {
					t.Fatalf("writer = (%+v, %q, %v), want the walk's (%+v, %q, %v)", prop, actual, ec, wantProp, wantName, wantEC)
				}
				requireSameReadSet(t, "writer", want, snapshotReadSet(user))
			})
		}
	}
}

// Every change that can move where a walk of #2.p (or #2.q) stops makes later
// snapshots walk again and see it, and fails the commit of a transaction that
// resolved through the memo before it exactly when a walking transaction at the
// same snapshot fails. A value write elsewhere fails neither.
func TestPropResolveMemoShapeChangesInvalidate(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		setup    func(t *testing.T, s *Store)
		change   func(t *testing.T, s *Store)
		conflict bool
	}{
		{name: "define", query: "q", change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().DefineProperty(0, "q", NewProperty(types.NewInt(7), 0, PropRead, false, true)); ec != types.E_NONE {
				t.Fatalf("DefineProperty: %v", ec)
			}
		}, conflict: true},
		{name: "delete", query: "p", change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().DeleteDefinedProperty(0, "p"); ec != types.E_NONE {
				t.Fatalf("DeleteDefinedProperty: %v", ec)
			}
		}, conflict: true},
		{name: "chparent", query: "p", change: func(t *testing.T, s *Store) {
			other, ec := s.DirectTxn().CreateObject(nil, 0, false)
			if ec != types.E_NONE {
				t.Fatalf("CreateObject: %v", ec)
			}
			if ec := s.DirectTxn().DefineProperty(other, "p", NewProperty(types.NewStr("other"), 0, PropRead, false, true)); ec != types.E_NONE {
				t.Fatalf("DefineProperty: %v", ec)
			}
			if ec := s.ChangeParents(2, []types.ObjID{other}); ec != types.E_NONE {
				t.Fatalf("ChangeParents: %v", ec)
			}
		}, conflict: true},
		{name: "override", query: "p", change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().SetPropertyValue(1, "p", types.NewStr("mid")); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue: %v", ec)
			}
		}, conflict: true},
		{name: "clear_property", query: "p", setup: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().SetPropertyValue(1, "p", types.NewStr("mid")); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue: %v", ec)
			}
		}, change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().ClearPropertyOverride(1, "p"); ec != types.E_NONE {
				t.Fatalf("ClearPropertyOverride: %v", ec)
			}
		}, conflict: true},
		{name: "recycle", query: "p", change: func(t *testing.T, s *Store) {
			if err := s.Recycle(1); err != nil {
				t.Fatalf("Recycle: %v", err)
			}
		}, conflict: true},
		{name: "unrelated value write", query: "p", change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().SetPropertyValue(0, "x", types.NewInt(5)); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue: %v", ec)
			}
		}, conflict: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := propMemoStore(t)
			if tc.setup != nil {
				tc.setup(t, s)
			}
			warmPropMemo(t, s, 2, tc.query)

			user := s.BeginSnapshot(0)
			defer user.Release()
			userProp, _, userEC := user.findProperty(2, tc.query)
			if !user.usedPropMemo {
				t.Fatal("user did not resolve through the store memo")
			}
			walker := s.BeginSnapshot(0)
			defer walker.Release()
			walker.propMemoDisabled = true
			walkProp, _, walkEC := walker.findProperty(2, tc.query)
			if walker.usedPropMemo || userProp != walkProp || userEC != walkEC {
				t.Fatalf("user (%+v, %v) and walker (%+v, %v) disagree before the change", userProp, userEC, walkProp, walkEC)
			}

			tc.change(t, s)

			wantProp, wantName, wantEC, _ := walkReference(t, s, 2, tc.query, nil)
			later := s.BeginSnapshot(0)
			prop, name, ec := later.findProperty(2, tc.query)
			usedStale := later.usedPropMemo
			later.Release()
			if prop != wantProp || name != wantName || ec != wantEC {
				t.Fatalf("later snapshot = (%+v, %q, %v), want the walk's (%+v, %q, %v)", prop, name, ec, wantProp, wantName, wantEC)
			}
			if tc.conflict && usedStale {
				t.Fatal("later snapshot used a memo entry from before the change")
			}

			// The snapshot keeps its own answer.
			if again, _, againEC := user.findProperty(2, tc.query); again != userProp || againEC != userEC {
				t.Fatalf("user's snapshot answer moved to (%+v, %v)", again, againEC)
			}
			writeOff(t, user, "w", 11)
			writeOff(t, walker, "v", 12)
			userErr, walkErr := user.Commit(), walker.Commit()
			if (userErr != types.E_NONE) != tc.conflict || (walkErr != types.E_NONE) != tc.conflict {
				t.Fatalf("after %s: memo user commit = %v, walker commit = %v, want conflict=%v for both", tc.name, userErr, walkErr, tc.conflict)
			}
		})
	}
}

// A transaction that resolved through the memo while it was only reading and
// then writes is validated on the walk's own marks, not the clock: its first
// write re-walks the hits into exact marks. An unrelated clear flip elsewhere
// then leaves its commit alone, and an override on the walked path still fails
// it.
func TestPropResolveMemoFirstWriteMaterializesMarks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(t *testing.T, s *Store)
		conflict bool
	}{
		{name: "unrelated clear flip", change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().SetPropertyValue(2, "x", types.NewInt(9)); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue #2.x: %v", ec)
			}
		}},
		{name: "override on the walked path", change: func(t *testing.T, s *Store) {
			if ec := s.DirectTxn().SetPropertyValue(1, "p", types.NewStr("mid")); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue #1.p: %v", ec)
			}
		}, conflict: true},
	} {
		for _, first := range []string{"property write", "create"} {
			t.Run(tc.name+"/"+first, func(t *testing.T) {
				s := propMemoStore(t)
				_, _, _, want := walkReference(t, s, 2, "p", nil)
				warmPropMemo(t, s, 2, "p")

				user := s.BeginSnapshot(0)
				defer user.Release()
				user.findProperty(2, "p")
				if !user.usedPropMemo {
					t.Fatal("expected a clock-validated memo hit")
				}
				switch first {
				case "property write":
					writeW(t, user, 1)
				case "create":
					if _, ec := user.CreateObject(nil, 0); ec != types.E_NONE {
						t.Fatalf("CreateObject: %v", ec)
					}
				}
				if user.usedPropMemo || len(user.propMemoHits) != 0 || user.propMemoDisabled {
					t.Fatalf("after the first write: used=%v hits=%v disabled=%v, want the dependency gone and the memo still on", user.usedPropMemo, user.propMemoHits, user.propMemoDisabled)
				}
				got := snapshotReadSet(user)
				for key, version := range want.propertyReads {
					if got.propertyReads[key] != version {
						t.Fatalf("first write did not materialize %v@%d: %v", key, version, got.propertyReads)
					}
				}

				tc.change(t, s)
				if ec := user.Commit(); (ec != types.E_NONE) != tc.conflict {
					t.Fatalf("writer commit after %s = %v, want conflict=%v", tc.name, ec, tc.conflict)
				}
			})
		}
	}
}

// A clear flip anywhere moves the property-shape clock; a value write to an
// existing non-clear slot does not.
func TestPropResolveMemoClockMovesOnClearFlipOnly(t *testing.T) {
	s := propMemoStore(t)
	before := s.props.shapeChangeTS.Load()
	if ec := s.DirectTxn().SetPropertyValue(0, "x", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if got := s.props.shapeChangeTS.Load(); got != before {
		t.Fatalf("value write moved the property-shape clock %d -> %d", before, got)
	}
	if ec := s.DirectTxn().SetPropertyValue(2, "x", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	moved := s.props.shapeChangeTS.Load()
	if moved <= before || moved != s.clock.Load() {
		t.Fatalf("override onto a clear slot: clock %d -> %d (store clock %d)", before, moved, s.clock.Load())
	}
}

// A transaction whose snapshot predates the property-shape clock never takes a
// memo entry, even one produced after the change.
func TestPropResolveMemoSkipsSnapshotOlderThanClock(t *testing.T) {
	s := propMemoStore(t)
	user := s.BeginSnapshot(0)
	defer user.Release()
	if ec := s.DirectTxn().SetPropertyValue(1, "p", types.NewStr("mid")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	warmPropMemo(t, s, 2, "p")
	prop, _, ec := user.findProperty(2, "p")
	if user.usedPropMemo {
		t.Fatal("a snapshot older than the clock used the store memo")
	}
	if ec != types.E_NONE || prop.value.Str() != "root" {
		t.Fatalf("old snapshot resolved (%v, %v), want root", prop.value, ec)
	}
}

// Anonymous objects are never memoized; missing properties are.
func TestPropResolveMemoSkipsAnonymousAndCachesMisses(t *testing.T) {
	s := propMemoStore(t)
	s.resetPropResolveMemoForTest()
	anon, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject anon: %v", ec)
	}
	tx := s.BeginSnapshot(0)
	if _, _, ec := tx.findProperty(anon, "p"); ec != types.E_NONE {
		t.Fatalf("anon findProperty: %v", ec)
	}
	if _, _, ec := tx.findProperty(2, "nosuch"); ec != types.E_PROPNF {
		t.Fatalf("nosuch: %v", ec)
	}
	tx.Release()
	if propMemoHas(s, anon, "p") {
		t.Fatal("anonymous object was memoized")
	}
	if !propMemoHas(s, 2, "nosuch") {
		t.Fatal("negative resolution was not memoized")
	}
	user := s.BeginSnapshot(0)
	defer user.Release()
	if _, _, ec := user.findProperty(2, "NoSuch"); ec != types.E_PROPNF || !user.usedPropMemo {
		t.Fatalf("memoized miss under another spelling: ec=%v used=%v", ec, user.usedPropMemo)
	}
	if got := snapshotReadSet(user); len(got.propertyReads)+len(got.propertyShapeScans)+len(got.propertyScans) != 0 {
		t.Fatalf("reader miss hit recorded marks: %+v", got)
	}
}

// Before a task's first live mutation, every resolution taken from the memo is
// re-walked into the walk's own marks, the memo is off for the rest of the
// transaction, and those marks still catch a concurrent change.
func TestPropResolveMemoLiveMutationMaterializesMarks(t *testing.T) {
	s := propMemoStore(t)
	_, _, _, want := walkReference(t, s, 2, "p", nil)
	warmPropMemo(t, s, 2, "p")

	user := s.BeginSnapshot(0)
	defer user.Release()
	user.findProperty(2, "p")
	if !user.usedPropMemo {
		t.Fatal("expected a mark-less memo hit")
	}
	user.PrepareLiveMutation()
	if user.usedPropMemo || len(user.propMemoHits) != 0 || !user.propMemoDisabled {
		t.Fatalf("PrepareLiveMutation left used=%v hits=%v disabled=%v", user.usedPropMemo, user.propMemoHits, user.propMemoDisabled)
	}
	requireSameReadSet(t, "materialized marks", want, snapshotReadSet(user))
	user.findProperty(2, "p")
	if user.usedPropMemo {
		t.Fatal("resolution after PrepareLiveMutation used the memo")
	}

	// Another task overrides #1.p; the materialized mark on #1.p fails the
	// coarse commit.
	if ec := s.DirectTxn().SetPropertyValue(1, "p", types.NewStr("mid")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	user.MarkLiveMutated()
	writeW(t, user, 3)
	if ec := user.Commit(); ec == types.E_NONE {
		t.Fatal("coarse commit ignored a concurrent override on the materialized walk path")
	}
}

// A memo user that renews at a hot read carries its dependency, and renewal is
// refused once the property-shape clock has moved past its snapshot.
func TestPropResolveMemoRenewalCarriesDependency(t *testing.T) {
	s := propMemoStore(t)
	warmPropMemo(t, s, 2, "p")
	user := s.BeginSnapshot(0)
	user.findProperty(2, "p")
	next := user.renewAtCurrentClock(nil)
	if next == nil {
		t.Fatal("renewal refused without a shape change")
	}
	if !next.usedPropMemo || len(next.propMemoHits) != 1 {
		t.Fatalf("renewed txn used=%v hits=%v", next.usedPropMemo, next.propMemoHits)
	}
	if ec := s.DirectTxn().SetPropertyValue(1, "p", types.NewStr("mid")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if again := next.renewAtCurrentClock(nil); again != nil {
		again.Release()
		t.Fatal("renewal carried a memo dependency past a property-shape change")
	}
	next.Release()
}
