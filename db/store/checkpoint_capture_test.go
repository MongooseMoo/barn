package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointCaptureHistoricalGraph(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	w := types.NewWaif(0, 0)
	w.SetProperty("n", types.NewInt(1))
	anon := NewObject(1, 0)
	anon.anonymous = true
	s.AddAnonymous(anon)
	if ec := s.DirectTxn().DefineProperty(1, "w", NewProperty(w, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.DirectTxn().DefineProperty(0, "a", NewProperty(types.NewAnon(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	c := s.CaptureCheckpoint(nil)
	defer c.Close()
	done := make(chan struct{})
	go func() {
		if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(2)); ec != types.E_NONE {
			t.Error(ec)
		}
		if ec := s.DirectTxn().SetPropertyValue(0, "a", types.NewInt(0)); ec != types.E_NONE {
			t.Error(ec)
		}
		if err := s.Recycle(1); err != nil {
			t.Error(err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("capture blocks coarse writes")
	}
	snap, _, err := c.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.AnonymousObjects) != 1 {
		t.Fatalf("anonymous count %d", len(snap.AnonymousObjects))
	}
	got, ok := snap.AnonymousObjects[0].Properties["w"].Value.GetProperty("n")
	if !ok || got.Int() != 1 {
		t.Fatalf("historical WAIF value %v, found %v", got, ok)
	}
	c.Close()
	c.Close()
	if n := s.activeFloorCount(); n != 0 {
		t.Fatalf("retained floors %d", n)
	}
}

func TestCheckpointCaptureBoundsAndOrdinaryCommit(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().DefineProperty(0, "n", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	c := s.CaptureCheckpoint(nil)
	defer c.Close()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.SetPropertyValue(0, "n", types.NewInt(2)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if err := s.Add(NewObject(dirSegSize+1, 0)); err != nil {
		t.Fatal(err)
	}
	snap, _, err := c.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if snap.MaxObject != 0 || len(snap.Objects) != 1 || snap.Objects[0].Properties["n"].Value.Int() != 1 {
		t.Fatalf("captured bounds/state: max=%d objects=%d n=%v", snap.MaxObject, len(snap.Objects), snap.Objects[0].Properties["n"].Value)
	}
}

func TestCheckpointMaterializeAllowsWritersDuringWalk(t *testing.T) {
	for _, coarse := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "coarse"}[coarse], func(t *testing.T) {
			s := NewStore()
			for id := types.ObjID(0); id < 2; id++ {
				if err := s.Add(NewObject(id, 0)); err != nil {
					t.Fatal(err)
				}
				if ec := s.DirectTxn().DefineProperty(id, "n", NewProperty(types.NewInt(10), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			c := s.CaptureCheckpoint(nil)
			defer c.Close()
			held, resume := make(chan struct{}), make(chan struct{})
			type result struct {
				snapshot Snapshot
				err      error
			}
			resultCh := make(chan result, 1)
			go func() {
				snap, _, err := c.materialize(func(id types.ObjID) {
					if id == 1 {
						close(held)
						<-resume
					}
				})
				resultCh <- result{snap, err}
			}()
			<-held
			done := make(chan types.ErrorCode, 1)
			go func() {
				if coarse {
					done <- s.DirectTxn().SetPropertyValue(0, "n", types.NewInt(20))
					return
				}
				tx := s.BeginSnapshot(0)
				defer tx.Release()
				if ec := tx.SetPropertyValue(0, "n", types.NewInt(5)); ec != types.E_NONE {
					done <- ec
					return
				}
				if ec := tx.SetPropertyValue(1, "n", types.NewInt(15)); ec != types.E_NONE {
					done <- ec
					return
				}
				done <- tx.Commit()
			}()
			select {
			case ec := <-done:
				if ec != types.E_NONE {
					t.Error(ec)
				}
			case <-time.After(2 * time.Second):
				t.Error("writer could not commit during held object walk")
			}
			close(resume)
			got := <-resultCh
			if got.err != nil {
				t.Fatal(got.err)
			}
			if sum := got.snapshot.Objects[0].Properties["n"].Value.Int() + got.snapshot.Objects[1].Properties["n"].Value.Int(); sum != 20 {
				t.Fatalf("torn checkpoint conservation: %d", sum)
			}
			for _, obj := range got.snapshot.Objects {
				if obj.Properties["n"].Value.Int() != 10 {
					t.Fatal("post-capture value in held snapshot")
				}
			}
		})
	}
}

func TestCheckpointCaptureTaskPendingCyclesAndDirectoryAnonymous(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	anon := NewObject(1, 0)
	anon.anonymous = true
	if err := s.Add(anon); err != nil {
		t.Fatal(err)
	}
	w := types.NewWaif(0, 0)
	w.SetProperty("self", w)
	w.SetProperty("anon", types.NewAnon(1))
	if ec := s.DirectTxn().DefineProperty(1, "cycle", NewProperty(w, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	root := types.NewMap([][2]types.Value{{w, types.NewAnon(9999)}, {types.NewAnon(1), w}})
	s.SetPendingFinalizations([]types.Value{w, types.NewAnon(1)})
	c := s.CaptureCheckpoint([]types.Value{root})
	defer c.Close()
	if err := s.Recycle(1); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().SetWaifProperty(w, "anon", types.NewInt(0)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	s.TakePendingFinalizations()
	snap, rewriter, err := c.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.PendingFinalizations) != 0 || len(snap.AnonymousObjects) != 1 {
		t.Fatalf("pending=%d anonymous=%d", len(snap.PendingFinalizations), len(snap.AnonymousObjects))
	}
	c.Close()
	frozen := rewriter.Rewrite(root)
	pairs := frozen.PairsInInsertionOrder()
	if pairs[0][0].Type() != types.TYPE_WAIF || pairs[1][0].Type() != types.TYPE_ANON {
		t.Fatal("map insertion order changed")
	}
	if pairs[0][1].ID() != types.ObjNothing {
		t.Fatal("dangling anonymous reference was not cleared")
	}
	waif := pairs[0][0]
	self, _ := waif.GetProperty("self")
	if self != waif || pairs[1][1] != waif || snap.AnonymousObjects[0].Properties["cycle"].Value != waif {
		t.Fatal("cycle/shared identity lost")
	}
	ref, _ := waif.GetProperty("anon")
	if ref.ID() != snap.AnonymousObjects[0].ID || pairs[1][0].ID() != ref.ID() {
		t.Fatal("anonymous remapping differs across task and object")
	}
	if n := s.activeFloorCount(); n != 0 {
		t.Fatalf("retained floors %d", n)
	}
}

func TestCheckpointCaptureHistoricalAncestry(t *testing.T) {
	s := NewStore()
	for id := types.ObjID(0); id < 3; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
	}
	for id, name := range []string{"old", "new"} {
		if ec := s.DirectTxn().DefineProperty(types.ObjID(id), name, NewProperty(types.NewInt(int64(id+10)), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	if ec := s.ChangeParents(2, []types.ObjID{0}); ec != types.E_NONE {
		t.Fatal(ec)
	}
	c := s.CaptureCheckpoint(nil)
	defer c.Close()
	if ec := s.ChangeParents(2, []types.ObjID{1}); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.DirectTxn().DefineProperty(0, "later", NewProperty(types.NewInt(99), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	snap, _, err := c.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snap.PropertyNames[2], []string{"old"}) {
		t.Fatalf("ancestry property order %v", snap.PropertyNames[2])
	}
	if !reflect.DeepEqual(snap.Objects[2].Parents, []types.ObjID{0}) || !reflect.DeepEqual(snap.Objects[0].Children, []types.ObjID{2}) || len(snap.Objects[1].Children) != 0 {
		t.Fatal("captured parent/children reciprocity lost")
	}
}

func TestCheckpointCaptureForeignWaifAndClosedErrors(t *testing.T) {
	s, foreign := NewStore(), NewStore()
	w := types.NewWaif(0, 0)
	if ec := foreign.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	c := s.CaptureCheckpoint([]types.Value{w})
	if _, _, err := c.Materialize(); err == nil {
		t.Fatal("foreign-domain WAIF silently materialized")
	}
	c.Close()
	c.Close()
	if n := s.activeFloorCount(); n != 0 {
		t.Fatalf("error retained floors %d", n)
	}
	if _, _, err := c.Materialize(); err == nil {
		t.Fatal("closed capture materialized")
	}
}

func TestCheckpointCapturePendingOnlySurvivesCollection(t *testing.T) {
	s := NewStore()
	anon := NewObject(1, 0)
	anon.anonymous = true
	s.AddAnonymous(anon)
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(1))
	if ec := s.DirectTxn().DefineProperty(1, "w", NewProperty(w, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	s.SetPendingFinalizations([]types.Value{types.NewAnon(1)})
	c := s.CaptureCheckpoint(nil)
	defer c.Close()
	s.TakePendingFinalizations()
	candidates := s.AnonymousRecycleCandidates(nil, 0)
	if !reflect.DeepEqual(candidates, []types.ObjID{1}) {
		t.Fatalf("collection candidates %v", candidates)
	}
	for _, id := range candidates {
		if err := s.Recycle(id); err != nil {
			t.Fatal(err)
		}
	}
	if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(2)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	snap, _, err := c.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.PendingFinalizations) != 1 || len(snap.AnonymousObjects) != 1 || snap.PendingFinalizations[0].ID() != snap.AnonymousObjects[0].ID {
		t.Fatal("pending-only anonymous graph lost or ownership duplicated")
	}
	n, _ := snap.AnonymousObjects[0].Properties["w"].Value.GetProperty("n")
	if n.Int() != 1 {
		t.Fatalf("pending-only historical WAIF value %v", n)
	}
}

func TestCheckpointCaptureRetainsAndReleasesHistory(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().DefineProperty(0, "n", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	if ec := s.DirectTxn().DefineProperty(0, "w", NewProperty(w, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	c := s.CaptureCheckpoint(nil)
	defer c.Close()
	write := func(n int64) {
		t.Helper()
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		if ec := tx.SetPropertyValue(0, "n", types.NewInt(n)); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if ec := tx.SetWaifProperty(w, "n", types.NewInt(n)); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if ec := tx.Commit(); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	for n := int64(1); n <= 20; n++ {
		write(n)
	}
	snap, _, err := c.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Objects[0].Properties["n"].Value.Int() != 0 {
		t.Fatal("capture history was pruned prematurely")
	}
	if n := s.historyLen(0); n == 0 {
		t.Fatal("no historical versions retained for capture")
	}
	if n := len(s.waifHistory); n != 1 {
		t.Fatalf("tracked WAIF histories while capture is live: %d, want 1", n)
	}
	t.Logf("before close: numbered_history=%d tracked_waif_histories=%d readers=%d", s.historyLen(0), len(s.waifHistory), s.HistoryStats().Readers)
	detached := snap.Objects[0].Properties["w"].Value
	if value, ok := detached.GetProperty("n"); !ok || value.Int() != 0 {
		t.Fatal("capture lost historical WAIF image")
	}
	c.Close()
	if n := len(s.waifHistory); n != 0 {
		t.Fatalf("tracked WAIF histories after close: %d, want 0", n)
	}
	if _, ok := w.WaifImageAt(s.waifDomain, c.tx.readTS); ok {
		t.Fatal("obsolete WAIF image remains after capture close")
	}
	write(21)
	if n := s.historyLen(0); n > 1 {
		t.Fatalf("history remained after capture close: %d", n)
	}
	t.Logf("after close and next write: numbered_history=%d tracked_waif_histories=%d readers=%d", s.historyLen(0), len(s.waifHistory), s.HistoryStats().Readers)
	if c.tx.objects != nil || c.anonymous != nil || c.segments != nil || c.roots != nil || c.pending != nil {
		t.Fatal("closed capture retains source graph")
	}
	if value, ok := detached.GetProperty("n"); !ok || value.Int() != 0 {
		t.Fatal("released source history changed detached WAIF")
	}
}
