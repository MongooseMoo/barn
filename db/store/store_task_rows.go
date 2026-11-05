package store

import (
	"strconv"
	"sync"

	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/types"
)

// TaskRowStatus is what MOO code in another task can see of a task's
// scheduling state.
type TaskRowStatus uint8

const (
	// TaskRowFinished is no row: the task completed or was never given one.
	TaskRowFinished TaskRowStatus = iota
	// TaskRowRunning is a live task resume() cannot reach: one running, a fork
	// waiting to start, or one waiting on exec().
	TaskRowRunning
	// TaskRowSuspended is a task waiting in suspend(); resume() readies it.
	TaskRowSuspended
	// TaskRowQueued is a task resume() readied that has not yet run; another
	// resume() replaces its Value.
	TaskRowQueued
	// TaskRowKilled is a task kill_task() killed.
	TaskRowKilled
)

// TaskRow is one task's row. Value is the pending resume value of a queued
// row, and By the task whose commit wrote it, the readier of a resume. Version
// is set by the store when the row is published.
type TaskRow struct {
	Owner   types.ObjID
	Status  TaskRowStatus
	Value   types.Value
	By      int64
	Version uint64
}

// Live reports whether the row names a task kill_task() and resume() can find.
func (r TaskRow) Live() bool {
	return r.Status != TaskRowFinished && r.Status != TaskRowKilled
}

// PublishedTaskRow is a row as a commit published it; a finished row was
// removed.
type PublishedTaskRow struct {
	ID int64
	TaskRow
}

// taskRowTable holds every task's row. mu is a leaf lock: a commit takes it
// inside store.mu and commitMu and holds it from validating its task-row reads
// through publishing its task-row writes.
type taskRowTable struct {
	mu       sync.Mutex
	rows     map[int64]TaskRow
	version  uint64
	observer func([]PublishedTaskRow)
}

// SetTaskRowObserver installs the function told of each publication of task
// rows by a transaction, after the publishing commit has released its locks.
func (s *Store) SetTaskRowObserver(observer func([]PublishedTaskRow)) {
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	s.tasks.observer = observer
}

// TaskRow returns the published row of task id.
func (s *Store) TaskRow(id int64) (TaskRow, bool) {
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	row, ok := s.tasks.rows[id]
	return row, ok
}

// PutTaskRow publishes row as task id's row outside any transaction and
// returns its version. The runtime uses it for a task it registers.
func (s *Store) PutTaskRow(id int64, row TaskRow) uint64 {
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	return s.tasks.putLocked(id, row)
}

// RemoveTaskRow removes task id's row outside any transaction, for a task that
// finished without committing its end.
func (s *Store) RemoveTaskRow(id int64) {
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	delete(s.tasks.rows, id)
}

// putLocked publishes row at a new version, or removes it when finished.
func (t *taskRowTable) putLocked(id int64, row TaskRow) uint64 {
	if row.Status == TaskRowFinished {
		delete(t.rows, id)
		return 0
	}
	t.version++
	row.Version = t.version
	lazySet(&t.rows, id, row)
	return row.Version
}

func (s *Store) notifyTaskRows(published []PublishedTaskRow) {
	if len(published) == 0 {
		return
	}
	s.tasks.mu.Lock()
	observer := s.tasks.observer
	s.tasks.mu.Unlock()
	if observer != nil {
		observer(published)
	}
}

// ReadTaskRow returns task id's row as this transaction sees it: its own
// staged write, or the latest published row, whose version the commit then
// validates.
func (tx *StoreTxn) ReadTaskRow(id int64) (TaskRow, bool) {
	if row, staged := tx.taskWrites[id]; staged {
		return row, row.Status != TaskRowFinished
	}
	row, ok := tx.store.TaskRow(id)
	if !tx.live() {
		if _, marked := tx.taskReads[id]; !marked {
			lazySet(&tx.taskReads, id, row.Version)
		}
	}
	return row, ok
}

// StageTaskRow writes task id's row at this transaction's commit. The direct
// transaction publishes it at once.
func (tx *StoreTxn) StageTaskRow(id int64, row TaskRow) {
	if tx.live() {
		s := tx.store
		s.tasks.mu.Lock()
		row.Version = s.tasks.putLocked(id, row)
		s.tasks.mu.Unlock()
		s.notifyTaskRows([]PublishedTaskRow{{ID: id, TaskRow: row}})
		return
	}
	lazySet(&tx.taskWrites, id, row)
}

// validateTaskReadsLocked checks every task-row read. The caller holds
// tasks.mu.
func (tx *StoreTxn) validateTaskReadsLocked(first *types.ErrorCode) {
	for id, version := range tx.taskReads {
		if row := tx.store.tasks.rows[id]; row.Version != version {
			tx.noteConflict(first, types.E_INVARG, ReadConflict{Kind: ConflictTask, ObjID: types.ObjNothing,
				Name: strconv.FormatInt(id, 10), Read: version, Live: row.Version, Missing: row.Version == 0})
		}
	}
}

// publishTaskRowsLocked publishes the staged task rows and moves this
// transaction's reads of them to the versions it published, so a renewed
// transaction carrying those reads does not conflict with its own commit. The
// caller holds tasks.mu.
func (tx *StoreTxn) publishTaskRowsLocked() []PublishedTaskRow {
	if len(tx.taskWrites) == 0 {
		return nil
	}
	published := make([]PublishedTaskRow, 0, len(tx.taskWrites))
	for id, row := range tx.taskWrites {
		row.Version = tx.store.tasks.putLocked(id, row)
		if _, read := tx.taskReads[id]; read {
			tx.taskReads[id] = row.Version
		}
		published = append(published, PublishedTaskRow{ID: id, TaskRow: row})
	}
	clear(tx.taskWrites)
	return published
}

// commitTaskRowsOnly commits a transaction whose only writes are task rows. It
// validates only the task-row reads: the rest of the transaction wrote
// nothing, so, like a read-only transaction, it is serialized at its snapshot.
// It still takes the shared commit gate, so an attempt holding the gate
// exclusively cannot lose to it.
func (tx *StoreTxn) commitTaskRowsOnly() (published []PublishedTaskRow, errCode types.ErrorCode) {
	if !tx.gateExempt {
		grant, err := tx.store.commitGate.Acquire(tx.commitContextOrBackground(), commitgate.Shared)
		if err != nil {
			return nil, tx.markTerminal(types.E_INTRPT)
		}
		defer grant.Release()
	} else if !tx.exclusiveGrant.Owns(&tx.store.commitGate, commitgate.Exclusive) {
		panic("commit with released exclusive grant")
	}
	tx.validationFail = false
	tx.conflicts = tx.conflicts[:0]
	s := tx.store
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	errCode = types.E_NONE
	tx.validateTaskReadsLocked(&errCode)
	if errCode != types.E_NONE {
		tx.lostValidation()
		return nil, errCode
	}
	return tx.publishTaskRowsLocked(), types.E_NONE
}
