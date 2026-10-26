package store

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// loadedClass adds, as the loader does, a parent defining slots properties and
// a loaded child of it whose every slot is clear, then shares the loaded slots.
func loadedClass(tb testing.TB, s *Store, slots int) (parent, sibling types.ObjID) {
	tb.Helper()
	parent, sibling = 1, 2
	defined := make(map[string]Property, slots)
	inherited := make(map[string]Property, slots)
	for i := range slots {
		name := fmt.Sprintf("p%03d", i)
		defined[name] = Property{value: types.NewInt(int64(i)), owner: 1, perms: PropRead, defined: true}
		inherited[name] = placeholder(1, PropRead)
	}
	addLoaded(tb, s, parent, nil, defined)
	addLoaded(tb, s, sibling, []types.ObjID{parent}, inherited)
	s.ShareLoadedPropertySlots(nil)
	return parent, sibling
}

func addLoaded(tb testing.TB, s *Store, id types.ObjID, parents []types.ObjID, props map[string]Property) {
	tb.Helper()
	b := NewObjectBuilder(id)
	b.SetName(fmt.Sprintf("obj%d", id))
	b.SetOwner(1)
	b.SetLocation(types.ObjNothing)
	b.SetParents(parents)
	order := make([]string, 0, len(props))
	for name, prop := range props {
		b.SetProperty(name, prop)
		if prop.defined {
			order = append(order, name)
		}
	}
	b.SetPropOrder(order)
	if err := s.Add(b.Build()); err != nil {
		tb.Fatalf("add #%d: %v", id, err)
	}
}

func createCommitted(tb testing.TB, s *Store, parent types.ObjID) types.ObjID {
	tb.Helper()
	tx := s.BeginSnapshot(0)
	id, code := tx.CreateObject([]types.ObjID{parent}, 1)
	if code != types.E_NONE {
		tb.Fatalf("CreateObject: %v", code)
	}
	if code := tx.Commit(); code != types.E_NONE {
		tb.Fatalf("Commit: %v", code)
	}
	return id
}

// A created object stores its inherited slots as a loaded one does: clear,
// on the same shared base as a loaded sibling, nothing private.
func TestCreatedChildSharesLoadedSiblingsBase(t *testing.T) {
	create := map[string]func(*Store, types.ObjID) types.ObjID{
		"txn": func(s *Store, parent types.ObjID) types.ObjID { return createCommitted(t, s, parent) },
		"direct": func(s *Store, parent types.ObjID) types.ObjID {
			id, code := s.DirectTxn().CreateObject([]types.ObjID{parent}, 1)
			if code != types.E_NONE {
				t.Fatalf("CreateObject: %v", code)
			}
			return id
		},
	}
	for name, create := range create {
		t.Run(name, func(t *testing.T) {
			s := NewStore()
			parent, sibling := loadedClass(t, s, 20)
			child := create(s, parent)

			got, want := s.load(child).properties, s.load(sibling).properties
			if got.s.base != want.s.base {
				t.Fatalf("created child's base %p, loaded sibling's %p", got.s.base, want.s.base)
			}
			if n := got.privateCount(); n != 0 {
				t.Fatalf("created child stores %d slots privately, want 0", n)
			}
			for _, name := range []string{"p000", "p007", "p019"} {
				view, code := s.DirectTxn().FindProperty(child, name)
				if code != types.E_NONE {
					t.Fatalf("%s: %v", name, code)
				}
				parentView, _ := s.DirectTxn().FindProperty(parent, name)
				if !view.Value.Equal(parentView.Value) {
					t.Fatalf("%s = %v, want the parent's %v", name, view.Value, parentView.Value)
				}
				if clear, _ := s.DirectTxn().PropertyClearState(child, name); !clear {
					t.Fatalf("%s is not clear on the created child", name)
				}
			}
		})
	}
}

// Defining a property on an object whose slots are shared keeps them shared;
// only the new definition is stored privately.
func TestDefinePropertyKeepsObjectShared(t *testing.T) {
	s := NewStore()
	parent, sibling := loadedClass(t, s, 20)
	child := createCommitted(t, s, parent)

	tx := s.BeginSnapshot(0)
	if code := tx.DefineProperty(child, "own", NewProperty(types.NewInt(1), 1, PropRead, false, true)); code != types.E_NONE {
		t.Fatalf("DefineProperty: %v", code)
	}
	if code := tx.Commit(); code != types.E_NONE {
		t.Fatalf("Commit: %v", code)
	}

	got := s.load(child).properties
	if got.s.base != s.load(sibling).properties.s.base {
		t.Fatalf("defining a property moved the object off its shared base")
	}
	if n := got.privateCount(); n != 1 {
		t.Fatalf("object stores %d slots privately, want 1 (the definition)", n)
	}
}

// BenchmarkCreateChild times creating and committing one child of a loaded
// 150-slot class.
func BenchmarkCreateChild(b *testing.B) {
	s := NewStore()
	parent, _ := loadedClass(b, s, 150)
	for b.Loop() {
		createCommitted(b, s, parent)
	}
}

// BenchmarkCreatedChildRetainedBytes reports the heap retained per object for
// 10,000 objects created at run time and for 10,000 loaded siblings of the
// same 150-slot class (a loaded Mongoose object averages 152 slots). Run with
// -benchtime=1x.
func BenchmarkCreatedChildRetainedBytes(b *testing.B) {
	const objects, slots = 10000, 150
	retained := func(fill func(s *Store, parent types.ObjID)) float64 {
		s := NewStore()
		parent, _ := loadedClass(b, s, slots)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		fill(s, parent)
		runtime.GC()
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(s)
		return float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / objects
	}
	for b.Loop() {
		created := retained(func(s *Store, parent types.ObjID) {
			for range objects {
				createCommitted(b, s, parent)
			}
		})
		loaded := retained(func(s *Store, parent types.ObjID) {
			inherited := make(map[string]Property, slots)
			for i := range slots {
				inherited[fmt.Sprintf("p%03d", i)] = placeholder(1, PropRead)
			}
			for i := range objects {
				props := make(map[string]Property, slots)
				for name, prop := range inherited {
					props[name] = prop
				}
				addLoaded(b, s, types.ObjID(100+i), []types.ObjID{parent}, props)
			}
			s.ShareLoadedPropertySlots(nil)
		})
		b.ReportMetric(created, "B/created")
		b.ReportMetric(loaded, "B/loaded")
	}
}
