package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// hotRead reads objID.name through FindPropertyRenewing and returns the
// transaction to use from here on.
func hotRead(t *testing.T, tx *StoreTxn, objID types.ObjID, name string) *StoreTxn {
	t.Helper()
	_, ec, next := tx.FindPropertyRenewing(objID, name)
	if ec != types.E_NONE {
		t.Fatalf("FindPropertyRenewing #%d.%s: %v", objID, name, ec)
	}
	return next
}

func stageValue(t *testing.T, tx *StoreTxn, objID types.ObjID, name string, value int64) {
	t.Helper()
	if ec := tx.SetPropertyValue(objID, name, types.NewInt(value)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue #%d.%s: %v", objID, name, ec)
	}
}

// earlyFixture is conflictFixture with tracking on and #a.p hot.
func earlyFixture(t *testing.T) (s *Store, a, b types.ObjID) {
	t.Helper()
	s, a, b = conflictFixture(t)
	s.SetConflictCensusTracking(true)
	s.MarkHotProperty(a, "p")
	return s, a, b
}

// A transaction reads hot #a.p and stages a write; a check finds nothing; another
// commit rewrites #a.p; the next check, 30 ticks into the slice, detects it; the
// commit at 100 ticks loses. The 70 ticks between are savable, run-wide and on
// #a.p's row.
func TestEarlyDetectionCountsSavableTicksOfALoss(t *testing.T) {
	s, a, b := earlyFixture(t)
	stub := &stubSite{ticks: 10}
	loser := s.BeginSnapshot(0)
	loser.SetConflictSiteFunc(stub.fn)
	loser = hotRead(t, loser, a, "p")
	stageValue(t, loser, b, "q", 7)
	loser.NoteSupersededHotRead()
	if e := s.ConflictCensus().EarlyDetection; e.Checks != 1 || e.Detected != 0 {
		t.Fatalf("before the rewrite: %+v, want one check and no detection", e)
	}
	commitValue(t, s, a, "p", types.NewInt(5))
	stub.ticks = 30
	loser.NoteSupersededHotRead()
	stub.ticks = 100
	if ec := loser.Commit(); ec == types.E_NONE || !loser.ValidationFailed() {
		t.Fatalf("commit = %v, want a lost validation", ec)
	}
	loser.Release()

	c := s.ConflictCensus()
	e := c.EarlyDetection
	t.Logf("early = %+v", e)
	want := EarlyDetection{Checks: 2, Detected: 1, DetectedLost: 1, SavableTicks: 70}
	if e != want {
		t.Errorf("early = %+v, want %+v", e, want)
	}
	if d := censusRow(t, c, ConflictProperty, a, "p").Diag; d.DetectedLosses != 1 || d.SavableTicks != 70 {
		t.Errorf("#a.p detected %d savable %d, want 1 and 70", d.DetectedLosses, d.SavableTicks)
	}
	if share := e.SavableShare(140); share != 50 {
		t.Errorf("SavableShare(140) = %v, want 50", share)
	}
	s.ResetConflictCensus()
	if e := s.ConflictCensus().EarlyDetection; e != (EarlyDetection{}) {
		t.Errorf("after reset: %+v", e)
	}
}

// BenchmarkNoteSupersededHotRead times one check past the fast exits: three
// hot reads, none superseded (the common case at a checkpoint), so every slot
// is compared. The clock exit is defeated each iteration.
func BenchmarkNoteSupersededHotRead(b *testing.B) {
	s, a, obj := conflictFixture(b)
	s.SetConflictCensusTracking(true)
	for _, k := range [][2]any{{a, "p"}, {a, "q"}, {obj, "p"}} {
		s.MarkHotProperty(k[0].(types.ObjID), k[1].(string))
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	for _, k := range [][2]any{{a, "p"}, {a, "q"}, {obj, "p"}} {
		if _, ec, next := tx.FindPropertyRenewing(k[0].(types.ObjID), k[1].(string)); ec != types.E_NONE || next != tx {
			b.Fatalf("read %v: %v", k, ec)
		}
	}
	if ec := tx.SetPropertyValue(obj, "q", types.NewInt(7)); ec != types.E_NONE {
		b.Fatalf("stage: %v", ec)
	}
	if len(tx.diag.hotReads) != 3 {
		b.Fatalf("hotReads = %v, want 3", tx.diag.hotReads)
	}
	b.ReportAllocs()
	for b.Loop() {
		tx.diag.lastCheckClock = ^uint64(0)
		tx.NoteSupersededHotRead()
	}
	if tx.diag.detected {
		b.Fatal("detected with nothing superseded")
	}
}

// Checks that must not detect, or must not get past the fast exits.
func TestEarlyDetectionDoesNotDetect(t *testing.T) {
	t.Run("unrelated_commit", func(t *testing.T) {
		s, a, b := earlyFixture(t)
		tx := hotRead(t, s.BeginSnapshot(0), a, "p")
		defer tx.Release()
		stageValue(t, tx, b, "q", 7)
		commitValue(t, s, b, "p", types.NewInt(5))
		tx.NoteSupersededHotRead()
		if e := s.ConflictCensus().EarlyDetection; e.Checks != 1 || e.Detected != 0 {
			t.Errorf("early = %+v, want one check and no detection", e)
		}
	})
	t.Run("read_only", func(t *testing.T) {
		s, a, _ := earlyFixture(t)
		tx := hotRead(t, s.BeginSnapshot(0), a, "p")
		defer tx.Release()
		commitValue(t, s, a, "p", types.NewInt(5))
		tx.NoteSupersededHotRead()
		if e := s.ConflictCensus().EarlyDetection; e.Checks != 0 || e.Detected != 0 {
			t.Errorf("early = %+v, want no check: a read-only transaction cannot lose", e)
		}
	})
	t.Run("unchanged_clock", func(t *testing.T) {
		s, a, b := earlyFixture(t)
		tx := hotRead(t, s.BeginSnapshot(0), a, "p")
		defer tx.Release()
		stageValue(t, tx, b, "q", 7)
		tx.NoteSupersededHotRead()
		tx.NoteSupersededHotRead()
		if e := s.ConflictCensus().EarlyDetection; e.Checks != 1 {
			t.Errorf("early = %+v, want the second check at the same clock skipped", e)
		}
		commitValue(t, s, a, "p", types.NewInt(5))
		tx.NoteSupersededHotRead()
		if e := s.ConflictCensus().EarlyDetection; e.Checks != 2 || e.Detected != 1 {
			t.Errorf("early = %+v, want the check after the clock moved to detect", e)
		}
	})
	t.Run("created_object", func(t *testing.T) {
		// A hot slot on an object the transaction created has no published
		// version to be superseded.
		s, a, b := earlyFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		id, ec := tx.CreateObject([]types.ObjID{a}, a)
		if ec != types.E_NONE {
			t.Fatalf("CreateObject: %v", ec)
		}
		s.MarkHotProperty(id, "p")
		tx = hotRead(t, tx, id, "p")
		stageValue(t, tx, b, "q", 7)
		commitValue(t, s, b, "p", types.NewInt(5))
		tx.NoteSupersededHotRead()
		if e := s.ConflictCensus().EarlyDetection; e.Detected != 0 {
			t.Errorf("early = %+v, want no detection on a created object", e)
		}
	})
}

// A renewal carries the hot slots read, so a rewrite after it is detected on
// the renewed transaction, and does not carry a detection.
func TestEarlyDetectionRenewalCarriesHotReads(t *testing.T) {
	t.Run("commit_and_renew_carrying_reads", func(t *testing.T) {
		s, a, b := earlyFixture(t)
		tx := hotRead(t, s.BeginSnapshot(0), a, "p")
		stageValue(t, tx, b, "q", 7)
		next, _, ec := tx.CommitAndRenewCarryingReads()
		if ec != types.E_NONE {
			t.Fatalf("CommitAndRenewCarryingReads: %v", ec)
		}
		defer next.Release()
		if !slices.Contains(next.diag.hotReads, propertyReadKey{objID: a, name: "p"}) {
			t.Fatalf("renewed hot reads = %v, want #a.p carried", next.diag.hotReads)
		}
		stageValue(t, next, b, "q", 8)
		commitValue(t, s, a, "p", types.NewInt(5))
		next.NoteSupersededHotRead()
		if e := s.ConflictCensus().EarlyDetection; e.Detected != 1 {
			t.Errorf("early = %+v, want the rewrite after renewal detected", e)
		}
	})
	t.Run("renewal_at_stale_hot_read", func(t *testing.T) {
		s, a, b := earlyFixture(t)
		s.MarkHotProperty(a, "q")
		tx := hotRead(t, s.BeginSnapshot(0), a, "q")
		stageValue(t, tx, b, "q", 7)
		commitValue(t, s, a, "p", types.NewInt(5))
		next := hotRead(t, tx, a, "p")
		defer next.Release()
		if next == tx {
			t.Fatal("stale hot read of #a.p did not renew")
		}
		want := []propertyReadKey{{objID: a, name: "q"}, {objID: a, name: "p"}}
		if !slices.Equal(next.diag.hotReads, want) || next.diag.detected {
			t.Errorf("renewed hot reads %v detected %v, want %v and not detected", next.diag.hotReads, next.diag.detected, want)
		}
	})
	t.Run("detection_not_carried", func(t *testing.T) {
		// A detected transaction whose only write is elided back to the original
		// value renews without committing; the renewed one is not detected.
		s, a, _ := earlyFixture(t)
		tx := hotRead(t, s.BeginSnapshot(0), a, "p")
		stageValue(t, tx, a, "p", 1)
		commitValue(t, s, a, "p", types.NewInt(5))
		tx.NoteSupersededHotRead()
		if !tx.diag.detected {
			t.Fatal("rewrite of #a.p not detected")
		}
		stageValue(t, tx, a, "p", 0)
		next, published, ec := tx.CommitAndRenew()
		if ec != types.E_NONE || published {
			t.Fatalf("CommitAndRenew = %v published %v, want an unpublished renewal", ec, published)
		}
		defer next.Release()
		if next.diag.detected {
			t.Error("renewed transaction carries the detection")
		}
	})
}

// With tracking off the check is a no-op and nothing is recorded.
func TestEarlyDetectionOffRecordsNothing(t *testing.T) {
	s, a, b := conflictFixture(t)
	s.MarkHotProperty(a, "p")
	tx := hotRead(t, s.BeginSnapshot(0), a, "p")
	defer tx.Release()
	stageValue(t, tx, b, "q", 7)
	commitValue(t, s, a, "p", types.NewInt(5))
	tx.NoteSupersededHotRead()
	if ec := tx.Commit(); ec == types.E_NONE {
		t.Fatal("commit did not lose")
	}
	if tx.diag != nil {
		t.Error("transaction carries a census record with tracking off")
	}
	if e := s.ConflictCensus().EarlyDetection; e != (EarlyDetection{}) {
		t.Errorf("early with tracking off = %+v", e)
	}
}

// A detected transaction whose write is elided back to the original value
// commits without validating; at its release it counts as detected, not lost.
func TestEarlyDetectionDetectedNotLost(t *testing.T) {
	s, a, _ := earlyFixture(t)
	tx := hotRead(t, s.BeginSnapshot(0), a, "p")
	stageValue(t, tx, a, "p", 1)
	commitValue(t, s, a, "p", types.NewInt(5))
	tx.NoteSupersededHotRead()
	stageValue(t, tx, a, "p", 0)
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit of an elided write = %v", ec)
	}
	tx.Release()
	if e := s.ConflictCensus().EarlyDetection; e.Detected != 1 || e.DetectedNotLost != 1 || e.DetectedLost != 0 {
		t.Errorf("early = %+v, want one detected, not lost", e)
	}
}

// An undetected loss is tracked when it is stale on a hot slot the transaction
// recorded (the rewrite came after the last check), untracked otherwise.
func TestEarlyDetectionUndetectedTrackedAndUntracked(t *testing.T) {
	s, a, b := earlyFixture(t)
	tracked := hotRead(t, s.BeginSnapshot(0), a, "p")
	stageValue(t, tracked, b, "q", 7)
	tracked.NoteSupersededHotRead()
	commitValue(t, s, a, "p", types.NewInt(5))
	if ec := tracked.Commit(); ec == types.E_NONE {
		t.Fatal("tracked commit did not lose")
	}
	tracked.Release()

	untracked := s.BeginSnapshot(0)
	readValue(t, untracked, a, "q")
	stageValue(t, untracked, b, "q", 8)
	commitValue(t, s, a, "q", types.NewInt(5))
	if ec := untracked.Commit(); ec == types.E_NONE {
		t.Fatal("untracked commit did not lose")
	}
	untracked.Release()

	e := s.ConflictCensus().EarlyDetection
	if e.Detected != 0 || e.UndetectedLost != 2 || e.UndetectedTracked != 1 || e.DetectedNotLost != 0 {
		t.Errorf("early = %+v, want two undetected losses, one tracked", e)
	}
}
