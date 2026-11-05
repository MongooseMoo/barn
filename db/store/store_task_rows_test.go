package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// taskRowStore is a store with one object carrying one property, for task-row
// tests that also read or write object state.
func taskRowStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DirectTxn().DefineProperty(0, "scratch", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	return store
}

func recordTaskRows(store *Store) *[]PublishedTaskRow {
	var published []PublishedTaskRow
	store.SetTaskRowObserver(func(rows []PublishedTaskRow) {
		published = append(published, rows...)
	})
	return &published
}

func TestTaskRowCommitPublishesStagedRow(t *testing.T) {
	store := taskRowStore(t)
	published := recordTaskRows(store)
	before := store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})

	tx := store.BeginSnapshot(0)
	defer tx.Release()
	row, ok := tx.ReadTaskRow(7)
	if !ok || row.Status != TaskRowSuspended || row.Owner != 3 || row.Version != before {
		t.Fatalf("ReadTaskRow = %+v, %v; want the suspended row at version %d", row, ok, before)
	}
	tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowQueued, Value: types.NewInt(5), By: 9})
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit = %v", errCode)
	}
	live, ok := store.TaskRow(7)
	if !ok || live.Status != TaskRowQueued || live.Value.Int() != 5 || live.By != 9 || live.Version <= before {
		t.Fatalf("live row = %+v, %v; want queued with 5 by 9 at a newer version", live, ok)
	}
	if len(*published) != 1 || (*published)[0].ID != 7 || (*published)[0].Version != live.Version {
		t.Fatalf("published = %+v, want the one committed row", *published)
	}
}

// Two transactions that read one row and both write it: the second to commit
// loses validation, and its retry reads what the first wrote.
func TestTaskRowConcurrentWritersConflict(t *testing.T) {
	store := taskRowStore(t)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})

	first, second := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer first.Release()
	defer second.Release()
	for _, tx := range []*StoreTxn{first, second} {
		if _, ok := tx.ReadTaskRow(7); !ok {
			t.Fatal("row missing")
		}
		tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowKilled})
	}
	if errCode := first.Commit(); errCode != types.E_NONE {
		t.Fatalf("first Commit = %v", errCode)
	}
	if errCode := second.Commit(); errCode != types.E_INVARG || !second.ValidationFailed() {
		t.Fatalf("second Commit = %v (validation failed %v), want a validation conflict", errCode, second.ValidationFailed())
	}
	retry := store.BeginSnapshot(0)
	defer retry.Release()
	if row, ok := retry.ReadTaskRow(7); !ok || row.Status != TaskRowKilled {
		t.Fatalf("retry reads %+v, %v; want the killed row", row, ok)
	}
}

func TestTaskRowReadsStagedWrite(t *testing.T) {
	store := taskRowStore(t)
	tx := store.BeginSnapshot(0)
	defer tx.Release()
	tx.StageTaskRow(4, TaskRow{Owner: 2, Status: TaskRowRunning})
	if row, ok := tx.ReadTaskRow(4); !ok || row.Status != TaskRowRunning {
		t.Fatalf("ReadTaskRow after stage = %+v, %v; want the staged row", row, ok)
	}
	tx.StageTaskRow(4, TaskRow{Status: TaskRowFinished})
	if row, ok := tx.ReadTaskRow(4); ok {
		t.Fatalf("ReadTaskRow after staging finished = %+v, want no row", row)
	}
	if _, ok := store.TaskRow(4); ok {
		t.Fatal("staged row is visible before commit")
	}
}

func TestTaskRowFinishedCommitRemovesRow(t *testing.T) {
	store := taskRowStore(t)
	published := recordTaskRows(store)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowRunning})
	tx := store.BeginSnapshot(0)
	defer tx.Release()
	tx.ReadTaskRow(7)
	tx.StageTaskRow(7, TaskRow{Status: TaskRowFinished})
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit = %v", errCode)
	}
	if row, ok := store.TaskRow(7); ok {
		t.Fatalf("row after finishing = %+v, want removed", row)
	}
	if len(*published) != 1 || (*published)[0].Status != TaskRowFinished {
		t.Fatalf("published = %+v, want the finished row", *published)
	}
}

// A row read as missing conflicts with a row inserted before the commit.
func TestTaskRowMissingReadConflictsWithInsert(t *testing.T) {
	store := taskRowStore(t)
	tx := store.BeginSnapshot(0)
	defer tx.Release()
	if _, ok := tx.ReadTaskRow(9); ok {
		t.Fatal("row 9 exists")
	}
	tx.StageTaskRow(10, TaskRow{Owner: 1, Status: TaskRowRunning})
	store.PutTaskRow(9, TaskRow{Owner: 1, Status: TaskRowRunning})
	if errCode := tx.Commit(); errCode != types.E_INVARG || !tx.ValidationFailed() {
		t.Fatalf("Commit = %v, want a validation conflict on the inserted row", errCode)
	}
}

// A commit that writes only task rows validates only task rows, as a slice
// that wrote nothing else never validated its object reads.
func TestTaskRowOnlyCommitDoesNotValidateObjectReads(t *testing.T) {
	store := taskRowStore(t)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowRunning})
	tx := store.BeginSnapshot(0)
	defer tx.Release()
	if _, errCode := tx.FindProperty(0, "scratch"); errCode != types.E_NONE {
		t.Fatalf("FindProperty = %v", errCode)
	}
	if errCode := store.DirectTxn().SetPropertyValue(0, "scratch", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue = %v", errCode)
	}
	tx.ReadTaskRow(7)
	tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit = %v, want the row-only commit to ignore the stale property read", errCode)
	}
}

// A commit that also writes object state validates every read, task rows
// included.
func TestTaskRowMixedCommitValidatesTaskReads(t *testing.T) {
	store := taskRowStore(t)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowRunning})
	tx := store.BeginSnapshot(0)
	defer tx.Release()
	tx.ReadTaskRow(7)
	if errCode := tx.SetPropertyValue(0, "scratch", types.NewInt(3)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue = %v", errCode)
	}
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowKilled})
	tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})
	if errCode := tx.Commit(); errCode != types.E_INVARG || !tx.ValidationFailed() {
		t.Fatalf("Commit = %v, want a validation conflict on the task row", errCode)
	}
	if value, _ := store.DirectTxn().PropertyValue(0, "scratch"); value.Int() != 1 {
		t.Fatalf("scratch = %v, want the lost commit unpublished", value)
	}
}

// A commit boundary ends the transaction's task-row reads, like its object
// reads: they were validated by the commit that published. The renewed
// transaction must not lose to what followed from that publication, such as
// the killed task's row being removed when it settles.
func TestCommitAndRenewStartsFreshTaskRowReads(t *testing.T) {
	store := taskRowStore(t)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})
	store.PutTaskRow(8, TaskRow{Owner: 3, Status: TaskRowSuspended})
	tx := store.BeginSnapshot(0)
	tx.ReadTaskRow(7)
	tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowKilled})
	next, published, errCode := tx.CommitAndRenew()
	if errCode != types.E_NONE || !published {
		t.Fatalf("CommitAndRenew = %v published %v", errCode, published)
	}
	defer next.Release()
	store.RemoveTaskRow(7)
	next.ReadTaskRow(8)
	next.StageTaskRow(8, TaskRow{Owner: 3, Status: TaskRowQueued, Value: types.NewInt(1)})
	if errCode := next.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit after renewal = %v, want no conflict on row 7 read before the boundary", errCode)
	}
}

// A task-row read after renewal is validated as usual.
func TestTaskRowReadAfterRenewalConflicts(t *testing.T) {
	store := taskRowStore(t)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowRunning})
	tx := store.BeginSnapshot(0)
	tx.StageTaskRow(8, TaskRow{Owner: 3, Status: TaskRowRunning})
	next, _, errCode := tx.CommitAndRenew()
	if errCode != types.E_NONE {
		t.Fatalf("CommitAndRenew = %v", errCode)
	}
	defer next.Release()
	next.ReadTaskRow(7)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowKilled})
	next.StageTaskRow(8, TaskRow{Owner: 3, Status: TaskRowSuspended})
	if errCode := next.Commit(); errCode != types.E_INVARG || !next.ValidationFailed() {
		t.Fatalf("Commit = %v, want the row read after renewal to conflict", errCode)
	}
}

// The direct transaction publishes a staged row at once.
func TestTaskRowDirectStagePublishesNow(t *testing.T) {
	store := taskRowStore(t)
	published := recordTaskRows(store)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})
	direct := store.DirectTxn()
	direct.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowKilled})
	if row, ok := store.TaskRow(7); !ok || row.Status != TaskRowKilled {
		t.Fatalf("live row = %+v, %v; want killed at once", row, ok)
	}
	if len(*published) != 1 || (*published)[0].Status != TaskRowKilled {
		t.Fatalf("published = %+v, want the killed row", *published)
	}
}

func TestRemoveTaskRow(t *testing.T) {
	store := taskRowStore(t)
	store.PutTaskRow(7, TaskRow{Owner: 3, Status: TaskRowRunning})
	store.RemoveTaskRow(7)
	if _, ok := store.TaskRow(7); ok {
		t.Fatal("row survived removal")
	}
	store.RemoveTaskRow(7) // removing a missing row is harmless
}
