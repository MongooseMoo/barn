package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestChangeParentsReseedMatchesToastRule moves objects around a hierarchy with
// multiple inheritance and an anonymous child, and checks every slot of the
// moved object and its descendants against Toast's rule
// (db_properties.cc:950-976): a property whose definer is in both the old and
// new ancestry keeps the slot the object had; any other gets a clear slot with
// the definer's perms, owned by the object's owner under the c flag and by the
// definer's owner otherwise. On the txn path the values are written in the
// same txn as the move.
func TestChangeParentsReseedMatchesToastRule(t *testing.T) {
	for _, path := range []string{"direct", "txn"} {
		t.Run(path, func(t *testing.T) {
			s := NewStore()
			direct := s.DirectTxn()
			create := func(parents []types.ObjID, owner types.ObjID, anonymous ...bool) types.ObjID {
				id, code := direct.CreateObject(parents, owner, anonymous...)
				if code != types.E_NONE {
					t.Fatalf("CreateObject: %v", code)
				}
				return id
			}
			define := func(obj types.ObjID, name string, perms PropertyPerms) {
				if code := direct.DefineProperty(obj, name, NewProperty(types.NewInt(0), 5, perms, false, true)); code != types.E_NONE {
					t.Fatalf("DefineProperty %s: %v", name, code)
				}
			}
			ids := func(ids ...types.ObjID) []types.ObjID { return ids }

			a := create(nil, 10)
			define(a, "q", PropRead)
			define(a, "cq", PropRead|PropChown)
			b := create(ids(a), 10)
			m := create(ids(a), 10)
			d := create(nil, 11)
			define(d, "r", PropRead)
			define(d, "cr", PropRead|PropChown)
			n := create(nil, 12)
			define(n, "s", PropRead)
			x := create(ids(b), 40)
			y := create(ids(x, m), 50)
			z := create(ids(y), 60)
			w := create(ids(x, n), 70)
			u := create(ids(x), 80, true)

			type slots map[string]Property
			view := func(read func(types.ObjID) *Object, id types.ObjID) slots {
				out := make(slots)
				for name, prop := range read(id).properties.all() {
					out[name] = prop
				}
				return out
			}
			live := func(id types.ObjID) *Object {
				s.mu.RLock()
				defer s.mu.RUnlock()
				return s.liveObjectLocked(id)
			}
			ancestry := func(id types.ObjID) map[types.ObjID]bool {
				list, code := s.DirectTxn().Ancestors(id, false)
				if code != types.E_NONE {
					t.Fatalf("ancestors #%d: %v", id, code)
				}
				set := make(map[types.ObjID]bool)
				for _, ancestorID := range list {
					set[ancestorID] = true
				}
				return set
			}

			value := int64(100)
			moves := []struct {
				obj     types.ObjID
				parents []types.ObjID
			}{
				{x, ids(d)}, {x, ids(b)}, {x, ids(m)}, {x, ids(d, b)}, {y, ids(x)}, {x, ids(n)}, {x, ids(b)},
			}
			for step, move := range moves {
				subtree, _ := inheritanceSubtree(move.obj, live)
				oldParents := append([]types.ObjID(nil), live(move.obj).parents...)
				tx := s.BeginSnapshot(0)
				read := live
				if path == "txn" {
					read = tx.object
				}
				// Write every inherited slot of the subtree, so kept and dropped
				// values both show.
				for _, id := range subtree {
					for name, prop := range view(live, id) {
						if prop.defined {
							continue
						}
						value++
						target := direct
						if path == "txn" {
							target = tx
						}
						if code := target.SetPropertyValue(id, name, types.NewInt(value)); code != types.E_NONE {
							t.Fatalf("step %d: SetPropertyValue #%d.%s: %v", step, id, name, code)
						}
					}
				}
				before := make(map[types.ObjID]slots)
				oldAncestry := make(map[types.ObjID]map[types.ObjID]bool)
				for _, id := range subtree {
					before[id] = view(read, id)
					oldAncestry[id] = ancestry(id)
				}

				if code := s.ChangeParents(move.obj, move.parents); code != types.E_NONE {
					t.Fatalf("step %d: ChangeParents: %v", step, code)
				}
				if path == "txn" {
					tx.MarkLiveMutated()
					if code := tx.AdoptLiveRelationships(append(append(ids(move.obj), oldParents...), move.parents...)...); code != types.E_NONE {
						t.Fatalf("step %d: AdoptLiveRelationships: %v", step, code)
					}
					if code := tx.ReseedInheritedProperties(move.obj, oldParents); code != types.E_NONE {
						t.Fatalf("step %d: ReseedInheritedProperties: %v", step, code)
					}
				}
				if code := tx.Commit(); code != types.E_NONE {
					t.Fatalf("step %d: Commit: %v", step, code)
				}

				for _, id := range subtree {
					obj := live(id)
					got := view(live, id)
					want := make(slots)
					for name, prop := range before[id] {
						if prop.defined {
							want[name] = prop
						}
					}
					for ancestorID := range ancestry(id) {
						for name, def := range view(live, ancestorID) {
							if !def.defined {
								continue
							}
							if oldAncestry[id][ancestorID] {
								want[name] = before[id][name]
								continue
							}
							owner := def.owner
							if def.perms.Has(PropChown) {
								owner = obj.owner
							}
							want[name] = Property{value: types.None, owner: owner, perms: def.perms, clear: true}
						}
					}
					if len(got) != len(want) {
						t.Errorf("step %d: #%d has %d slots, want %d: got %v want %v", step, id, len(got), len(want), got, want)
					}
					for name, w := range want {
						g, ok := got[name]
						if !ok {
							t.Errorf("step %d: #%d.%s missing", step, id, name)
							continue
						}
						if !g.value.Equal(w.value) || g.owner != w.owner || g.perms != w.perms || g.clear != w.clear || g.defined != w.defined {
							t.Errorf("step %d: #%d.%s = {%v #%d %v clear=%v}, want {%v #%d %v clear=%v}", step, id, name,
								g.value, g.owner, g.perms, g.clear, w.value, w.owner, w.perms, w.clear)
						}
					}
				}
			}
			_ = z
			_ = w
			_ = u
		})
	}
}
