package format

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Toast keeps every dumped slot, recycled ones included, in num_objects after
// a restart, so max_object() stays at the highest slot and create() does not
// hand out a recycled id again.
func TestLoadKeepsTrailingRecycledSlotsInMaxObject(t *testing.T) {
	s := store.NewStore()
	for id := types.ObjID(0); id <= 2; id++ {
		if err := s.Add(store.NewObject(id, 2)); err != nil {
			t.Fatal(err)
		}
	}
	x, code := s.DirectTxn().CreateObject(nil, 2)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	y, code := s.DirectTxn().CreateObject(nil, 2)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	if x != 3 || y != 4 {
		t.Fatalf("created %d, %d; want 3, 4", x, y)
	}
	for _, id := range []types.ObjID{x, y} {
		if err := s.Recycle(id); err != nil {
			t.Fatal(err)
		}
	}

	var dump bytes.Buffer
	if err := NewWriter(&dump, s.Snapshot()).WriteDatabase(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := parseDatabaseWithWaifIdentities(bufio.NewReader(&dump), nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}

	if got := loaded.DirectTxn().MaxObject(); got != y {
		t.Fatalf("max_object after reload = #%d, want #%d", got, y)
	}
	next, code := loaded.DirectTxn().CreateObject(nil, 2)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	if next != y+1 {
		t.Fatalf("create after reload = #%d, want #%d", next, y+1)
	}
}
