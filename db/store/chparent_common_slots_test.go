package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// chparent keeps the moved object's and its descendants' slots for properties
// of an ancestor in both the old and new ancestry, and gives each of them new
// clear slots for the new ancestors' properties (Toast db_properties.cc:950-976,
// recursing at :997-1046). On the txn path the values are written in the same
// txn as the chparent, as the builtin sees them.
func TestChangeParentsKeepsCommonAncestorSlots(t *testing.T) {
	for _, path := range []string{"direct", "committed", "txn"} {
		t.Run(path, func(t *testing.T) {
			s := NewStore()
			do := func(f func(tx *StoreTxn)) {
				if path == "direct" {
					f(s.DirectTxn())
					return
				}
				tx := s.BeginSnapshot(0)
				f(tx)
				if code := tx.Commit(); code != types.E_NONE {
					t.Fatalf("Commit: %v", code)
				}
			}
			create := func(parents []types.ObjID, owner types.ObjID) (id types.ObjID) {
				do(func(tx *StoreTxn) {
					var code types.ErrorCode
					if id, code = tx.CreateObject(parents, owner); code != types.E_NONE {
						t.Fatalf("CreateObject: %v", code)
					}
				})
				return id
			}
			define := func(obj types.ObjID, name string, perms PropertyPerms) {
				do(func(tx *StoreTxn) {
					if code := tx.DefineProperty(obj, name, NewProperty(types.NewInt(0), 5, perms, false, true)); code != types.E_NONE {
						t.Fatalf("DefineProperty %s: %v", name, code)
					}
				})
			}
			var x, y types.ObjID
			write := func(tx *StoreTxn) {
				for obj, v := range map[types.ObjID]int64{x: 42, y: 7} {
					if code := tx.SetPropertyValue(obj, "q", types.NewInt(v)); code != types.E_NONE {
						t.Fatalf("SetPropertyValue #%d.q: %v", obj, code)
					}
				}
			}

			a := create(nil, 10)
			define(a, "q", PropRead)
			b := create([]types.ObjID{a}, 10)
			d := create([]types.ObjID{a}, 10)
			define(d, "cx", PropRead|PropChown)
			x = create([]types.ObjID{b}, 40)
			y = create([]types.ObjID{x}, 50)

			move := func() {
				if code := s.ChangeParents(x, []types.ObjID{d}); code != types.E_NONE {
					t.Fatalf("ChangeParents: %v", code)
				}
			}
			if path != "txn" {
				do(write)
				move()
			} else {
				do(func(tx *StoreTxn) {
					write(tx)
					move()
					tx.MarkLiveMutated()
					if code := tx.AdoptLiveRelationships(x, b, d); code != types.E_NONE {
						t.Fatalf("AdoptLiveRelationships: %v", code)
					}
					if code := tx.ReseedInheritedProperties(x, []types.ObjID{b}); code != types.E_NONE {
						t.Fatalf("ReseedInheritedProperties: %v", code)
					}
				})
			}

			for obj, want := range map[types.ObjID]int64{x: 42, y: 7} {
				view, code := s.findProperty(obj, "q")
				if code != types.E_NONE {
					t.Fatalf("#%d.q: %v", obj, code)
				}
				if !view.Value.Equal(types.NewInt(want)) {
					t.Errorf("#%d.q = %v, want %d", obj, view.Value, want)
				}
			}
			for obj, owner := range map[types.ObjID]types.ObjID{x: 40, y: 50} {
				view, code := s.findProperty(obj, "cx")
				if code != types.E_NONE {
					t.Fatalf("#%d.cx: %v", obj, code)
				}
				if view.Owner != owner {
					t.Errorf("#%d.cx owner #%d, want #%d", obj, view.Owner, owner)
				}
			}
		})
	}
}
