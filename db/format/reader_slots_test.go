package format

import (
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// loadedObject returns a builder in the state the object reader leaves it:
// own definitions named, slots held by position. Slot i holds the int i and is
// owned by the object.
func loadedObject(id types.ObjID, parents []types.ObjID, definitions []string, slotCount int) *store.ObjectBuilder {
	obj := store.NewObjectBuilder(id)
	obj.SetParents(parents)
	obj.SetPropDefsCount(len(definitions))
	obj.SetPropOrder(definitions)
	var slots store.LoadedSlots
	for i := range slotCount {
		slots.Append(store.NewProperty(types.NewInt(int64(i)), id, store.PropRead, false, i < len(definitions)))
	}
	obj.SetLoadedSlots(&slots)
	return obj
}

func requireSlots(t *testing.T, obj *store.ObjectBuilder, want map[string]int64) {
	t.Helper()
	for name, value := range want {
		prop, ok := obj.Property(name)
		if !ok {
			t.Errorf("#%d has no property %q", obj.ID(), name)
			continue
		}
		if prop.Value.Int() != value {
			t.Errorf("#%d.%s = %v, want slot %d", obj.ID(), name, prop.Value, value)
		}
	}
	if got := obj.PropertyCount(); got != len(want) {
		t.Errorf("#%d has %d properties, want %d", obj.ID(), got, len(want))
	}
}

func TestResolvePropertyNamesNamesSlotsFromTheAncestry(t *testing.T) {
	database := &Database{Objects: map[types.ObjID]*store.ObjectBuilder{
		1: loadedObject(1, nil, []string{"a", "b"}, 2),
		2: loadedObject(2, []types.ObjID{1}, []string{"c"}, 3),
		// An instance: no definitions, one parent, as many slots as the parent.
		3: loadedObject(3, []types.ObjID{2}, nil, 3),
		// Fewer slots than the ancestry defines: the leading names apply.
		4: loadedObject(4, []types.ObjID{2}, nil, 2),
		// More slots than the ancestry defines: the rest keep placeholders.
		5: loadedObject(5, []types.ObjID{2}, nil, 4),
		// Two parents: self, then each parent's ancestry, each object once.
		6: loadedObject(6, []types.ObjID{2, 1}, []string{"d"}, 4),
		// A parent loop must not hang, and resolves to placeholders.
		7: loadedObject(7, []types.ObjID{8}, nil, 1),
		8: loadedObject(8, []types.ObjID{7}, nil, 1),
	}}
	database.AnonymousObjs = []*store.ObjectBuilder{loadedObject(3, []types.ObjID{2}, nil, 3)}

	database.resolvePropertyNames()

	requireSlots(t, database.Objects[1], map[string]int64{"a": 0, "b": 1})
	requireSlots(t, database.Objects[2], map[string]int64{"c": 0, "a": 1, "b": 2})
	requireSlots(t, database.Objects[3], map[string]int64{"c": 0, "a": 1, "b": 2})
	requireSlots(t, database.Objects[4], map[string]int64{"c": 0, "a": 1})
	requireSlots(t, database.Objects[5], map[string]int64{"c": 0, "a": 1, "b": 2, "_inherited_3": 3})
	requireSlots(t, database.Objects[6], map[string]int64{"d": 0, "c": 1, "a": 2, "b": 3})
	requireSlots(t, database.Objects[7], map[string]int64{"_inherited_0": 0})
	requireSlots(t, database.Objects[8], map[string]int64{"_inherited_0": 0})
	requireSlots(t, database.AnonymousObjs[0], map[string]int64{"c": 0, "a": 1, "b": 2})
}
