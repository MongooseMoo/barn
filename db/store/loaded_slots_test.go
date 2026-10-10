package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestLoadedSlotsKeepOnlySlotsThatAreNotPlaceholders(t *testing.T) {
	slots := []Property{
		placeholder(2, PropRead),
		{value: types.NewInt(1), owner: 3, perms: PropWrite, defined: true},
		placeholder(4, PropRead|PropChown),
		// Clear, but defined: not what a base slot reads as.
		{value: types.None, owner: 5, perms: PropRead, clear: true, defined: true},
		placeholder(6, 0),
	}

	loaded := loadedSlots(slots)

	if loaded.Len() != len(slots) {
		t.Fatalf("Len = %d, want %d", loaded.Len(), len(slots))
	}
	if len(loaded.stored) != 2 {
		t.Fatalf("%d slots kept whole, want 2 (positions 1 and 3)", len(loaded.stored))
	}
	for at, want := range slots {
		if loaded.owners[at] != want.owner || loaded.perms[at] != want.perms {
			t.Fatalf("position %d has owner %d and perms %d, want %d and %d", at, loaded.owners[at], loaded.perms[at], want.owner, want.perms)
		}
		j, ok := loaded.find(int32(at))
		if ok == isBasePlaceholder(want) {
			t.Fatalf("position %d kept whole = %v, want %v", at, ok, !isBasePlaceholder(want))
		}
		if ok && !sameProperty(loaded.stored[j].prop, want) {
			t.Fatalf("position %d = %+v, want %+v", at, loaded.stored[j].prop, want)
		}
	}
	if _, ok := loaded.find(int32(len(slots))); ok {
		t.Fatalf("a position past the last slot is kept whole")
	}
}

func TestEmptyLoadedSlots(t *testing.T) {
	var loaded LoadedSlots

	if loaded.Len() != 0 {
		t.Fatalf("Len = %d, want 0", loaded.Len())
	}
	if _, ok := loaded.find(0); ok {
		t.Fatalf("an empty set of slots keeps position 0 whole")
	}
}

// A reader fills one LoadedSlots for object after object; what a builder was
// given must not change when the reader goes on to the next object.
func TestSetLoadedSlotsKeepsItsOwnCopy(t *testing.T) {
	pool := NewPropSharePool()
	var slots LoadedSlots
	slots.Append(NewProperty(types.NewInt(1), 5, PropRead, false, true))
	slots.Append(placeholder(2, PropRead))
	builder := NewObjectBuilder(5)
	builder.SetLoadedSlots(&slots)

	slots.Reset(3)
	if slots.Len() != 0 {
		t.Fatalf("Len after Reset = %d, want 0", slots.Len())
	}
	slots.Append(NewProperty(types.NewStr("other"), 9, PropWrite, false, true))
	slots.Append(NewProperty(types.NewStr("object"), 9, PropWrite, false, false))
	slots.Append(placeholder(9, PropWrite))

	builder.ResolveLoadedSlots(pool.Layout([]string{"mine", "inherited"}), pool)
	want := map[string]Property{
		"mine":      NewProperty(types.NewInt(1), 5, PropRead, false, true),
		"inherited": placeholder(2, PropRead),
	}
	requireTableMatches(t, builder.obj.properties, want, "after the reader reused its slots")
	if builder.LoadedSlotCount() != 2 {
		t.Fatalf("LoadedSlotCount = %d, want 2", builder.LoadedSlotCount())
	}
}

// A repeated name is supplied by its last slot, whichever of the two slots is
// a placeholder.
func TestTableFromLayoutRepeatedNameTakesTheLastSlot(t *testing.T) {
	set := Property{value: types.NewInt(7), owner: 2, perms: PropRead, defined: true}
	other := Property{value: types.NewInt(8), owner: 2, perms: PropRead}
	names := []string{"colour", "size", "Colour"}
	cases := []struct {
		name    string
		slots   []Property
		private int
	}{
		{"set then placeholder", []Property{set, other, placeholder(3, PropWrite)}, 1},
		{"placeholder then set", []Property{placeholder(3, PropWrite), other, set}, 2},
		{"set then set", []Property{set, other, {value: types.NewInt(9), owner: 3, perms: PropWrite}}, 2},
		{"placeholder then placeholder", []Property{placeholder(2, PropRead), other, placeholder(3, PropWrite)}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := newPropSharePool()

			table := pool.tableFromLayout(pool.Layout(names), loadedSlots(tc.slots))

			requireTableMatches(t, table, map[string]Property{"colour": tc.slots[2], "size": tc.slots[1]}, tc.name)
			if got := table.privateCount(); got != tc.private {
				t.Fatalf("%d private slots, want %d", got, tc.private)
			}
			for i := 1; i < len(table.s.set); i++ {
				if table.s.set[i-1].idx >= table.s.set[i].idx {
					t.Fatalf("set is not sorted by base index: %d before %d", table.s.set[i-1].idx, table.s.set[i].idx)
				}
			}
			// The base takes the last slot's owner and perms too.
			colour := table.s.base.shape.index["colour"]
			if got := table.s.base.slot(colour); got.owner != tc.slots[2].owner || got.perms != tc.slots[2].perms {
				t.Fatalf("base colour has owner %d and perms %d, want the last slot's %d and %d", got.owner, got.perms, tc.slots[2].owner, tc.slots[2].perms)
			}
		})
	}
}
