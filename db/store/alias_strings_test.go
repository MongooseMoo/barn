package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func aliasStrings(s *Store, id types.ObjID) []string {
	var got []string
	s.VisitAliasStrings(id, func(alias string) bool {
		got = append(got, alias)
		return true
	})
	return got
}

// A clear aliases slot reads as the value it inherits now, as every other
// property read does, so changing the parent's aliases changes the child's.
func TestVisitAliasStringsResolvesClearSlotThroughAncestors(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	aliases := func(names ...string) types.Value {
		values := make([]types.Value, len(names))
		for i, name := range names {
			values[i] = types.NewStr(name)
		}
		return types.NewList(values)
	}
	tx := s.DirectTxn()
	if code := tx.DefineProperty(0, "aliases", NewProperty(aliases("old"), 0, PropRead|PropWrite, false, true)); code != types.E_NONE {
		t.Fatalf("DefineProperty: %v", code)
	}
	child, code := tx.CreateObject([]types.ObjID{0}, 0)
	if code != types.E_NONE {
		t.Fatalf("CreateObject: %v", code)
	}
	if code := tx.SetPropertyValue(0, "aliases", aliases("new", "newer")); code != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", code)
	}

	if got, want := aliasStrings(s, child), []string{"new", "newer"}; !slices.Equal(got, want) {
		t.Fatalf("child aliases = %q, want %q", got, want)
	}
}
