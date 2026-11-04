package store

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestSimplePublicationPreservesBorrowedFacetsAndOldReaders(t *testing.T) {
	for _, propertyWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "scalar", true: "property"}[propertyWrite], func(t *testing.T) {
			parents, children, contents := []types.ObjID{1}, []types.ObjID{2}, []types.ObjID{3}
			order := []string{"p"}
			props := map[string]Property{"p": NewProperty(types.NewInt(7), 0, PropRead|PropWrite, false, true)}
			b := NewObjectBuilder(0)
			b.SetName("before")
			b.SetParents(parents)
			b.SetChildren(children)
			b.SetContents(contents)
			b.SetPropDefsCount(1)
			b.ResetProperties(props, order)
			s := NewStore()
			if err := s.Add(b.Build()); err != nil {
				t.Fatal(err)
			}
			old := s.BeginSnapshot(0)
			defer old.Release()
			// Change the original admission backing before the first publication.
			parents[0], children[0], contents[0], order[0] = 99, 99, 99, "external"
			props["p"] = NewProperty(types.NewInt(99), 0, PropRead, false, true)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			var ec types.ErrorCode
			if propertyWrite {
				ec = tx.SetPropertyValue(0, "p", types.NewInt(8))
			} else {
				ec = tx.SetObjectName(0, "after")
			}
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.Commit(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			for _, image := range []*Object{old.object(0), s.load(0)} {
				if !reflect.DeepEqual(image.parents, []types.ObjID{1}) || !reflect.DeepEqual(image.children, []types.ObjID{2}) || !reflect.DeepEqual(image.contents, []types.ObjID{3}) || !reflect.DeepEqual(image.propOrder, []string{"p"}) {
					t.Fatal("admitted facet backing changed")
				}
			}
			if value, ec := old.PropertyValue(0, "p"); ec != types.E_NONE || value.Int() != 7 {
				t.Fatalf("old property = %v, %v; want 7", value, ec)
			}
			if name, ec := old.ObjectName(0); ec != types.E_NONE || name != "before" {
				t.Fatalf("old name = %q, %v; want before", name, ec)
			}
			want := int64(7)
			if propertyWrite {
				want = 8
			}
			if value, ec := s.DirectTxn().PropertyValue(0, "p"); ec != types.E_NONE || value.Int() != want {
				t.Fatalf("live property = %v, %v; want %d", value, ec, want)
			}
		})
	}
}

func TestLiveDefinitionFlushKeepsScalarDescendantHistory(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	child, ec := s.DirectTxn().CreateObject(nil, 0)
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.SetObjectName(child, "after"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.DefineProperty(0, "new", NewProperty(types.NewInt(9), 0, PropRead, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	// This descendant joins after definition staging, so its only staged write
	// is scalar. The live inheritance walk must own its property table too.
	if ec := s.ChangeParents(child, []types.ObjID{0}); ec != types.E_NONE {
		t.Fatal(ec)
	}
	old := s.BeginSnapshot(0)
	defer old.Release()
	if ec := tx.FlushStagedToLive(); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if _, ec := old.FindProperty(child, "new"); ec != types.E_PROPNF {
		t.Fatalf("old descendant sees new definition: %v", ec)
	}
	if value, ec := s.DirectTxn().PropertyValue(child, "new"); ec != types.E_NONE || value.Int() != 9 {
		t.Fatalf("live inherited value = %v, %v; want 9", value, ec)
	}
}
