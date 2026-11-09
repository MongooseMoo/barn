package store

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A coarse commit publishes one image per object. A reader whose snapshot sits
// exactly at that commit's timestamp, and that first touches the object after a
// later commit has superseded it, must be served the image the commit
// published, not one it built on the way.
func TestCoarseCommitHistoryKeepsCommittedImage(t *testing.T) {
	definedA := NewProperty(types.NewInt(1), 0, PropRead, false, true)
	rows := []struct {
		name  string
		stage func(t *testing.T, tx *StoreTxn) types.ObjID
		check func(t *testing.T, reader *StoreTxn, id types.ObjID)
	}{
		{
			name: "create and name",
			stage: func(t *testing.T, tx *StoreTxn) types.ObjID {
				id, ec := tx.CreateObject([]types.ObjID{0}, 0)
				if ec != types.E_NONE {
					t.Fatal(ec)
				}
				if ec := tx.SetObjectName(id, "committed"); ec != types.E_NONE {
					t.Fatal(ec)
				}
				return id
			},
			check: func(t *testing.T, reader *StoreTxn, id types.ObjID) {
				if name, ec := reader.ObjectName(id); ec != types.E_NONE || name != "committed" {
					t.Fatalf("reader name = %q, %v; want committed", name, ec)
				}
			},
		},
		{
			name: "scalar and define",
			stage: func(t *testing.T, tx *StoreTxn) types.ObjID {
				if ec := tx.SetObjectName(1, "n"); ec != types.E_NONE {
					t.Fatal(ec)
				}
				if ec := tx.DefineProperty(1, "x", NewProperty(types.NewInt(1), 0, PropRead, false, true)); ec != types.E_NONE {
					t.Fatal(ec)
				}
				return 1
			},
			check: func(t *testing.T, reader *StoreTxn, id types.ObjID) {
				if v, ec := reader.PropertyValue(id, "x"); ec != types.E_NONE || v.Int() != 1 {
					t.Fatalf("reader x = %v, %v; want 1", v, ec)
				}
			},
		},
		{
			name: "delete then redefine",
			stage: func(t *testing.T, tx *StoreTxn) types.ObjID {
				if ec := tx.DeleteDefinedProperty(2, "a"); ec != types.E_NONE {
					t.Fatal(ec)
				}
				if ec := tx.DefineProperty(2, "a", NewProperty(types.NewInt(2), 0, PropRead, false, true)); ec != types.E_NONE {
					t.Fatal(ec)
				}
				return 2
			},
			check: func(t *testing.T, reader *StoreTxn, id types.ObjID) {
				if v, ec := reader.PropertyValue(id, "a"); ec != types.E_NONE || v.Int() != 2 {
					t.Fatalf("reader a = %v, %v; want 2", v, ec)
				}
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			s := NewStore()
			for id := types.ObjID(0); id <= 2; id++ {
				if err := s.Add(NewObject(id, 0)); err != nil {
					t.Fatal(err)
				}
			}
			if ec := s.DirectTxn().DefineProperty(2, "a", definedA); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx := s.BeginSnapshot(0)
			id := row.stage(t, tx)
			tx.MarkLiveMutated()
			if ec := tx.Commit(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx.Release()

			reader := s.BeginSnapshot(0)
			defer reader.Release()
			later := s.BeginSnapshot(0)
			if ec := later.SetObjectName(id, "later"); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := later.Commit(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			later.Release()
			row.check(t, reader, id)
		})
	}
}

// Both commit routes must publish the same state for the same staged writes.
// Fixture: #0 root, #1 child of #0 that defines "x", #2 child of #1, #3 a room,
// #4 a thing in #0, #5 a plain object carrying verb "look".
func TestCommitRoutesPublishSameState(t *testing.T) {
	scenarios := []struct {
		name  string
		setup func(t *testing.T, s *Store) []types.ObjID
		stage func(t *testing.T, tx *StoreTxn, ids []types.ObjID) []types.ObjID
		check func(t *testing.T, s *Store, ids []types.ObjID)
	}{
		{
			name: "define with child",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				mustOK(t, tx.DefineProperty(1, "y", NewProperty(types.NewInt(7), 0, PropRead, false, true)))
				return nil
			},
			check: func(t *testing.T, s *Store, _ []types.ObjID) {
				view, ok, ec := s.DirectTxn().LocalProperty(2, "y")
				if ec != types.E_NONE || !ok {
					t.Fatalf("child #2 slot y: ok=%v ec=%v", ok, ec)
				}
				if clear, ec := s.DirectTxn().PropertyClearState(2, "y"); ec != types.E_NONE || !clear {
					t.Fatalf("child #2 slot y clear=%v ec=%v view=%+v", clear, ec, view)
				}
			},
		},
		{
			name: "definition delete with overriding child",
			setup: func(t *testing.T, s *Store) []types.ObjID {
				mustOK(t, s.DirectTxn().SetPropertyValue(2, "x", types.NewInt(5)))
				return nil
			},
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				mustOK(t, tx.DeleteDefinedProperty(1, "x"))
				return nil
			},
			check: func(t *testing.T, s *Store, _ []types.ObjID) {
				if _, ok, _ := s.DirectTxn().LocalProperty(2, "x"); ok {
					t.Fatal("child #2 kept slot x after its definition was deleted")
				}
			},
		},
		{
			name: "definition delete with anonymous child",
			setup: func(t *testing.T, s *Store) []types.ObjID {
				anon, ec := s.DirectTxn().CreateObject([]types.ObjID{1}, 0, true)
				mustOK(t, ec)
				return []types.ObjID{anon}
			},
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				mustOK(t, tx.DeleteDefinedProperty(1, "x"))
				return nil
			},
			check: func(t *testing.T, s *Store, ids []types.ObjID) {
				if _, ok, _ := s.DirectTxn().LocalProperty(ids[0], "x"); ok {
					t.Fatal("anonymous child kept slot x after its definition was deleted")
				}
			},
		},
		{
			name: "delete then redefine",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				mustOK(t, tx.DeleteDefinedProperty(1, "x"))
				mustOK(t, tx.DefineProperty(1, "x", NewProperty(types.NewInt(9), 0, PropRead, false, true)))
				return nil
			},
			check: func(t *testing.T, s *Store, _ []types.ObjID) {
				if v, ec := s.DirectTxn().PropertyValue(2, "x"); ec != types.E_NONE || v.Int() != 9 {
					t.Fatalf("child #2 x = %v, %v; want 9", v, ec)
				}
			},
		},
		{
			name: "verb code",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				mustOK(t, tx.SetVerbCode(5, "look", []string{"return 2;"}))
				return nil
			},
		},
		{
			name: "verb delete",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				resolved, err := tx.ResolveVerbOnObject(5, "look")
				if err != nil {
					t.Fatal(err)
				}
				mustOK(t, tx.DeleteResolvedVerb(resolved))
				return nil
			},
		},
		{
			name: "create with self-writes",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				id, ec := tx.CreateObject([]types.ObjID{1}, 0)
				mustOK(t, ec)
				mustOK(t, tx.SetObjectName(id, "made"))
				mustOK(t, tx.SetPropertyValue(id, "x", types.NewInt(3)))
				return []types.ObjID{id}
			},
		},
		{
			name: "recycle",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				handled, ec := tx.RecycleObject(4)
				if !handled || ec != types.E_NONE {
					t.Fatalf("recycle handled=%v ec=%v", handled, ec)
				}
				return nil
			},
		},
		{
			name: "move",
			stage: func(t *testing.T, tx *StoreTxn, _ []types.ObjID) []types.ObjID {
				mustOK(t, tx.MoveObject(4, 3, 0))
				return nil
			},
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			states := make([]string, len(commitRoutes))
			for i, route := range commitRoutes {
				s := commitRouteFixture(t)
				var ids []types.ObjID
				if sc.setup != nil {
					ids = sc.setup(t, s)
				}
				tx := s.BeginSnapshot(0)
				ids = append(ids, sc.stage(t, tx, ids)...)
				route.prepare(tx)
				if ec := tx.Commit(); ec != types.E_NONE {
					t.Fatalf("%s commit: %v", route.name, ec)
				}
				tx.Release()
				if sc.check != nil {
					t.Run(route.name, func(t *testing.T) { sc.check(t, s, ids) })
				}
				states[i] = publishedState(s, append([]types.ObjID{0, 1, 2, 3, 4, 5}, ids...))
			}
			for i := 1; i < len(states); i++ {
				if states[i] != states[0] {
					t.Fatalf("%s published\n%s\n%s published\n%s", commitRoutes[0].name, states[0], commitRoutes[i].name, states[i])
				}
			}
		})
	}
}

func mustOK(t *testing.T, ec types.ErrorCode) {
	t.Helper()
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
}

func commitRouteFixture(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	direct := s.DirectTxn()
	for _, parents := range [][]types.ObjID{{0}, {1}, nil, nil, nil} {
		if _, ec := direct.CreateObject(parents, 0, false); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	mustOK(t, direct.DefineProperty(1, "x", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)))
	mustOK(t, direct.MoveObject(4, 0, 0))
	if _, ec := s.AddVerb(5, NewVerb([]string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 1;"})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	return s
}

// publishedState renders the published state of ids, versions left out, so two
// stores that took different commit routes can be compared.
func publishedState(s *Store, ids []types.ObjID) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	for _, id := range ids {
		obj := s.load(id)
		if obj == nil {
			obj = s.anonObjects[id]
		}
		if obj == nil {
			fmt.Fprintf(&b, "#%d absent\n", id)
			continue
		}
		fmt.Fprintf(&b, "#%d name=%q owner=%d flags=%v recycled=%v loc=%d contents=%v parents=%v children=%v anon=%v order=%v defs=%d\n",
			id, obj.name, obj.owner, obj.flags, obj.recycled, obj.location, obj.contents, obj.parents, obj.children,
			obj.anonymousChildren, obj.propOrder, obj.propDefsCount)
		var props []string
		for name, prop := range obj.properties.all() {
			props = append(props, fmt.Sprintf("%s=%v clear=%v defined=%v owner=%d perms=%v",
				name, prop.value, prop.clear, prop.defined, prop.owner, prop.perms))
		}
		sort.Strings(props)
		for _, p := range props {
			fmt.Fprintf(&b, "  prop %s\n", p)
		}
		for _, verb := range obj.verbList {
			fmt.Fprintf(&b, "  verb %v code=%q\n", verb.names, verb.code)
		}
	}
	return b.String()
}

// A define staged before another task makes #2 a child of the definer still
// reaches #2 when the task flushes: the flush is not validated, and the store's
// own walk over the live children gives #2 its inherited slot.
func TestFlushDefineReachesChildAddedAfterStaging(t *testing.T) {
	s := NewStore()
	for id := types.ObjID(0); id <= 2; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	mustOK(t, tx.DefineProperty(1, "x", NewProperty(types.NewInt(1), 0, PropRead, false, true)))
	mustOK(t, s.ChangeParents(2, []types.ObjID{1}))
	mustOK(t, tx.FlushStagedToLive())
	if _, ok, ec := s.DirectTxn().LocalProperty(2, "x"); ec != types.E_NONE || !ok {
		t.Fatalf("child #2 slot x after flush: ok=%v ec=%v", ok, ec)
	}
	if clear, ec := s.DirectTxn().PropertyClearState(2, "x"); ec != types.E_NONE || !clear {
		t.Fatalf("child #2 slot x clear=%v ec=%v", clear, ec)
	}
}

// Renumber moves a delete-then-redefine onto the new id and replays it on the
// adopted live image; the view must hold the redefinition.
func TestApplyStagedObjectWritesDeleteThenRedefine(t *testing.T) {
	s := NewStore()
	for id := types.ObjID(0); id <= 2; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Recycle(1); err != nil {
		t.Fatal(err)
	}
	mustOK(t, s.DirectTxn().DefineProperty(2, "a", NewProperty(types.NewInt(1), 0, PropRead, false, true)))
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	mustOK(t, tx.DeleteDefinedProperty(2, "a"))
	mustOK(t, tx.DefineProperty(2, "a", NewProperty(types.NewInt(2), 0, PropRead, false, true)))
	if err := s.Renumber(2, 1); err != nil {
		t.Fatal(err)
	}
	tx.MarkLiveMutated()
	tx.MoveStagedObjectWrites(2, 1)
	tx.ForgetObject(2)
	mustOK(t, tx.AdoptLiveObject(1))
	tx.ApplyStagedObjectWrites(1)
	if v, ec := tx.PropertyValue(1, "a"); ec != types.E_NONE || v.Int() != 2 {
		t.Fatalf("view a = %v, %v; want 2", v, ec)
	}
	names, ec := tx.DefinedPropertyNames(1)
	mustOK(t, ec)
	if len(names) != 1 || names[0] != "a" {
		t.Fatalf("view defined names = %v, want [a]", names)
	}
	mustOK(t, tx.Commit())
	if v, ec := s.DirectTxn().PropertyValue(1, "a"); ec != types.E_NONE || v.Int() != 2 {
		t.Fatalf("published a = %v, %v; want 2", v, ec)
	}
}
