package store

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/types"
)

var publicationArgsA = VerbArgs{This: "this", Prep: "none", That: "none"}
var publicationArgsB = VerbArgs{This: "any", Prep: "with", That: "any"}

func newVerbArgsPublicationStore(t testing.TB, count, lines int) *Store {
	t.Helper()
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("v%03d", i)
		code := make([]string, lines)
		for j := range code {
			code[j] = fmt.Sprintf("x = %d;", j)
		}
		verb := NewVerb(name, []string{name, "alias_" + name, "wild_" + name + "*"}, 0,
			VerbRead|VerbExecute|VerbDebug, publicationArgsA, code)
		if _, ec := s.AddVerb(0, verb); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	return s
}

func publicationVerb(t testing.TB, s *Store, index int) VerbView {
	t.Helper()
	v, ec := s.DirectTxn().VerbByIndex(0, index)
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	return v
}

func ownPublicationView(v VerbView) VerbView {
	v.Names = append([]string(nil), v.Names...)
	v.Code = append([]string(nil), v.Code...)
	return v
}

func requirePublicationView(t testing.TB, got, want VerbView) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verb view changed: got %#v, want %#v", got, want)
	}
	if got.CodeKey != sourcekey.Of(got.Code) {
		t.Fatal("code key no longer describes source")
	}
}

func TestVerbArgsPublicationPreservesOldReadersAndRetainedViews(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 2, 3)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	old, _, err := tx.FindVerb(0, "v000")
	if err != nil {
		t.Fatal(err)
	}
	wantOld := ownPublicationView(old)
	other := publicationVerb(t, s, 1)
	wantOther := ownPublicationView(other)
	if ec := s.SetVerbArgs(0, "alias_v000", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	warmed := publicationVerb(t, s, 0)
	wantWarmed := ownPublicationView(warmed)
	if warmed.ArgSpec != publicationArgsB {
		t.Fatal("updated argument specification not visible")
	}
	if ec := s.DirectTxn().SetVerbCode(0, "v000", []string{"return 73;"}); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.SetVerbInfo(0, "v000", 0, VerbRead|VerbExecute, []string{"renamed", "ren*amed"}); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.SetVerbArgs(0, "renamed", publicationArgsA); ec != types.E_NONE {
		t.Fatal(ec)
	}
	requirePublicationView(t, old, wantOld)
	requirePublicationView(t, warmed, wantWarmed)
	requirePublicationView(t, other, wantOther)
	fromOldSnapshot, _, err := tx.FindVerb(0, "v000")
	if err != nil {
		t.Fatal(err)
	}
	requirePublicationView(t, fromOldSnapshot, wantOld)
}

func TestVerbArgsPublicationPreservesDefinitionOrder(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 0, 0)
	for _, names := range [][]string{{"look", "l*ook", "glance"}, {"look", "glance", "other*"}} {
		index := len(s.load(0).verbList)
		if _, ec := s.AddVerb(0, NewVerb("look", names, 0, VerbRead|VerbExecute,
			publicationArgsA, []string{fmt.Sprintf("return %d;", index)})); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	second := ownPublicationView(publicationVerb(t, s, 1))
	if ec := s.SetVerbArgs(0, "glance", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	for _, name := range []string{"look", "glance", "lo"} {
		v, _, err := s.DirectTxn().FindCallableVerb(0, name)
		if err != nil || v.ArgSpec != publicationArgsB || len(v.Code) != 1 || v.Code[0] != "return 0;" {
			t.Fatalf("definition-order lookup %q = %#v, %v", name, v, err)
		}
	}
	requirePublicationView(t, publicationVerb(t, s, 1), second)
}

func TestVerbArgsPublicationErrorsStampsHistoryAndSameArgs(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 2, 1)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	old := tx.object(0)
	clock, shape := s.clock.Load(), s.verbShapeChangeTS.Load()
	history := len(s.history[0])
	for _, tc := range []struct {
		id   types.ObjID
		name string
		want types.ErrorCode
	}{{999, "v000", types.E_INVIND}, {0, "absent", types.E_VERBNF}} {
		if ec := s.SetVerbArgs(tc.id, tc.name, publicationArgsB); ec != tc.want {
			t.Fatalf("error = %s, want %s", ec, tc.want)
		}
		if s.load(0) != old || s.clock.Load() != clock || s.verbShapeChangeTS.Load() != shape || len(s.history[0]) != history {
			t.Fatal("failed operation published or changed generation/history")
		}
	}
	before := ownPublicationView(old.verbList[0].View())
	if ec := s.SetVerbArgs(0, "v000", publicationArgsA); ec != types.E_NONE {
		t.Fatal(ec)
	}
	fresh := s.load(0)
	if fresh == old || fresh.verbVersion <= old.verbVersion || fresh.verbList[0].version <= old.verbList[0].version || s.verbShapeChangeTS.Load() <= shape {
		t.Fatal("successful same-args update lost publication or generation changes")
	}
	if fresh.scalarVersion != old.scalarVersion || fresh.relationshipVersion != old.relationshipVersion || fresh.propertyVersion != old.propertyVersion || fresh.verbList[1].version != old.verbList[1].version {
		t.Fatal("argument-only publication changed unrelated generations")
	}
	found := false
	for _, entry := range s.history[0] {
		found = found || entry.obj == old
	}
	if !found {
		t.Fatal("live old reader lost its retained history image")
	}
	requirePublicationView(t, old.verbList[0].View(), before)
	requirePublicationView(t, publicationVerb(t, s, 0), before)
}

func TestVerbArgsPublicationDetachesBorrowedVerbInputs(t *testing.T) {
	s := NewStore()
	initial := NewObjectBuilder(0)
	published := false
	var names, code [][]string
	add := func(i int) {
		n := []string{fmt.Sprintf("v%03d", i), fmt.Sprintf("alias_%d", i)}
		c := []string{fmt.Sprintf("return %d;", i)}
		names, code = append(names, n), append(code, c)
		verb := NewVerb(n[0], n, 0, VerbRead|VerbExecute, publicationArgsA, c)
		if !published {
			initial.AppendVerb(verb)
		} else if _, ec := s.AddVerb(0, verb); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	add(0)
	add(1)
	// Both selected and nonselected nodes still carry borrowed inputs at the
	// first publication boundary; no intervening AddVerb has detached either.
	if err := s.Add(initial.Build()); err != nil {
		t.Fatal(err)
	}
	published = true
	if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	for i := range names {
		want := ownPublicationView(publicationVerb(t, s, i))
		names[i][0], code[i][0] = "external-name", "return 999;"
		requirePublicationView(t, publicationVerb(t, s, i), want)
	}
	// Warm publication must not cause a subsequently admitted borrowed node to
	// inherit any ownership assumption about the earlier collection.
	add(2)
	if ec := s.SetVerbArgs(0, "v000", publicationArgsA); ec != types.E_NONE {
		t.Fatal(ec)
	}
	want := ownPublicationView(publicationVerb(t, s, 2))
	names[2][0], code[2][0] = "later-external-name", "return 1000;"
	requirePublicationView(t, publicationVerb(t, s, 2), want)
}

func TestVerbArgsPublicationDetachesBorrowedObjectFacets(t *testing.T) {
	s := NewStore()
	parents, children, contents := []types.ObjID{1}, []types.ObjID{2}, []types.ObjID{3}
	order := []string{"p"}
	for _, id := range []types.ObjID{0, 1, 2, 3} {
		b := NewObjectBuilder(id)
		b.SetOwner(0)
		b.SetLocation(types.ObjNothing)
		switch id {
		case 0:
			b.SetParents(parents)
			b.SetChildren(children)
			b.SetContents(contents)
			b.SetProperty("p", NewProperty(types.NewInt(7), 0, PropRead|PropWrite, false, true))
			b.SetPropDefsCount(1)
			b.SetPropOrder(order)
			b.AppendVerb(NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, publicationArgsA, nil))
		case 1:
			b.SetChildren([]types.ObjID{0})
		case 2:
			b.SetParents([]types.ObjID{0})
		case 3:
			b.SetLocation(0)
		}
		if err := s.Add(b.Build()); err != nil {
			t.Fatal(err)
		}
	}
	if ec := s.SetVerbArgs(0, "look", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	parents[0], children[0], contents[0], order[0] = 99, 99, 99, "external-property"
	obj := s.load(0)
	if !reflect.DeepEqual(obj.parents, []types.ObjID{1}) || !reflect.DeepEqual(obj.children, []types.ObjID{2}) || !reflect.DeepEqual(obj.contents, []types.ObjID{3}) || !reflect.DeepEqual(obj.propOrder, []string{"p"}) {
		t.Fatal("argument publication retained borrowed non-verb facet arrays")
	}
}

func TestVerbArgsPublicationAdoptionPreservesStagedCodeAndReadValidation(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict=%v", conflict), func(t *testing.T) {
			s := newVerbArgsPublicationStore(t, 2, 1)
			if err := s.Add(NewObject(1, 0)); err != nil {
				t.Fatal(err)
			}
			if ec := s.DirectTxn().DefineProperty(1, "guard", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			if _, ec := tx.PropertyValue(1, "guard"); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.SetVerbCode(0, "v000", []string{"return 73;"}); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx.MarkLiveMutated()
			if ec := tx.AdoptLiveVerbs(0); ec != types.E_NONE {
				t.Fatal(ec)
			}
			v, _, err := tx.FindCallableVerb(0, "alias_v000")
			if err != nil || v.ArgSpec != publicationArgsB || v.Code[0] != "return 73;" || v.CodeKey != sourcekey.Of(v.Code) {
				t.Fatalf("adopted/staged view = %#v, %v", v, err)
			}
			if conflict {
				if ec := s.DirectTxn().SetPropertyValue(1, "guard", types.NewInt(1)); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			result := tx.Commit()
			if conflict {
				if result != types.E_INVARG || !tx.ValidationFailed() || publicationVerb(t, s, 0).Code[0] == "return 73;" {
					t.Fatal("facet adoption hid unrelated conflict or published rejected code")
				}
			} else if result != types.E_NONE || publicationVerb(t, s, 0).Code[0] != "return 73;" {
				t.Fatal("staged code did not commit after facet adoption")
			}
		})
	}
}
