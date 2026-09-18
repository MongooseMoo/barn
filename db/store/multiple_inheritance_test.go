package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestRenumberParentPreservesMultipleParentTopology(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	create := func(name string) types.ObjID {
		t.Helper()
		id, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject %s failed: %v", name, errCode)
		}
		return id
	}

	hole := create("hole")
	left := create("left parent")
	right := create("right parent")
	child, errCode := store.DirectTxn().CreateObject([]types.ObjID{left, right}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	if err := store.Recycle(hole); err != nil {
		t.Fatalf("Recycle hole failed: %v", err)
	}
	if err := store.Renumber(right, hole); err != nil {
		t.Fatalf("Renumber right parent failed: %v", err)
	}

	parents, errCode := store.DirectTxn().Parents(child)
	if errCode != types.E_NONE {
		t.Fatalf("Parents child failed: %v", errCode)
	}
	if want := []types.ObjID{left, hole}; !slices.Equal(parents, want) {
		t.Fatalf("Parents child = %v, want %v", parents, want)
	}

	children, errCode := store.DirectTxn().Children(hole)
	if errCode != types.E_NONE {
		t.Fatalf("Children renumbered parent failed: %v", errCode)
	}
	if !slices.Contains(children, child) {
		t.Fatalf("Children renumbered parent = %v, want child %d", children, child)
	}
	if store.DirectTxn().Valid(right) {
		t.Fatalf("old parent id %d remains valid after renumber", right)
	}
}
