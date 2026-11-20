package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// The renewed transaction carries a keyed read on the map the boundary
// republished as a keyed read, not a whole one: its own publication is no
// conflict, a later change to the entry it read is.
func TestBoundaryRenewCarriesKeyedReadsOnRepublishedMap(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(map[bool]string{false: "own-publication", true: "later-conflict"}[change], func(t *testing.T) {
			store := mapKeyStore(t, kv("a", 1), kv("b", 2))
			if ec := store.DirectTxn().DefineProperty(0, "later", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx := store.BeginSnapshot(0)
			if value, present := keyedRead(t, tx, "a"); !present || value.Int() != 1 {
				t.Fatalf("keyed observation=%v present=%v", value, present)
			}
			keyedWrite(t, tx, "b", 3)
			next, published, ec := tx.CommitAndRenewCarryingReads()
			if ec != types.E_NONE || !published {
				t.Fatalf("renew=%v published=%v", ec, published)
			}
			defer next.Release()
			if _, whole := next.propertyReads[propertyReadKey{objID: 0, name: "m"}]; len(next.mapKeys) != 1 || whole {
				t.Fatalf("keyed dependency not carried keyed: keyed=%d whole=%v", len(next.mapKeys), whole)
			}
			if change {
				writer := store.BeginSnapshot(0)
				defer writer.Release()
				keyedWrite(t, writer, "a", 4)
				if ec := writer.Commit(); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			if ec := next.SetPropertyValue(0, "later", types.NewInt(5)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			ec = next.Commit()
			if (!change && ec != types.E_NONE) || (change && (ec != types.E_INVARG || !next.ValidationFailed())) {
				t.Fatalf("change=%v commit=%v validationFailure=%v", change, ec, next.ValidationFailed())
			}
		})
	}
}
