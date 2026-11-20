package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A property both parents inherit from a common ancestor and override: vc's
// own slot is clear, so vc.q resolves to va's value and, after a reparent, to
// vb's. A transaction that read vc.q by walking, then wrote, must not commit
// after another commit reparented vc.
func TestPropertyWalkConflictsWithConcurrentChparent(t *testing.T) {
	for _, start := range []types.ObjID{probeVC, probeChild} {
		t.Run(map[types.ObjID]string{probeVC: "receiver", probeChild: "descendant"}[start], func(t *testing.T) {
			s := chparentProbeStore(t)
			direct := s.DirectTxn()
			if ec := direct.DefineProperty(probeRoot, "q", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatalf("DefineProperty: %v", ec)
			}
			for id, value := range map[types.ObjID]int64{probeVA: 1, probeVB: 2} {
				if ec := direct.SetPropertyValue(id, "q", types.NewInt(value)); ec != types.E_NONE {
					t.Fatalf("SetPropertyValue #%d: %v", id, ec)
				}
			}

			tx := s.BeginSnapshot(0)
			defer tx.Release()
			if got, ec := tx.PropertyValue(start, "q"); ec != types.E_NONE || !got.Equal(types.NewInt(1)) {
				t.Fatalf("#%d.q = %v (%v), want 1", start, got, ec)
			}
			stageLogWrite(t, tx)

			reparentVC(t, s)

			after := s.BeginSnapshot(0)
			got, ec := after.PropertyValue(start, "q")
			after.Release()
			if ec != types.E_NONE || !got.Equal(types.NewInt(2)) {
				t.Fatalf("after the reparent #%d.q = %v (%v), want 2", start, got, ec)
			}
			if ec := tx.Commit(); ec == types.E_NONE {
				t.Fatalf("commit succeeded after a concurrent chparent changed what #%d.q resolves to", start)
			}
		})
	}
}
