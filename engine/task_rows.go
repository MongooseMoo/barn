package engine

import (
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Each task has a row in the store (dbstore.TaskRow) holding what MOO code in
// another task can see or change of it. kill_task() and resume() stage writes
// to it; a task's own slices read it when they start and write their end state
// in their commit, so a kill or resume that commits during a slice makes the
// slice lose its commit and re-run against it.

// applyTaskRows is the store's task-row observer: it applies the rows a commit
// published to the scheduler. Each task records the newest row version it has
// seen, and a row no newer than that is skipped, so a late application cannot
// undo a transition that followed it.
func (s *Runtime) applyTaskRows(rows []dbstore.PublishedTaskRow) {
	now := time.Now()
	for _, row := range rows {
		t := s.taskManager.GetTask(row.ID)
		if t == nil {
			continue
		}
		switch row.Status {
		case dbstore.TaskRowKilled:
			if t.KillAtRow(row.Version) {
				s.session.CancelHTTPReadTask(row.ID)
			}
		case dbstore.TaskRowQueued:
			var readier *task.State
			if by := s.taskManager.GetTask(row.By); by != nil && row.By != 0 {
				readier = &by.State
			}
			s.scheduler.ResumeAtRow(t, row.Version, row.Value, readier, now)
		default:
			t.NoteRowVersion(row.Version)
		}
	}
}

// readOwnRow reads t's row at the start of an attempt, so the attempt's
// commit validates it. A task without a row (a server hook, or one never
// registered) has nothing to read.
func (s *Runtime) readOwnRow(t *Task, txn *dbstore.StoreTxn) (dbstore.TaskRow, bool) {
	if t.ID == 0 || txn == nil {
		return dbstore.TaskRow{}, false
	}
	row, ok := txn.ReadTaskRow(t.ID)
	if ok {
		t.NoteRowVersion(row.Version)
	}
	return row, ok
}

// ownEndRow is the row a slice that ended with result commits for its task.
// Any end but a suspension finishes it. A task waiting in suspend() can be
// resumed, and kill_task() and resume() check the programmer of the verb it is
// suspended in (Toast tasks.cc kill_task and do_resume, progr_of_cur_verb). A
// task waiting in read() or read_http() is its connection's reading task:
// resume() cannot reach it, and kill_task() checks the connection's player.
// One waiting on exec() cannot be resumed either.
func ownEndRow(t *Task, row dbstore.TaskRow, result types.Result) dbstore.TaskRow {
	end := dbstore.TaskRow{Owner: row.Owner, By: t.ID}
	if result.Flow != types.FlowSuspend {
		return end
	}
	if t.Context != nil {
		end.Owner = t.Context.Programmer
	}
	end.Status = dbstore.TaskRowSuspended
	t.Mu.RLock()
	switch {
	case t.ReadingPlayer != types.ObjNothing:
		end.Status = dbstore.TaskRowRunning
		end.Owner = t.ReadingPlayer
	case t.IsHTTPReadSuspended, t.IsExecSuspended:
		end.Status = dbstore.TaskRowRunning
	}
	t.Mu.RUnlock()
	return end
}

// ownRowKilled reports whether t's published row says kill_task() killed it.
func (s *Runtime) ownRowKilled(t *Task) bool {
	row, ok := s.store.TaskRow(t.ID)
	return ok && row.Status == dbstore.TaskRowKilled
}
