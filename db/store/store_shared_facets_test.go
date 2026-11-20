package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// imageFacets is a deep copy of what a staged write must never change in a
// published image: the property slots, the order, the relationship lists and
// the verb nodes.
type imageFacets struct {
	props    map[string]Property
	order    []string
	contents []types.ObjID
	children []types.ObjID
	verbs    []Verb
}

func facetsOf(obj *Object) imageFacets {
	f := imageFacets{
		props:    make(map[string]Property),
		order:    slices.Clone(obj.propOrder),
		contents: slices.Clone(obj.contents),
		children: slices.Clone(obj.children),
	}
	for name, prop := range obj.properties.all() {
		f.props[name] = prop
	}
	for _, verb := range obj.verbList {
		v := *verb
		v.code = slices.Clone(verb.code)
		f.verbs = append(f.verbs, v)
	}
	return f
}

func sameFacets(a, b imageFacets) bool {
	if len(a.props) != len(b.props) || !slices.Equal(a.order, b.order) ||
		!slices.Equal(a.contents, b.contents) || !slices.Equal(a.children, b.children) ||
		len(a.verbs) != len(b.verbs) {
		return false
	}
	for name, prop := range a.props {
		if other, ok := b.props[name]; !ok || other.version != prop.version || !other.value.Identical(prop.value) || other.clear != prop.clear {
			return false
		}
	}
	for i := range a.verbs {
		if !slices.Equal(a.verbs[i].code, b.verbs[i].code) || a.verbs[i].id != b.verbs[i].id {
			return false
		}
	}
	return true
}

// Two transactions' private copies of one image must not share the spare
// capacity of its relationship lists: an append into one would overwrite what
// the other appended.
func TestPrivateCopiesDoNotShareListCapacity(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	room, _ := d.CreateObject([]types.ObjID{0}, 0)
	var things []types.ObjID
	for range 5 {
		thing, _ := d.CreateObject([]types.ObjID{0}, 0)
		things = append(things, thing)
	}
	// Grow the room's contents by appends until the published slice has spare
	// capacity, which is what two private copies could both append into.
	inRoom := 0
	for ; inRoom < 3; inRoom++ {
		if ec := d.MoveObject(things[inRoom], room, 0); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if live := s.load(room); cap(live.contents) > len(live.contents) {
			inRoom++
			break
		}
	}
	if live := s.load(room); cap(live.contents) == len(live.contents) {
		t.Fatalf("setup: published contents %v have no spare capacity", live.contents)
	}
	first, second := things[inRoom], things[inRoom+1]
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	if ec := ta.MoveObject(first, room, 0); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tb.MoveObject(second, room, 0); ec != types.E_NONE {
		t.Fatal(ec)
	}
	want := slices.Clone(things[:inRoom])
	if contents, _ := ta.Contents(room); !slices.Equal(contents, append(slices.Clone(want), first)) {
		t.Errorf("first txn's view of the room = %v, want %v then #%d", contents, want, first)
	}
	if contents, _ := tb.Contents(room); !slices.Equal(contents, append(slices.Clone(want), second)) {
		t.Errorf("second txn's view of the room = %v, want %v then #%d", contents, want, second)
	}

	parent, _ := d.CreateObject([]types.ObjID{0}, 0)
	for range 4 {
		if _, ec := d.CreateObject([]types.ObjID{parent}, 0); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if live := s.load(parent); cap(live.children) > len(live.children) {
			break
		}
	}
	if live := s.load(parent); cap(live.children) == len(live.children) {
		t.Fatalf("setup: published children %v have no spare capacity", live.children)
	}
	tc, td := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer tc.Release()
	defer td.Release()
	c1, _ := tc.CreateObject([]types.ObjID{parent}, 0)
	c2, _ := td.CreateObject([]types.ObjID{parent}, 0)
	if children, _ := tc.Children(parent); !slices.Contains(children, c1) || slices.Contains(children, c2) {
		t.Errorf("first txn's children of #%d = %v, want #%d and not #%d", parent, children, c1, c2)
	}
	if children, _ := td.Children(parent); !slices.Contains(children, c2) || slices.Contains(children, c1) {
		t.Errorf("second txn's children of #%d = %v, want #%d and not #%d", parent, children, c2, c1)
	}
}

// A transaction's private copy shares its image's facets until a write changes
// one. Every staged write must leave the published images exactly as they were.
func TestStagedWritesLeavePublishedImagesUnchanged(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	room, _ := d.CreateObject([]types.ObjID{0}, 0)
	parent, _ := d.CreateObject([]types.ObjID{0}, 0)
	child, _ := d.CreateObject([]types.ObjID{parent}, 0)
	anon, _ := d.CreateObject([]types.ObjID{parent}, 0, true)
	thing, _ := d.CreateObject([]types.ObjID{0}, 0)
	for _, ec := range []types.ErrorCode{
		d.DefineProperty(parent, "p", NewProperty(types.NewInt(1), 0, PropRead, false, true)),
		d.DefineProperty(parent, "gone", NewProperty(types.NewInt(1), 0, PropRead, false, true)),
		d.DefineProperty(parent, "m", NewProperty(types.NewMap([][2]types.Value{{types.NewStr("k"), types.NewInt(1)}}), 0, PropRead, false, true)),
		d.SetPropertyValue(child, "p", types.NewInt(2)),
		d.MoveObject(thing, room, 0),
	} {
		if ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	for _, id := range []types.ObjID{parent, child} {
		if _, ec := s.AddVerb(id, NewVerb([]string{"v"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 1;"})); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}

	ids := []types.ObjID{room, parent, child, thing}
	before := make(map[types.ObjID]imageFacets)
	for _, id := range ids {
		before[id] = facetsOf(s.load(id))
	}
	anonBefore := facetsOf(s.anonObjects[anon])

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	steps := []struct {
		name string
		run  func() types.ErrorCode
	}{
		{"SetPropertyValue", func() types.ErrorCode { return tx.SetPropertyValue(child, "p", types.NewInt(3)) }},
		{"SetPropertyInfo", func() types.ErrorCode { return tx.SetPropertyInfo(child, "p", nil, new(PropertyPerms)) }},
		{"ClearPropertyOverride", func() types.ErrorCode { return tx.ClearPropertyOverride(child, "p") }},
		{"DefineProperty", func() types.ErrorCode {
			return tx.DefineProperty(parent, "q", NewProperty(types.NewInt(4), 0, PropRead, false, true))
		}},
		{"DeleteDefinedProperty", func() types.ErrorCode { return tx.DeleteDefinedProperty(parent, "gone") }},
		{"MapKeyWrite", func() types.ErrorCode {
			view, _, ec := tx.MapPropertyForKey(parent, "m", true)
			if ec != types.E_NONE {
				return ec
			}
			next := view.Value.MapSet(types.NewStr("k"), types.NewInt(5))
			return tx.SetPropertyMapKey(parent, "m", types.NewStr("k"), next, 0)
		}},
		{"MoveObject", func() types.ErrorCode { return tx.MoveObject(child, room, 0) }},
		{"CreateObject", func() types.ErrorCode { _, ec := tx.CreateObject([]types.ObjID{room}, 0); return ec }},
		{"SetVerbCode", func() types.ErrorCode { return tx.SetVerbCode(parent, "v", []string{"return 2;"}) }},
		{"SetVerbCodeByIndex", func() types.ErrorCode { return tx.SetVerbCodeByIndex(child, 0, []string{"return 3;"}) }},
		{"DeleteResolvedVerb", func() types.ErrorCode {
			resolved, err := tx.ResolveVerbOnObject(child, "v")
			if err != nil {
				return types.E_VERBNF
			}
			return tx.DeleteResolvedVerb(resolved)
		}},
	}
	for _, step := range steps {
		if ec := step.run(); ec != types.E_NONE {
			t.Fatalf("%s: %s", step.name, ec)
		}
		for _, id := range ids {
			if !sameFacets(before[id], facetsOf(s.load(id))) {
				t.Fatalf("%s changed the published image of #%d", step.name, id)
			}
		}
		if !sameFacets(anonBefore, facetsOf(s.anonObjects[anon])) {
			t.Fatalf("%s changed the live anonymous object #%d", step.name, anon)
		}
	}
}
