package vm

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// The Jepsen campaign's keyed-map rows (scripts/jepsen_checkpoint.py
// keyed_code) must reach the store as keyed accesses, not whole reads of the
// map. The fixture holds the MOO the adapter generates, pinned by
// scripts/test_jepsen_checkpoint.py; its layout puts the keyed parent at #11
// and the holder with its own map at #12.
func TestJepsenKeyedRowsAreKeyedAccesses(t *testing.T) {
	data, err := os.ReadFile("../scripts/testdata/jepsen_keyed_txns.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name       string
			BothCommit bool `json:"both_commit"`
			Entries    map[string][]int64
			Txns       []struct{ Code string }
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if len(c.Txns) != 2 {
				t.Fatalf("%d transactions; want 2", len(c.Txns))
			}
			store := jepsenKeyedStore(t)
			ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
			defer ta.Release()
			defer tb.Release()
			runOn(t, store, ta, c.Txns[0].Code)
			runOn(t, store, tb, c.Txns[1].Code)
			if ec := ta.Commit(); ec != types.E_NONE {
				t.Fatalf("first commit: %v %v", ec, ta.Conflicts())
			}
			ec := tb.Commit()
			if !c.BothCommit {
				if ec == types.E_NONE || !tb.ValidationFailed() {
					t.Fatalf("second commit = %v; want a lost validation", ec)
				}
				return
			}
			if ec != types.E_NONE {
				t.Fatalf("second commit: %v %v", ec, tb.Conflicts())
			}
			m, code := store.DirectTxn().PropertyValue(12, "jm")
			if code != types.E_NONE {
				t.Fatal(code)
			}
			for key, want := range c.Entries {
				k, err := strconv.ParseInt(key, 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				elements := make([]types.Value, len(want))
				for i, v := range want {
					elements[i] = types.NewInt(v)
				}
				if got, ok := m.MapGet(types.NewInt(k)); !ok || !got.Equal(types.NewList(elements)) {
					t.Fatalf("#12.jm[%d] = %v (present %v); want %v", k, got, ok, want)
				}
			}
		})
	}
}

// jepsenKeyedStore is the campaign's keyed layout: parent #11 defines jm, #12
// is a child holding its own empty map and #13 a child whose slot is clear.
func jepsenKeyedStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 11} {
		b := dbstore.NewObjectBuilder(id)
		b.SetOwner(0)
		b.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagFertile)
		if id == 11 {
			b.SetProperty("jm", dbstore.NewProperty(types.NewEmptyMap(), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if err := store.Add(b.Build()); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []types.ObjID{12, 13} {
		id, code := store.DirectTxn().CreateObject([]types.ObjID{11}, 0)
		if code != types.E_NONE || id != want {
			t.Fatalf("created %v (%v); want #%d", id, code, want)
		}
	}
	if code := store.DirectTxn().SetPropertyValue(12, "jm", types.NewEmptyMap()); code != types.E_NONE {
		t.Fatal(code)
	}
	return store
}
