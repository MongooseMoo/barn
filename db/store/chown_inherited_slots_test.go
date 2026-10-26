package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A property with the c flag gives every object holding it a slot owned by
// that object's owner, the definer included (Toast db_properties.cc:133,
// :974); without c every slot keeps the owner the property was added with.
func TestChownFlagSlotsOwnedByEachObjectOwner(t *testing.T) {
	for _, path := range []string{"txn", "direct"} {
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

			p := create(nil, 10)
			existing := create([]types.ObjID{p}, 20)
			do(func(tx *StoreTxn) {
				for name, perms := range map[string]PropertyPerms{"cx": PropRead | PropChown, "nx": PropRead} {
					if code := tx.DefineProperty(p, name, NewProperty(types.NewInt(0), 5, perms, false, true)); code != types.E_NONE {
						t.Fatalf("DefineProperty %s: %v", name, code)
					}
				}
			})
			created := create([]types.ObjID{p}, 30)
			moved := create(nil, 40)
			if code := s.ChangeParents(moved, []types.ObjID{p}); code != types.E_NONE {
				t.Fatalf("ChangeParents: %v", code)
			}

			for obj, owner := range map[types.ObjID]types.ObjID{p: 10, existing: 20, created: 30, moved: 40} {
				for name, want := range map[string]types.ObjID{"cx": owner, "nx": 5} {
					view, code := s.DirectTxn().FindProperty(obj, name)
					if code != types.E_NONE {
						t.Fatalf("#%d.%s: %v", obj, name, code)
					}
					if view.Owner != want {
						t.Errorf("#%d.%s owner #%d, want #%d", obj, name, view.Owner, want)
					}
				}
			}
		})
	}
}
