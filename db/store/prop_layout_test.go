package store

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// randomSlots returns positional slots and their names the way a database
// stores them: a few names repeat, some differ only in case, most slots are
// clear placeholders and a few are set.
func randomSlots(rng *rand.Rand, count int) ([]string, []Property) {
	names := make([]string, count)
	slots := make([]Property, count)
	for i := range count {
		switch rng.IntN(12) {
		case 0:
			names[i] = fmt.Sprintf("Name%d", rng.IntN(count+1))
		case 1:
			names[i] = fmt.Sprintf("NAME%d", rng.IntN(count+1))
		default:
			names[i] = fmt.Sprintf("name%d", rng.IntN(2*count+1))
		}
		slots[i] = placeholder(types.ObjID(2+rng.IntN(3)), PropertyPerms(rng.IntN(4)))
		if rng.IntN(5) == 0 {
			slots[i] = Property{value: types.NewInt(int64(i)), owner: types.ObjID(rng.IntN(4)), perms: PropRead, defined: rng.IntN(2) == 0}
		}
	}
	return names, slots
}

// loadedSlots holds slots the way a reader hands them to a builder.
func loadedSlots(slots []Property) *LoadedSlots {
	loaded := &LoadedSlots{}
	for _, prop := range slots {
		loaded.Append(prop)
	}
	return loaded
}

// tableByName is the table the loader used to build: every slot stored under
// its name in position order, a later slot replacing an earlier one of the
// same name.
func tableByName(names []string, slots []Property) map[string]Property {
	want := make(map[string]Property, len(names))
	for i, name := range names {
		want[propertyNameKey(name)] = slots[i]
	}
	return want
}

func TestTableFromLayoutReadsAsSlotsStoredByName(t *testing.T) {
	for seed := uint64(1); seed <= 200; seed++ {
		rng := rand.New(rand.NewPCG(seed, seed))
		names, slots := randomSlots(rng, rng.IntN(60))
		pool := newPropSharePool()

		table := pool.tableFromLayout(pool.Layout(names), loadedSlots(slots))

		want := tableByName(names, slots)
		requireTableMatches(t, table, want, fmt.Sprintf("seed %d", seed))
		private := 0
		for _, prop := range want {
			if !isBasePlaceholder(prop) {
				private++
			}
		}
		if got := table.privateCount(); got != private {
			t.Fatalf("seed %d: %d private slots, want %d (only slots that are not placeholders)", seed, got, private)
		}
	}
}

func TestTableFromLayoutSharesWithTablesSharedFromMaps(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	names, slots := randomSlots(rng, 50)
	pool := newPropSharePool()

	fromMap := propTableFromMap(tableByName(names, slots))
	fromMap.share(pool)
	layout := pool.Layout(names)
	fromLayout := pool.tableFromLayout(layout, loadedSlots(slots))

	if fromLayout.s.base != fromMap.s.base {
		t.Fatalf("the same slots built two bases: one from a map, one from a layout")
	}

	// A sibling with the same names, owners and perms but other values
	// shares the base; one with a different owner does not.
	sibling := append([]Property(nil), slots...)
	sibling[0].value, sibling[0].clear = types.NewStr("set"), false
	if got := pool.tableFromLayout(layout, loadedSlots(sibling)); got.s.base != fromLayout.s.base {
		t.Fatalf("a sibling with the same owners and perms got its own base")
	}
	stranger := append([]Property(nil), slots...)
	stranger[layout.from[0]].owner += 100
	if got := pool.tableFromLayout(layout, loadedSlots(stranger)); got.s.base == fromLayout.s.base {
		t.Fatalf("a table with a different slot owner shares the base")
	}
}

func TestLayoutLastSlotOfARepeatedNameWins(t *testing.T) {
	pool := newPropSharePool()
	names := []string{"colour", "size", "Colour"}
	slots := []Property{
		{value: types.NewInt(1), owner: 2, perms: PropRead, defined: true},
		{value: types.NewInt(2), owner: 2, perms: PropRead, defined: true},
		{value: types.NewInt(3), owner: 3, perms: PropRead},
	}

	layout := pool.Layout(names)
	table := pool.tableFromLayout(layout, loadedSlots(slots))

	if layout.Slots() != 3 || table.count() != 2 {
		t.Fatalf("layout covers %d slots and the table holds %d, want 3 and 2", layout.Slots(), table.count())
	}
	if prop, ok := table.lookup("colour"); !ok || !sameProperty(prop, slots[2]) {
		t.Fatalf("colour = %+v, %v, want the last slot of that name", prop, ok)
	}
}

func TestTableFromEmptyLayoutIsAnEmptyWritableTable(t *testing.T) {
	pool := newPropSharePool()

	table := pool.tableFromLayout(pool.Layout(nil), nil)

	if table.count() != 0 {
		t.Fatalf("count = %d, want 0", table.count())
	}
	table.put("added", placeholder(2, PropRead))
	if _, ok := table.lookup("added"); !ok || table.count() != 1 {
		t.Fatalf("a slot put into an empty layout's table is not there")
	}
}

func TestResolveLoadedSlotsBuildsTheObjectsTable(t *testing.T) {
	pool := NewPropSharePool()
	builder := NewObjectBuilder(5)
	builder.SetPropDefsCount(1)
	builder.SetPropOrder([]string{"Mine"})
	builder.SetLoadedSlots(loadedSlots([]Property{
		NewProperty(types.NewInt(1), 5, PropRead, false, true),
		NewProperty(types.None, 2, PropRead, true, false),
	}))
	if builder.LoadedSlotCount() != 2 {
		t.Fatalf("LoadedSlotCount = %d, want 2", builder.LoadedSlotCount())
	}

	builder.ResolveLoadedSlots(pool.Layout([]string{"Mine", "inherited"}), pool)

	if builder.LoadedSlotCount() != 2 {
		t.Fatalf("LoadedSlotCount after resolving = %d, want 2", builder.LoadedSlotCount())
	}
	mine, ok := builder.Property("mine")
	if !ok || mine.Value.Int() != 1 || !mine.Defined {
		t.Fatalf("mine = %+v, %v", mine, ok)
	}
	inherited, ok := builder.Property("inherited")
	if !ok || !inherited.Clear || inherited.Owner != 2 {
		t.Fatalf("inherited = %+v, %v", inherited, ok)
	}
	obj := builder.Build()
	if obj.properties.privateCount() != 1 {
		t.Fatalf("%d private slots, want 1", obj.properties.privateCount())
	}
}
