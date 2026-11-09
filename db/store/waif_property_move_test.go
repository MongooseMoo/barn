package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A WAIF's values are keyed by its class's ":" property names, so a rename of
// the class property moves the value and a delete drops it (Toast
// rename_waif_prop_recursively, db_properties.cc:233-249; update_waif_propdefs,
// waif.cc:412-575), on the class and every descendant class.

type waifMoveFixture struct {
	s             *Store
	class, sub    types.ObjID
	w, v, unwrote types.Value // w and unwrote of class; v of sub
}

func newWaifMoveFixture(t *testing.T) waifMoveFixture {
	t.Helper()
	s := oneBodyStore(t)
	d := s.DirectTxn()
	f := waifMoveFixture{s: s}
	f.class = mustCreate(t, d, 0, false)
	mustDefine(t, d, f.class, ":a", 1)
	mustDefine(t, d, f.class, ":b", 2)
	f.sub = mustCreate(t, d, f.class, false)
	f.w, f.v, f.unwrote = types.NewWaif(f.class, 0), types.NewWaif(f.sub, 0), types.NewWaif(f.class, 0)
	s.RegisterWaif(f.class, f.w)
	s.RegisterWaif(f.sub, f.v)
	s.RegisterWaif(f.class, f.unwrote)
	mustSetWaif(t, d, f.w, "a", 10)
	mustSetWaif(t, d, f.w, "b", 20)
	mustSetWaif(t, d, f.v, "a", 30)
	return f
}

func mustSetWaif(t *testing.T, tx *StoreTxn, w types.Value, name string, value int64) {
	t.Helper()
	if ec := tx.SetWaifProperty(w, name, types.NewInt(value)); ec != types.E_NONE {
		t.Fatalf("set waif .%s: %s", name, ec)
	}
}

// wantWaif checks w's own values: want maps each name to its value, or to -1
// for a name w must not hold.
func wantWaif(t *testing.T, tx *StoreTxn, label string, w types.Value, want map[string]int64) {
	t.Helper()
	for name, value := range want {
		got, held, ec := tx.WaifProperty(w, name)
		switch {
		case ec != types.E_NONE:
			t.Errorf("%s.%s: %s", label, name, ec)
		case value < 0 && held:
			t.Errorf("%s.%s = %v, want absent", label, name, got)
		case value >= 0 && (!held || !got.Identical(types.NewInt(value))):
			t.Errorf("%s.%s = %v (held %v), want %d", label, name, got, held, value)
		}
	}
}

func TestWaifValuesFollowClassPropertyRename(t *testing.T) {
	for _, route := range renameRoutes {
		t.Run(route.name, func(t *testing.T) {
			f := newWaifMoveFixture(t)
			route.run(t, f.s, func(tx *StoreTxn) types.ErrorCode {
				for _, step := range [][2]string{{":a", ":t"}, {":b", ":a"}, {":t", ":b"}} {
					if ec := tx.RenameProperty(f.class, step[0], step[1]); ec != types.E_NONE {
						return ec
					}
				}
				wantWaif(t, tx, "w in txn", f.w, map[string]int64{"a": 20, "b": 10, "t": -1})
				return types.E_NONE
			})
			tx := f.s.BeginSnapshot(0)
			defer tx.Release()
			wantWaif(t, tx, "w", f.w, map[string]int64{"a": 20, "b": 10, "t": -1})
			wantWaif(t, tx, "v", f.v, map[string]int64{"a": -1, "b": 30})
			wantWaif(t, tx, "unwrote", f.unwrote, map[string]int64{"a": -1, "b": -1})
		})
	}
}

func TestWaifValuesGoWithDeletedClassProperty(t *testing.T) {
	for _, route := range renameRoutes {
		t.Run(route.name, func(t *testing.T) {
			f := newWaifMoveFixture(t)
			route.run(t, f.s, func(tx *StoreTxn) types.ErrorCode {
				if ec := tx.DeleteDefinedProperty(f.class, ":a"); ec != types.E_NONE {
					return ec
				}
				wantWaif(t, tx, "w in txn", f.w, map[string]int64{"a": -1, "b": 20})
				return tx.DefineProperty(f.class, ":a", NewProperty(types.NewInt(7), 0, PropRead|PropWrite, false, true))
			})
			tx := f.s.BeginSnapshot(0)
			defer tx.Release()
			wantWaif(t, tx, "w", f.w, map[string]int64{"a": -1, "b": 20})
			wantWaif(t, tx, "v", f.v, map[string]int64{"a": -1})
		})
	}
}

// A WAIF of the class published after the renaming txn walked the class is
// moved by that txn's commit.
func TestWaifRenameMovesWaifPublishedAfterWalk(t *testing.T) {
	f := newWaifMoveFixture(t)
	tx := f.s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.RenameProperty(f.class, ":a", ":c"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	late := types.NewWaif(f.sub, 0)
	f.s.RegisterWaif(f.sub, late)
	mustSetWaif(t, f.s.DirectTxn(), late, "a", 40)
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %s", ec)
	}
	after := f.s.BeginSnapshot(0)
	defer after.Release()
	wantWaif(t, after, "late", late, map[string]int64{"a": -1, "c": 40})
	wantWaif(t, after, "w", f.w, map[string]int64{"a": -1, "c": 10})
}

// A task that found the class property and staged a WAIF value under its name
// is stale once the property is renamed.
func TestWaifWriterConflictsWithRename(t *testing.T) {
	f := newWaifMoveFixture(t)
	writer := f.s.BeginSnapshot(0)
	defer writer.Release()
	if _, ec := writer.FindProperty(f.sub, ":b"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	mustSetWaif(t, writer, f.v, "b", 50)
	if ec := f.s.DirectTxn().RenameProperty(f.class, ":b", ":c"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := writer.Commit(); ec != types.E_INVARG || !writer.ValidationFailed() {
		t.Fatalf("writer Commit = %s, validation failed %v; want E_INVARG, true", ec, writer.ValidationFailed())
	}
	after := f.s.BeginSnapshot(0)
	defer after.Release()
	wantWaif(t, after, "v", f.v, map[string]int64{"a": 30, "b": -1, "c": -1})
}
