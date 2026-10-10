package store

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func sameProperty(a, b Property) bool {
	return a.owner == b.owner && a.perms == b.perms && a.clear == b.clear &&
		a.defined == b.defined && a.version == b.version && a.value.Identical(b.value)
}

// requireTableMatches checks every read path of table against the plain map
// it is supposed to behave as.
func requireTableMatches(t *testing.T, table propTable, want map[string]Property, when string) {
	t.Helper()
	if got := table.count(); got != len(want) {
		t.Fatalf("%s: count = %d, want %d", when, got, len(want))
	}
	seen := make(map[string]bool, len(want))
	for key, prop := range table.all() {
		if seen[key] {
			t.Fatalf("%s: all() visited %q twice", when, key)
		}
		seen[key] = true
		wantProp, ok := want[key]
		if !ok {
			t.Fatalf("%s: all() yielded %q, which is not in the table", when, key)
		}
		if !sameProperty(prop, wantProp) {
			t.Fatalf("%s: all() %q = %+v, want %+v", when, key, prop, wantProp)
		}
	}
	for key, wantProp := range want {
		if !seen[key] {
			t.Fatalf("%s: all() missed %q", when, key)
		}
		prop, ok := table.lookup(key)
		if !ok || !sameProperty(prop, wantProp) {
			t.Fatalf("%s: lookup(%q) = %+v, %v, want %+v", when, key, prop, ok, wantProp)
		}
	}
}

func placeholder(owner types.ObjID, perms PropertyPerms) Property {
	return Property{value: types.None, owner: owner, perms: perms, clear: true}
}

// classTable builds the kind of table a loaded object has: mostly clear
// inherited slots, a few set ones.
func classTable(set map[string]Property) (propTable, map[string]Property) {
	want := make(map[string]Property)
	for i := range 40 {
		want[fmt.Sprintf("p%02d", i)] = placeholder(types.ObjID(2+i%3), PropRead|PropChown)
	}
	for key, prop := range set {
		want[key] = prop
	}
	m := make(map[string]Property, len(want))
	for key, prop := range want {
		m[key] = prop
	}
	return propTableFromMap(m), want
}

func TestPropTableShareKeepsWhatItReadsAs(t *testing.T) {
	pool := newPropSharePool()
	set := map[string]Property{
		"p03":  {value: types.NewInt(7), owner: 9, perms: PropRead},
		"p11":  {value: types.NewStr("x"), owner: 2, perms: PropRead | PropWrite, defined: true},
		"p20":  {value: types.None, owner: 2, perms: PropRead, clear: true, version: 5},
		"own1": {value: types.NewInt(1), owner: 4, perms: PropRead, defined: true},
	}
	table, want := classTable(set)

	table.share(pool)

	requireTableMatches(t, table, want, "after share")
	if got := table.privateCount(); got != len(set) {
		t.Fatalf("privateCount = %d, want the %d slots that are not placeholders", got, len(set))
	}
}

func TestPropTableShareGivesSiblingsOneBase(t *testing.T) {
	pool := newPropSharePool()
	first, _ := classTable(map[string]Property{"p01": {value: types.NewInt(1), owner: 3, perms: PropRead | PropChown}})
	second, _ := classTable(map[string]Property{"p30": {value: types.NewInt(2), owner: 2, perms: PropRead | PropChown}})
	otherOwner, _ := classTable(map[string]Property{"p01": {value: types.NewInt(1), owner: 77, perms: PropRead | PropChown}})

	first.share(pool)
	second.share(pool)
	otherOwner.share(pool)

	if first.s.base == nil || first.s.base != second.s.base {
		t.Fatalf("two objects with the same slots, owners and perms do not share a base")
	}
	if otherOwner.s.base == first.s.base {
		t.Fatalf("a slot with a different owner shares a base")
	}
	if otherOwner.s.base.shape != first.s.base.shape {
		t.Fatalf("the same slot names do not share a shape")
	}
}

func TestPropTableWritesNeverReachTheBase(t *testing.T) {
	pool := newPropSharePool()
	first, wantFirst := classTable(nil)
	second, wantSecond := classTable(nil)
	first.share(pool)
	second.share(pool)

	first.put("p05", Property{value: types.NewInt(5), owner: 2, perms: PropRead, version: 9})
	first.remove("p06")
	first.put("fresh", Property{value: types.NewInt(1), owner: 2, perms: PropRead, defined: true})
	wantFirst["p05"] = Property{value: types.NewInt(5), owner: 2, perms: PropRead, version: 9}
	delete(wantFirst, "p06")
	wantFirst["fresh"] = Property{value: types.NewInt(1), owner: 2, perms: PropRead, defined: true}

	requireTableMatches(t, first, wantFirst, "written table")
	requireTableMatches(t, second, wantSecond, "sibling of the written table")
}

func TestPropTableCopyAliasesAndCloneDoesNot(t *testing.T) {
	for _, shared := range []bool{false, true} {
		table, want := classTable(nil)
		if shared {
			table.share(newPropSharePool())
		}
		alias := table
		copied := table.clone()
		wantCopied := make(map[string]Property, len(want))
		for key, prop := range want {
			wantCopied[key] = prop
		}

		alias.put("p01", Property{value: types.NewInt(1), owner: 2, perms: PropRead})
		alias.remove("p02")
		alias.put("added", Property{value: types.NewInt(2), owner: 2, perms: PropRead})
		want["p01"] = Property{value: types.NewInt(1), owner: 2, perms: PropRead}
		delete(want, "p02")
		want["added"] = Property{value: types.NewInt(2), owner: 2, perms: PropRead}

		requireTableMatches(t, table, want, fmt.Sprintf("shared=%v: table written through its alias", shared))
		requireTableMatches(t, copied, wantCopied, fmt.Sprintf("shared=%v: clone taken before the writes", shared))

		copied.put("p03", Property{value: types.NewInt(3), owner: 2, perms: PropRead})
		requireTableMatches(t, table, want, fmt.Sprintf("shared=%v: table after its clone was written", shared))
	}
}

func TestPropTableFindFoldsCase(t *testing.T) {
	for _, shared := range []bool{false, true} {
		table, want := classTable(map[string]Property{"mixed": {value: types.NewInt(1), owner: 2, perms: PropRead}})
		if shared {
			table.share(newPropSharePool())
		}
		for _, name := range []string{"P07", "p07", "MiXeD"} {
			key, prop, ok := table.find(name)
			if !ok || key != propertyNameKey(name) || !sameProperty(prop, want[propertyNameKey(name)]) {
				t.Fatalf("shared=%v: find(%q) = %q, %+v, %v", shared, name, key, prop, ok)
			}
		}
		if _, _, ok := table.find("absent"); ok {
			t.Fatalf("shared=%v: find reported a slot that is not there", shared)
		}
	}
}

func TestPropTableRangeAllowsWritingTheVisitedSlot(t *testing.T) {
	for _, shared := range []bool{false, true} {
		table, want := classTable(map[string]Property{"own1": {value: types.NewInt(1), owner: 3, perms: PropRead, defined: true}})
		if shared {
			table.share(newPropSharePool())
		}
		visits := make(map[string]int)
		for key, prop := range table.all() {
			visits[key]++
			switch prop.owner {
			case 3:
				prop.owner = 30
				table.put(key, prop)
				changed := want[key]
				changed.owner = 30
				want[key] = changed
			case 4:
				table.remove(key)
				delete(want, key)
			}
		}
		for key, n := range visits {
			if n != 1 {
				t.Fatalf("shared=%v: %q visited %d times", shared, key, n)
			}
		}
		if len(visits) != 41 {
			t.Fatalf("shared=%v: visited %d slots, want 41", shared, len(visits))
		}
		requireTableMatches(t, table, want, fmt.Sprintf("shared=%v: after writing during the range", shared))
	}
}

// A shared table keeps the slots it sets in a sorted slice. Writing the
// visited slot during a range inserts into or deletes from that slice; every
// slot must still be yielded once with the value it had before the range.
func TestPropTableRangeYieldsEachSlotsValueWhileTheBodyWrites(t *testing.T) {
	for _, write := range []string{"put", "remove", "alternate"} {
		set := make(map[string]Property)
		for i := 0; i < 40; i += 3 {
			set[fmt.Sprintf("p%02d", i)] = Property{value: types.NewInt(int64(i)), owner: 9, perms: PropRead}
		}
		table, want := classTable(set)
		table.share(newPropSharePool())
		before := make(map[string]Property, len(want))
		for key, prop := range want {
			before[key] = prop
		}

		visited := 0
		for key, prop := range table.all() {
			if !sameProperty(prop, before[key]) {
				t.Fatalf("%s: range yielded %q = %+v, want %+v", write, key, prop, before[key])
			}
			delete(before, key)
			doPut := write == "put" || (write == "alternate" && visited%2 == 0)
			if doPut {
				prop.version = 7
				table.put(key, prop)
				want[key] = prop
			} else {
				table.remove(key)
				delete(want, key)
			}
			visited++
		}
		if len(before) != 0 {
			t.Fatalf("%s: range missed %d slots", write, len(before))
		}
		requireTableMatches(t, table, want, write+": after the range")
	}
}

// TestPropTableBehavesAsAMap drives a table and a plain map with the same
// random puts and removes, sharing the table part-way, and compares every
// read path after each step.
func TestPropTableBehavesAsAMap(t *testing.T) {
	names := make([]string, 60)
	for i := range names {
		names[i] = fmt.Sprintf("p%02d", i)
	}
	for seed := range uint64(40) {
		rng := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
		table, want := classTable(nil)
		shareAt := rng.IntN(60)
		for step := range 300 {
			if step == shareAt {
				table.share(newPropSharePool())
				requireTableMatches(t, table, want, fmt.Sprintf("seed %d: share at step %d", seed, step))
			}
			key := names[rng.IntN(len(names))]
			when := fmt.Sprintf("seed %d step %d", seed, step)
			switch rng.IntN(4) {
			case 0:
				table.remove(key)
				delete(want, key)
			case 1:
				prop := placeholder(types.ObjID(2+rng.IntN(3)), PropRead|PropChown)
				table.put(key, prop)
				want[key] = prop
			default:
				prop := Property{
					value:   types.NewInt(int64(rng.IntN(5))),
					owner:   types.ObjID(2 + rng.IntN(3)),
					perms:   PropertyPerms(rng.IntN(8)),
					clear:   rng.IntN(5) == 0,
					defined: rng.IntN(5) == 0,
					version: uint64(rng.IntN(3)),
				}
				table.put(key, prop)
				want[key] = prop
			}
			requireTableMatches(t, table, want, when)
		}
		copied := table.clone()
		requireTableMatches(t, copied, want, fmt.Sprintf("seed %d: clone", seed))
	}
}

func TestZeroPropTableReadsAsEmpty(t *testing.T) {
	var table propTable
	if table.count() != 0 {
		t.Fatalf("zero table count = %d", table.count())
	}
	if _, ok := table.lookup("x"); ok {
		t.Fatalf("zero table has a slot")
	}
	for range table.all() {
		t.Fatalf("zero table yielded a slot")
	}
	table.remove("x")
}

// BenchmarkPropTableLookup times one slot read: from a table with no base,
// from a shared base, and from the private part of a table that has a base.
func BenchmarkPropTableLookup(b *testing.B) {
	set := map[string]Property{"p03": {value: types.NewInt(7), owner: 9, perms: PropRead}}
	private, _ := classTable(set)
	shared, _ := classTable(set)
	shared.share(newPropSharePool())
	cases := []struct {
		name  string
		table propTable
		key   string
	}{
		{"private", private, "p20"},
		{"shared-base", shared, "p20"},
		{"shared-override", shared, "p03"},
		{"shared-miss", shared, "absent"},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				c.table.lookup(c.key)
			}
		})
	}
}

func TestShareLoadedPropertySlotsKeepsEveryObjectsProperties(t *testing.T) {
	s := NewStore()
	const parent, first, second = types.ObjID(1), types.ObjID(2), types.ObjID(3)
	build := func(id types.ObjID, parents []types.ObjID, props map[string]Property) {
		b := NewObjectBuilder(id)
		b.SetName(fmt.Sprintf("obj%d", id))
		b.SetOwner(1)
		b.SetLocation(types.ObjNothing)
		b.SetParents(parents)
		order := make([]string, 0, len(props))
		for name, prop := range props {
			b.SetProperty(name, prop)
			order = append(order, name)
		}
		b.SetPropOrder(order)
		if err := s.Add(b.Build()); err != nil {
			t.Fatalf("add #%d: %v", id, err)
		}
	}
	build(parent, nil, map[string]Property{
		"color": {value: types.NewStr("red"), owner: 1, perms: PropRead | PropChown, defined: true},
		"size":  {value: types.NewInt(3), owner: 1, perms: PropRead, defined: true},
	})
	build(first, []types.ObjID{parent}, map[string]Property{
		"color": placeholder(1, PropRead|PropChown),
		"size":  {value: types.NewInt(9), owner: 1, perms: PropRead},
	})
	build(second, []types.ObjID{parent}, map[string]Property{
		"color": placeholder(1, PropRead|PropChown),
		"size":  placeholder(1, PropRead),
	})

	type reading struct {
		value types.Value
		clear bool
	}
	read := func() map[string]reading {
		got := make(map[string]reading)
		for _, id := range []types.ObjID{parent, first, second} {
			for _, name := range []string{"color", "size"} {
				view, ec := s.findProperty(id, name)
				if ec != types.E_NONE {
					t.Fatalf("#%d.%s: %v", id, name, ec)
				}
				clear, ec := s.propertyClearState(id, name)
				if ec != types.E_NONE {
					t.Fatalf("#%d.%s clear state: %v", id, name, ec)
				}
				got[fmt.Sprintf("#%d.%s", id, name)] = reading{value: view.Value, clear: clear}
			}
		}
		return got
	}
	before := read()
	_, slotsBefore, clearBefore, privateBefore := s.PropertySlotCensus()

	s.ShareLoadedPropertySlots()

	after := read()
	for key, want := range before {
		got := after[key]
		if got.clear != want.clear || !got.value.Equal(want.value) {
			t.Errorf("%s = %+v after sharing, want %+v", key, got, want)
		}
	}
	_, slots, clearSlots, private := s.PropertySlotCensus()
	if slots != slotsBefore || clearSlots != clearBefore {
		t.Fatalf("census changed: slots %d -> %d, clear %d -> %d", slotsBefore, slots, clearBefore, clearSlots)
	}
	if privateBefore != 6 || private != 3 {
		t.Fatalf("private slots %d -> %d, want 6 -> 3 (the two definitions and the one override)", privateBefore, private)
	}
	if s.load(first).properties.s.base.shape != s.load(second).properties.s.base.shape {
		t.Fatalf("siblings do not share a shape")
	}
}
