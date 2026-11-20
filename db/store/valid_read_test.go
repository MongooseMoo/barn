package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestValidCheckConflictsWithConcurrentRecycle: one task records that x is valid
// by writing y.flag; another recycles x only while y.flag is unset. Each serial
// order leaves exactly one of "x recycled" and "y.flag set". Run from one
// snapshot, the recycle commits first, so the valid() task must fail validation.
func TestValidCheckConflictsWithConcurrentRecycle(t *testing.T) {
	checks := map[string]func(tx *StoreTxn, id types.ObjID) bool{
		"Valid":        (*StoreTxn).Valid,
		"ObjectExists": func(tx *StoreTxn, id types.ObjID) bool { return tx.ObjectExists(id) == types.E_NONE },
	}
	for name, exists := range checks {
		t.Run(name, func(t *testing.T) {
			s, ids := immutFixture(t, 2)
			x, y := ids[0], ids[1]
			if ec := s.DirectTxn().DefineProperty(y, "flag", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatalf("DefineProperty: %v", ec)
			}

			checker := s.BeginSnapshot(0)
			defer checker.Release()
			recycler := s.BeginSnapshot(0)
			defer recycler.Release()

			if !exists(checker, x) {
				t.Fatalf("checker: x not valid at snapshot")
			}
			if ec := checker.SetPropertyValue(y, "flag", types.NewInt(1)); ec != types.E_NONE {
				t.Fatalf("checker SetPropertyValue: %v", ec)
			}

			flag, ec := recycler.PropertyValue(y, "flag")
			if ec != types.E_NONE || !flag.Equal(types.NewInt(0)) {
				t.Fatalf("recycler y.flag = %v, %v; want 0", flag, ec)
			}
			if handled, ec := recycler.RecycleObject(x); !handled || ec != types.E_NONE {
				t.Fatalf("RecycleObject: handled=%v ec=%v", handled, ec)
			}
			if ec := recycler.Commit(); ec != types.E_NONE {
				t.Fatalf("recycler commit: %v", ec)
			}

			if ec := checker.Commit(); ec == types.E_NONE {
				flag, _ := s.DirectTxn().PropertyValue(y, "flag")
				t.Fatalf("checker committed after concurrent recycle: x recycled=%v, y.flag=%v; no serial order gives both",
					s.load(x).recycled, flag)
			}
		})
	}
}
