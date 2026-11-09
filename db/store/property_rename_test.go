package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A rename (Toast db_rename_propdef, db_properties.cc:251-280) renames the
// definition in place, and every descendant's slot keeps its contents under the
// new name. Each test runs it on every route a write publishes by: the direct
// transaction (writeNow), a task transaction's commit (shared, or exclusive
// when an anonymous child is written) and the coarse flush.

type renameFixture struct {
	s                                *Store
	owner, parent, child, grandchild types.ObjID
	anon                             types.ObjID // types.ObjNothing without an anonymous child
}

func mustCreate(t *testing.T, d *StoreTxn, parent types.ObjID, anonymous bool) types.ObjID {
	t.Helper()
	id, ec := d.CreateObject([]types.ObjID{parent}, 0, anonymous)
	if ec != types.E_NONE {
		t.Fatalf("create child of #%d: %s", parent, ec)
	}
	return id
}

func mustDefine(t *testing.T, d *StoreTxn, id types.ObjID, name string, value int64) {
	t.Helper()
	if ec := d.DefineProperty(id, name, NewProperty(types.NewInt(value), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("define #%d.%s: %s", id, name, ec)
	}
}

func mustSet(t *testing.T, d *StoreTxn, id types.ObjID, name string, value int64) {
	t.Helper()
	if ec := d.SetPropertyValue(id, name, types.NewInt(value)); ec != types.E_NONE {
		t.Fatalf("set #%d.%s: %s", id, name, ec)
	}
}

// newRenameFixture builds parent (defining one, foo, three), child (foo set to
// 5 and given to owner with r perms), grandchild (foo clear) and, withAnon, an
// anonymous child of parent with foo set to 6.
func newRenameFixture(t *testing.T, withAnon bool) renameFixture {
	t.Helper()
	s := oneBodyStore(t)
	d := s.DirectTxn()
	f := renameFixture{s: s, anon: types.ObjNothing}
	f.owner = mustCreate(t, d, 0, false)
	f.parent = mustCreate(t, d, 0, false)
	mustDefine(t, d, f.parent, "one", 1)
	mustDefine(t, d, f.parent, "foo", 2)
	mustDefine(t, d, f.parent, "three", 3)
	f.child = mustCreate(t, d, f.parent, false)
	mustSet(t, d, f.child, "foo", 5)
	perms := PropRead
	if ec := d.SetPropertyInfo(f.child, "foo", &f.owner, &perms); ec != types.E_NONE {
		t.Fatalf("set child info: %s", ec)
	}
	f.grandchild = mustCreate(t, d, f.child, false)
	if withAnon {
		f.anon = mustCreate(t, d, f.parent, true)
		mustSet(t, d, f.anon, "foo", 6)
	}
	return f
}

// renameRoutes runs op on each route and leaves its writes published.
var renameRoutes = []struct {
	name string
	run  func(t *testing.T, s *Store, op func(tx *StoreTxn) types.ErrorCode)
}{
	{"direct", func(t *testing.T, s *Store, op func(tx *StoreTxn) types.ErrorCode) {
		t.Helper()
		if ec := op(s.DirectTxn()); ec != types.E_NONE {
			t.Fatalf("op: %s", ec)
		}
	}},
	{"commit", func(t *testing.T, s *Store, op func(tx *StoreTxn) types.ErrorCode) {
		t.Helper()
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		if ec := op(tx); ec != types.E_NONE {
			t.Fatalf("op: %s", ec)
		}
		if ec := tx.Commit(); ec != types.E_NONE {
			t.Fatalf("Commit: %s", ec)
		}
	}},
	{"flush", func(t *testing.T, s *Store, op func(tx *StoreTxn) types.ErrorCode) {
		t.Helper()
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		if ec := op(tx); ec != types.E_NONE {
			t.Fatalf("op: %s", ec)
		}
		if ec := tx.FlushStagedToLive(); ec != types.E_NONE {
			t.Fatalf("FlushStagedToLive: %s", ec)
		}
	}},
}

func eachRenameCase(t *testing.T, test func(t *testing.T, f renameFixture, run func(op func(tx *StoreTxn) types.ErrorCode))) {
	for _, route := range renameRoutes {
		for _, withAnon := range []bool{false, true} {
			name := route.name
			if withAnon {
				name += "/anonymous_child"
			}
			t.Run(name, func(t *testing.T) {
				f := newRenameFixture(t, withAnon)
				test(t, f, func(op func(tx *StoreTxn) types.ErrorCode) { route.run(t, f.s, op) })
			})
		}
	}
}

func wantValue(t *testing.T, tx *StoreTxn, id types.ObjID, name string, want int64) {
	t.Helper()
	got, ec := tx.PropertyValue(id, name)
	if ec != types.E_NONE || !got.Identical(types.NewInt(want)) {
		t.Errorf("#%d.%s = %v, %s; want %d", id, name, got, ec, want)
	}
}

func wantSlot(t *testing.T, tx *StoreTxn, id types.ObjID, name string, clear bool) PropertyView {
	t.Helper()
	view, ok, ec := tx.LocalProperty(id, name)
	if ec != types.E_NONE || !ok {
		t.Fatalf("#%d has no slot %q: %s", id, name, ec)
	}
	if view.Clear != clear || view.Defined {
		t.Errorf("#%d slot %q clear=%v defined=%v, want clear=%v defined=false", id, name, view.Clear, view.Defined, clear)
	}
	return view
}

func wantNoSlot(t *testing.T, tx *StoreTxn, id types.ObjID, name string) {
	t.Helper()
	if _, ok, ec := tx.LocalProperty(id, name); ec != types.E_NONE || ok {
		t.Errorf("#%d slot %q: present=%v, %s; want absent", id, name, ok, ec)
	}
}

func wantDefined(t *testing.T, tx *StoreTxn, id types.ObjID, want ...string) {
	t.Helper()
	got, ec := tx.DefinedPropertyNames(id)
	if ec != types.E_NONE || !slices.Equal(got, want) {
		t.Errorf("#%d defines %q, %s; want %q", id, got, ec, want)
	}
}

func TestRenamePropertyKeepsDefinitionAndDescendantSlots(t *testing.T) {
	eachRenameCase(t, func(t *testing.T, f renameFixture, run func(op func(tx *StoreTxn) types.ErrorCode)) {
		run(func(tx *StoreTxn) types.ErrorCode { return tx.RenameProperty(f.parent, "FOO", "Bar") })

		tx := f.s.BeginSnapshot(0)
		defer tx.Release()
		wantDefined(t, tx, f.parent, "one", "Bar", "three")
		wantValue(t, tx, f.parent, "bar", 2)
		if _, ec := tx.FindProperty(f.parent, "foo"); ec != types.E_PROPNF {
			t.Errorf("parent.foo after rename: %s, want E_PROPNF", ec)
		}
		child := wantSlot(t, tx, f.child, "bar", false)
		if !child.Value.Identical(types.NewInt(5)) || child.Owner != f.owner || child.Perms != PropRead {
			t.Errorf("child slot = %+v, want value 5, owner #%d, perms r", child, f.owner)
		}
		wantNoSlot(t, tx, f.child, "foo")
		wantSlot(t, tx, f.grandchild, "bar", true)
		wantNoSlot(t, tx, f.grandchild, "foo")
		wantValue(t, tx, f.grandchild, "bar", 5)
		if f.anon != types.ObjNothing {
			wantSlot(t, tx, f.anon, "bar", false)
			wantNoSlot(t, tx, f.anon, "foo")
			wantValue(t, tx, f.anon, "bar", 6)
		}
	})
}

// Three renames in one transaction swap two names. Each slot ends under the
// name it was renamed to on every object, and reads inside the transaction see
// the renames without making its commit conflict with itself.
func TestRenamePropertySwapInOneTransaction(t *testing.T) {
	eachRenameCase(t, func(t *testing.T, f renameFixture, run func(op func(tx *StoreTxn) types.ErrorCode)) {
		run(func(tx *StoreTxn) types.ErrorCode {
			for _, step := range [][2]string{{"one", "tmp"}, {"three", "one"}, {"tmp", "three"}} {
				if ec := tx.RenameProperty(f.parent, step[0], step[1]); ec != types.E_NONE {
					return ec
				}
			}
			if v, ec := tx.PropertyValue(f.parent, "three"); ec != types.E_NONE || !v.Identical(types.NewInt(1)) {
				t.Errorf("parent.three inside the txn = %v, %s; want 1", v, ec)
			}
			return types.E_NONE
		})

		tx := f.s.BeginSnapshot(0)
		defer tx.Release()
		wantDefined(t, tx, f.parent, "three", "foo", "one")
		wantValue(t, tx, f.parent, "one", 3)
		wantValue(t, tx, f.parent, "three", 1)
		wantSlot(t, tx, f.child, "one", true)
		wantSlot(t, tx, f.child, "three", true)
		wantNoSlot(t, tx, f.child, "tmp")
		wantValue(t, tx, f.grandchild, "one", 3)
	})
}

// The swap of a slot a descendant has written: on the exclusive route the store
// walk and the staged writes both move it, and it must still end under the
// name it was renamed to.
func TestRenamePropertySwapMovesWrittenSlots(t *testing.T) {
	eachRenameCase(t, func(t *testing.T, f renameFixture, run func(op func(tx *StoreTxn) types.ErrorCode)) {
		run(func(tx *StoreTxn) types.ErrorCode {
			for _, step := range [][2]string{{"foo", "tmp"}, {"one", "foo"}, {"tmp", "one"}} {
				if ec := tx.RenameProperty(f.parent, step[0], step[1]); ec != types.E_NONE {
					return ec
				}
			}
			return types.E_NONE
		})

		tx := f.s.BeginSnapshot(0)
		defer tx.Release()
		wantDefined(t, tx, f.parent, "foo", "one", "three")
		wantValue(t, tx, f.parent, "one", 2)
		wantValue(t, tx, f.parent, "foo", 1)
		wantSlot(t, tx, f.child, "one", false)
		wantValue(t, tx, f.child, "one", 5)
		wantSlot(t, tx, f.child, "foo", true)
		wantValue(t, tx, f.child, "foo", 1)
		if f.anon != types.ObjNothing {
			wantValue(t, tx, f.anon, "one", 6)
			wantSlot(t, tx, f.anon, "foo", true)
		}
	})
}

// After a rename the old name is free: it can be defined again in the same
// transaction, and deleting the renamed property deletes it from descendants.
func TestRenamePropertyThenDefineOldNameAndDeleteNew(t *testing.T) {
	eachRenameCase(t, func(t *testing.T, f renameFixture, run func(op func(tx *StoreTxn) types.ErrorCode)) {
		run(func(tx *StoreTxn) types.ErrorCode {
			if ec := tx.RenameProperty(f.parent, "foo", "bar"); ec != types.E_NONE {
				return ec
			}
			if ec := tx.DefineProperty(f.parent, "foo", NewProperty(types.NewInt(7), 0, PropRead, false, true)); ec != types.E_NONE {
				return ec
			}
			return tx.DeleteDefinedProperty(f.parent, "bar")
		})

		tx := f.s.BeginSnapshot(0)
		defer tx.Release()
		wantDefined(t, tx, f.parent, "one", "three", "foo")
		wantValue(t, tx, f.parent, "foo", 7)
		for _, id := range []types.ObjID{f.child, f.grandchild, f.anon} {
			if id == types.ObjNothing {
				continue
			}
			wantSlot(t, tx, id, "foo", true)
			wantValue(t, tx, id, "foo", 7)
			wantNoSlot(t, tx, id, "bar")
		}
	})
}

// A property renamed after this transaction read it is stale at its commit.
func TestRenamePropertyConflictsWithStaleWriter(t *testing.T) {
	f := newRenameFixture(t, false)
	stale := f.s.BeginSnapshot(0)
	defer stale.Release()
	if _, ec := stale.PropertyValue(f.parent, "foo"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := f.s.DirectTxn().RenameProperty(f.parent, "foo", "bar"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	mustSet(t, stale, f.parent, "foo", 9)
	if ec := stale.Commit(); ec != types.E_INVARG || !stale.ValidationFailed() {
		t.Fatalf("stale Commit = %s, validation failed %v; want E_INVARG, true", ec, stale.ValidationFailed())
	}
	tx := f.s.BeginSnapshot(0)
	defer tx.Release()
	wantDefined(t, tx, f.parent, "one", "bar", "three")
	wantValue(t, tx, f.parent, "bar", 2)
}

func TestRenamePropertyRefusals(t *testing.T) {
	f := newRenameFixture(t, false)
	d := f.s.DirectTxn()
	for _, c := range []struct {
		obj      types.ObjID
		from, to string
		want     types.ErrorCode
	}{
		{f.child, "foo", "bar", types.E_PROPNF},     // inherited, not defined here
		{f.parent, "nosuch", "bar", types.E_PROPNF}, // no such property
		{f.parent, "foo", "THREE", types.E_INVARG},  // another slot of the object
	} {
		if ec := d.RenameProperty(c.obj, c.from, c.to); ec != c.want {
			t.Errorf("RenameProperty(#%d, %q, %q) = %s, want %s", c.obj, c.from, c.to, ec, c.want)
		}
	}
	tx := f.s.BeginSnapshot(0)
	defer tx.Release()
	wantDefined(t, tx, f.parent, "one", "foo", "three")
}
