package store

import (
	"testing"

	"barn/types"
)

// historyLen returns the number of retained old versions for id (test-only).
func (s *Store) historyLen(id types.ObjID) int {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	return len(s.history[id])
}

// activeFloorCount returns the number of distinct live readTS registrations
// (test-only). Used to assert the floor registry is not leaking.
func (s *Store) activeFloorCount() int {
	s.floorMu.Lock()
	defer s.floorMu.Unlock()
	return len(s.activeReadTS)
}

// TestHistoryGCKeepsLongReaderSnapshotThenPrunes is the Phase 4 gate: a long-lived
// read transaction opened at an OLD readTS must keep reading its snapshot value
// across many newer commits (the live-read floor pins its needed history entry),
// and once that reader is released the now-dead history must shrink under the next
// commit's prune (bounded growth — not append-only forever).
func TestHistoryGCKeepsLongReaderSnapshotThenPrunes(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "n", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	// Commit a known baseline value the long reader will snapshot.
	base := store.BeginReadOnly(0)
	if errCode := base.SetPropertyValue(0, "n", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("baseline SetPropertyValue failed: %v", errCode)
	}
	if errCode := base.Commit(); errCode != types.E_NONE {
		t.Fatalf("baseline Commit failed: %v", errCode)
	}
	base.Release()

	// Open the LONG-LIVED reader at the OLD readTS (sees n==1). It stays live.
	reader := store.BeginReadOnly(0)
	prop, errCode := reader.FindProperty(0, "n")
	if errCode != types.E_NONE {
		t.Fatalf("reader initial FindProperty failed: %v", errCode)
	}
	if got := prop.Value.Int(); got != 1 {
		t.Fatalf("reader initial value = %d, want 1", got)
	}

	// Do MANY newer commits on the same property. Each pushes the prior image into
	// history; the long reader's needed entry (n==1) must NOT be pruned out from
	// under it because the floor == reader.readTS pins it.
	const commits = 200
	for i := 2; i <= commits+1; i++ {
		w := store.BeginReadOnly(0)
		if errCode := w.SetPropertyValue(0, "n", types.NewInt(int64(i))); errCode != types.E_NONE {
			t.Fatalf("commit %d SetPropertyValue failed: %v", i, errCode)
		}
		if errCode := w.Commit(); errCode != types.E_NONE {
			t.Fatalf("commit %d Commit failed: %v", i, errCode)
		}
		w.Release()

		// (a) The long reader STILL reads its snapshot value across every commit.
		prop, errCode := reader.FindProperty(0, "n")
		if errCode != types.E_NONE {
			t.Fatalf("reader FindProperty after commit %d failed: %v", i, errCode)
		}
		if got := prop.Value.Int(); got != 1 {
			t.Fatalf("reader value after commit %d = %d, want stable 1 (pruned out from under live reader!)", i, got)
		}
	}

	// While the reader is live at the OLD readTS, the floor == reader.readTS pins the
	// reader's snapshot entry AND (per the invariant — min-floor cannot prove no
	// other reader needs an intermediate version) every version newer than the floor.
	// So with a single old reader, history holds the reader's snapshot plus the
	// intermediate images; the version EQUAL to the floor is the newest-<=floor that
	// is retained, and all stale versions strictly below it are gone. The key GC
	// property proven here: the entries strictly OLDER than the reader's snapshot
	// (e.g. the n==0 baseline) are NOT retained even though they were committed.
	liveLen := store.historyLen(0)
	if liveLen < 1 {
		t.Fatalf("history length while reader live = %d, want >=1 (reader's snapshot must be retained)", liveLen)
	}
	// The baseline n==0 version (ts strictly below the reader's snapshot ts) must have
	// been pruned: history holds at most `commits` images (snapshot + intermediates),
	// never the +1 that would mean the dead pre-snapshot version was kept.
	if liveLen > commits {
		t.Fatalf("history length while reader live = %d, want <= %d (pre-snapshot dead version not pruned)", liveLen, commits)
	}

	// The live reader still reads its snapshot one final time before release.
	prop, errCode = reader.FindProperty(0, "n")
	if errCode != types.E_NONE {
		t.Fatalf("reader final FindProperty failed: %v", errCode)
	}
	if got := prop.Value.Int(); got != 1 {
		t.Fatalf("reader final value = %d, want 1", got)
	}

	// (b) Release the long reader: now NO live txn pins the old versions. The floor
	// rises to the clock. The next commit's prune must shrink history to the bound
	// (only the newest-<=floor version is retained; everything older is dead).
	reader.Release()

	w := store.BeginReadOnly(0)
	if errCode := w.SetPropertyValue(0, "n", types.NewInt(9999)); errCode != types.E_NONE {
		t.Fatalf("post-release SetPropertyValue failed: %v", errCode)
	}
	if errCode := w.Commit(); errCode != types.E_NONE {
		t.Fatalf("post-release Commit failed: %v", errCode)
	}
	// w is itself live during its own commit, but it registered at the current clock,
	// and the prune samples the floor AFTER the append, so its readTS does not pin the
	// old versions below it. Release it too, then confirm the dead history is gone.
	w.Release()

	afterLen := store.historyLen(0)
	if afterLen > liveLen {
		t.Fatalf("history length after reader release = %d, did not shrink below live length %d (dead history leaked)", afterLen, liveLen)
	}
	// Concretely: with no reader below the newest version, at most one old version
	// (the newest <= floor) is retained.
	if afterLen > 1 {
		t.Fatalf("history length after reader release = %d, want <=1 (dead versions not pruned)", afterLen)
	}

	// The live value is the last write; new readers see it.
	live, errCode := store.PropertyValue(0, "n")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %v", errCode)
	}
	if got := live.Int(); got != 9999 {
		t.Fatalf("live value = %d, want 9999", got)
	}

	// The floor registry must not leak: every begun txn was Released.
	if n := store.activeFloorCount(); n != 0 {
		t.Fatalf("active floor registrations = %d, want 0 (registration leak)", n)
	}
}

// TestHistoryGCRegistryDeregistersOnRelease asserts the readTS multiset tracks and
// releases registrations correctly, including duplicate readTS values.
func TestHistoryGCRegistryDeregistersOnRelease(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	a := store.BeginReadOnly(0)
	b := store.BeginReadOnly(0) // same readTS as a (no commits between) -> multiset count 2
	if got := store.activeFloorCount(); got != 1 {
		t.Fatalf("distinct active readTS = %d, want 1 (a and b share readTS)", got)
	}

	a.Release()
	// b still holds the same readTS, so the key must remain.
	if got := store.activeFloorCount(); got != 1 {
		t.Fatalf("distinct active readTS after releasing a = %d, want 1 (b still live)", got)
	}
	b.Release()
	if got := store.activeFloorCount(); got != 0 {
		t.Fatalf("distinct active readTS after releasing both = %d, want 0", got)
	}

	// Double Release is a no-op (idempotent), must not drive the count negative.
	a.Release()
	b.Release()
	if got := store.activeFloorCount(); got != 0 {
		t.Fatalf("distinct active readTS after double release = %d, want 0", got)
	}
}

func TestCOWHistoryDoesNotShareCollectionsWithLiveImage(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "n", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	reader := store.BeginReadOnly(0)
	defer reader.Release()

	writer := store.BeginReadOnly(0)
	if errCode := writer.SetPropertyValue(0, "n", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := writer.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	writer.Release()

	if _, errCode := store.AddVerb(0, NewVerb("later", []string{"later"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	if verb, _, err := reader.FindVerb(0, "later"); err == nil {
		t.Fatalf("reader found verb added after its snapshot: %#v", verb)
	}
}
