package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestCreateInheritedBasePreservesSlotMetadata(t *testing.T) {
	for _, anonymous := range []bool{false, true} {
		for _, change := range []string{"none", "owner", "perms", "chown", "added"} {
			t.Run(fmt.Sprintf("anon=%t/%s", anonymous, change), func(t *testing.T) {
				s := NewStore()
				parent, sibling := loadedClass(t, s, 3)
				tx := s.BeginSnapshot(0)
				t.Cleanup(tx.Release)
				owner, perms := types.ObjID(7), PropRead|PropWrite
				switch change {
				case "owner":
					if ec := tx.SetPropertyInfo(parent, "p000", &owner, nil); ec != types.E_NONE {
						t.Fatal(ec)
					}
				case "perms":
					if ec := tx.SetPropertyInfo(parent, "p000", nil, &perms); ec != types.E_NONE {
						t.Fatal(ec)
					}
				case "chown":
					perms |= PropChown
					if ec := tx.SetPropertyInfo(parent, "p000", nil, &perms); ec != types.E_NONE {
						t.Fatal(ec)
					}
				case "added":
					if ec := tx.DefineProperty(parent, "extra", NewProperty(types.NewInt(91), 1, PropRead, false, true)); ec != types.E_NONE {
						t.Fatal(ec)
					}
				}
				// Anonymous creation reads live parents; publish setup for both
				// paths so they inherit identical metadata.
				if ec := tx.Commit(); ec != types.E_NONE {
					t.Fatal(ec)
				}
				tx = s.BeginSnapshot(0)
				defer tx.Release()
				id, ec := tx.CreateObject([]types.ObjID{parent}, 8, anonymous)
				if ec != types.E_NONE {
					t.Fatal(ec)
				}
				if ec := tx.Commit(); ec != types.E_NONE {
					t.Fatal(ec)
				}
				child := s.liveObjectLocked(id)
				wantOwner, wantPerms := types.ObjID(1), PropRead
				switch change {
				case "owner":
					wantOwner = 7
				case "perms":
					wantPerms = perms
				case "chown":
					wantOwner, wantPerms = 8, perms
				}
				slot, ok := child.properties.lookup("p000")
				if !ok || slot.owner != wantOwner || slot.perms != wantPerms || !slot.clear || slot.defined || !slot.value.IsNone() || slot.version != 0 {
					t.Fatalf("inherited p000 = %+v, want clear owner=%d perms=%v", slot, wantOwner, wantPerms)
				}
				if change == "none" && child.properties.s.base != s.load(sibling).properties.s.base {
					t.Fatal("unchanged inherited slots did not share the loaded sibling's base")
				}
				if change == "added" {
					if _, ok := child.properties.lookup("extra"); !ok {
						t.Fatal("new parent slot was lost")
					}
				}
				if ec := s.DirectTxn().SetPropertyValue(id, "p001", types.NewInt(999)); ec != types.E_NONE {
					t.Fatal(ec)
				}
				view, ec := s.DirectTxn().FindProperty(sibling, "p001")
				if ec != types.E_NONE || view.Value.Int() != 1 {
					t.Fatalf("child write changed sibling: %+v %v", view, ec)
				}
			})
		}
	}
}

func TestCreateInheritedBaseUnionsMultipleParents(t *testing.T) {
	for _, anonymous := range []bool{false, true} {
		t.Run(fmt.Sprintf("anon=%t", anonymous), func(t *testing.T) {
			s := NewStore()
			parent, sibling := loadedClass(t, s, 3)
			addLoaded(t, s, 3, nil, map[string]Property{"extra": NewProperty(types.NewInt(91), 1, PropRead, false, true)})
			s.ShareLoadedPropertySlots(nil)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			id, ec := tx.CreateObject([]types.ObjID{parent, 3}, 1, anonymous)
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.Commit(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			child := s.liveObjectLocked(id)
			if child.properties.count() != 4 || child.properties.privateCount() != 0 {
				t.Fatalf("union has %d slots, %d private", child.properties.count(), child.properties.privateCount())
			}
			if s.load(sibling).properties.count() != 3 {
				t.Fatal("union changed a shared sibling's shape")
			}
			view, ec := s.DirectTxn().FindProperty(id, "extra")
			if ec != types.E_NONE || view.Value.Int() != 91 {
				t.Fatalf("second parent's value = %+v %v", view, ec)
			}
		})
	}
}

func TestCreateInheritedBaseKeepsAncestorDependencies(t *testing.T) {
	s := NewStore()
	parent, sibling := loadedClass(t, s, 3)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	id, ec := tx.CreateObject([]types.ObjID{sibling}, 1)
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.DirectTxn().SetPropertyValue(parent, "p001", types.NewInt(99)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.Commit(); ec == types.E_NONE || !tx.ValidationFailed() {
		t.Fatalf("ancestor changed after inheritance: commit=%v failed=%t", ec, tx.ValidationFailed())
	}
	if s.DirectTxn().Valid(id) {
		t.Fatal("conflicting child was published")
	}
}

func TestVisitPropertyValuesKeepsValueDependencies(t *testing.T) {
	s := NewStore()
	parent, _ := loadedClass(t, s, 3)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	n := 0
	if ec := tx.VisitPropertyValues(parent, func(types.Value) { n++ }); ec != types.E_NONE || n != 3 {
		t.Fatalf("visit = %v, count=%d", ec, n)
	}
	if ec := tx.SetObjectName(parent, "dependent"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.DirectTxn().SetPropertyValue(parent, "p001", types.NewInt(99)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.Commit(); ec == types.E_NONE || !tx.ValidationFailed() {
		t.Fatalf("value changed after visit: commit=%v failed=%t", ec, tx.ValidationFailed())
	}
	if ec := s.DirectTxn().VisitPropertyValues(99999, func(types.Value) { t.Fatal("visited invalid object") }); ec != types.E_INVIND {
		t.Fatalf("invalid object: %v", ec)
	}
}

// Recycling an object drops, from its children and their descendants, every
// slot of a property it defined, as Toast's bf_recycle does by reparenting
// each child through db_change_parents. A stale clear slot made the property
// read E_PROPNF but still accept a write (issue #951).
func TestRecycleDropsPropertiesDefinedByRecycledObject(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, types.ObjNothing)); err != nil {
		t.Fatal(err)
	}
	d := s.DirectTxn()
	// #0 defines top; #1 (child of #0) defines mid; #2 (child of #1) overrides
	// both; #3 (child of #1) inherits mid without overriding it.
	for _, parent := range []types.ObjID{0, 1, 1} {
		if _, ec := d.CreateObject([]types.ObjID{parent}, types.ObjNothing); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	for id, name := range map[types.ObjID]string{0: "top", 1: "mid"} {
		if ec := d.DefineProperty(id, name, NewProperty(types.NewInt(0), id, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	for _, name := range []string{"top", "mid"} {
		if ec := d.SetPropertyValue(2, name, types.NewInt(7)); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	if err := s.Recycle(1); err != nil {
		t.Fatal(err)
	}

	for _, id := range []types.ObjID{2, 3} {
		if ids, _ := d.Parents(id); len(ids) != 1 || ids[0] != 0 {
			t.Fatalf("#%d parents = %v, want [#0]", id, ids)
		}
		if _, ec := d.FindProperty(id, "mid"); ec != types.E_PROPNF {
			t.Fatalf("#%d.mid read gives %s, want E_PROPNF", id, ec)
		}
		tx := s.BeginSnapshot(0)
		if ec := tx.SetPropertyValue(id, "mid", types.NewInt(9)); ec != types.E_PROPNF {
			t.Fatalf("#%d.mid write gives %s, want E_PROPNF", id, ec)
		}
		tx.Release()
		if _, ok, _ := d.LocalProperty(id, "mid"); ok {
			t.Fatalf("#%d keeps a slot of mid after its definer was recycled", id)
		}
	}
	// The override of a property defined by a surviving ancestor stays.
	if p, ec := d.FindProperty(2, "top"); ec != types.E_NONE || !p.Value.Equal(types.NewInt(7)) || p.Clear {
		t.Fatalf("#2.top = %v clear=%v (%s), want the kept override 7", p.Value, p.Clear, ec)
	}
	if p, ok, ec := d.LocalProperty(3, "top"); ec != types.E_NONE || !ok || !p.Clear {
		t.Fatalf("#3 local top slot = %v ok=%v clear=%v (%s), want the inherited clear slot", p.Value, ok, p.Clear, ec)
	}
}
