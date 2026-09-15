package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func newBoundaryRenewStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	for _, name := range []string{"seen", "written", "later"} {
		if errCode := store.DirectTxn().DefineProperty(0, name, NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty %s failed: %v", name, errCode)
		}
	}
	return store
}

// A stale read is reported even when nothing has been written yet: a plain
// CommitAndRenew would have skipped validation and silently dropped the read.
func TestCommitAndRenewCarryingReadsRejectsStaleReadWithoutWrites(t *testing.T) {
	store := newBoundaryRenewStore(t)
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if _, errCode := tx.PropertyValue(0, "seen"); errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if errCode := store.DirectTxn().SetPropertyValue(0, "seen", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("live SetPropertyValue failed: %v", errCode)
	}

	next, published, errCode := tx.CommitAndRenewCarryingReads()
	if errCode != types.E_INVARG || published {
		t.Fatalf("renew over a stale read = (%v, published=%v), want E_INVARG without publishing", errCode, published)
	}
	if next != tx {
		t.Fatal("a failed renew must hand back the same transaction")
	}
	if !tx.ValidationFailed() {
		t.Fatal("a stale read at the boundary must count as a validation failure")
	}
}

// The boundary publishes the staged writes and hands back a transaction at the
// current clock. Reads on the object it republished are carried at the versions
// the publication gave them, so the renewed transaction neither conflicts with
// its own publication nor stops watching that object.
func TestCommitAndRenewCarryingReadsCarriesReadsAndPublishes(t *testing.T) {
	store := newBoundaryRenewStore(t)
	tx := store.BeginReadOnly(0)
	if _, errCode := tx.PropertyValue(0, "seen"); errCode != types.E_NONE {
		t.Fatalf("PropertyValue seen failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(0, "written", types.NewInt(5)); errCode != types.E_NONE {
		t.Fatalf("staged SetPropertyValue failed: %v", errCode)
	}

	next, published, errCode := tx.CommitAndRenewCarryingReads()
	if errCode != types.E_NONE || !published {
		t.Fatalf("renew = (%v, published=%v), want E_NONE with the write published", errCode, published)
	}
	defer next.Release()
	if next == tx || next.ReadTimestamp() <= tx.ReadTimestamp() {
		t.Fatalf("renewed txn readTS %d must advance past %d on a fresh transaction", next.ReadTimestamp(), tx.ReadTimestamp())
	}
	live, errCode := store.DirectTxn().PropertyValue(0, "written")
	if errCode != types.E_NONE || live.Int() != 5 {
		t.Fatalf("published value = %v (%v), want 5", live, errCode)
	}

	// #0 is the object the boundary republished; the carried read of #0.seen must
	// not make the renewed transaction conflict with that publication.
	if errCode := next.SetPropertyValue(0, "later", types.NewInt(7)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue on renewed txn failed: %v", errCode)
	}
	if errCode := next.Commit(); errCode != types.E_NONE {
		t.Fatalf("renewed txn commit = %v, want E_NONE", errCode)
	}
}

func TestCommitAndRenewCarryingReadsStillWatchesRepublishedObject(t *testing.T) {
	store := newBoundaryRenewStore(t)
	tx := store.BeginReadOnly(0)
	if _, errCode := tx.PropertyValue(0, "seen"); errCode != types.E_NONE {
		t.Fatalf("PropertyValue seen failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(0, "written", types.NewInt(5)); errCode != types.E_NONE {
		t.Fatalf("staged SetPropertyValue failed: %v", errCode)
	}
	next, _, errCode := tx.CommitAndRenewCarryingReads()
	if errCode != types.E_NONE {
		t.Fatalf("renew = %v, want E_NONE", errCode)
	}
	defer next.Release()

	// A live mutation of the carried read on the republished object, after the
	// boundary, must still be detected by the renewed transaction's commit.
	if errCode := store.DirectTxn().SetPropertyValue(0, "seen", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("live SetPropertyValue seen failed: %v", errCode)
	}
	if errCode := next.SetPropertyValue(0, "later", types.NewInt(7)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue later failed: %v", errCode)
	}
	if errCode := next.Commit(); errCode != types.E_INVARG || !next.ValidationFailed() {
		t.Fatalf("renewed txn commit = %v (validationFailed=%v), want E_INVARG conflict on the carried read", errCode, next.ValidationFailed())
	}
}

func TestCommitAndRenewCarryingReadsKeepsReadsOfUntouchedObjects(t *testing.T) {
	store := newBoundaryRenewStore(t)
	child, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(child, "other", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty other failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := tx.PropertyValue(child, "other"); errCode != types.E_NONE {
		t.Fatalf("PropertyValue other failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(0, "written", types.NewInt(5)); errCode != types.E_NONE {
		t.Fatalf("staged SetPropertyValue failed: %v", errCode)
	}
	next, _, errCode := tx.CommitAndRenewCarryingReads()
	if errCode != types.E_NONE {
		t.Fatalf("renew = %v, want E_NONE", errCode)
	}
	defer next.Release()

	// A live mutation of the carried read after the boundary must still be
	// detected when the renewed transaction commits.
	if errCode := store.DirectTxn().SetPropertyValue(child, "other", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("live SetPropertyValue other failed: %v", errCode)
	}
	if errCode := next.SetPropertyValue(0, "later", types.NewInt(7)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue later failed: %v", errCode)
	}
	if errCode := next.Commit(); errCode != types.E_INVARG || !next.ValidationFailed() {
		t.Fatalf("renewed txn commit = %v (validationFailed=%v), want E_INVARG conflict on the carried read", errCode, next.ValidationFailed())
	}
}
