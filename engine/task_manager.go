package engine

import (
	"sync"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// TaskManager tracks the tasks owned by one execution engine.
type TaskManager struct {
	tasks           map[int64]*Task
	mu              sync.RWMutex
	scheduleChanged chan struct{}

	// unfinished is the catalog's tasks that are neither completed nor killed,
	// kept exact by State's settlement notifications. unfinishedMu is a leaf lock: it is
	// taken under a task's lock and never the other way round.
	unfinishedMu sync.Mutex
	unfinished   map[int64]*Task

	// rows, when set, holds each registered task's store row: a task gets one
	// when it is registered and loses it when it finishes.
	rows *dbstore.Store
}

// SetTaskRows gives each task registered from now on a row in store.
func (m *TaskManager) SetTaskRows(store *dbstore.Store) {
	m.rows = store
}

// registrationRow is the row a task gets when it is registered.
func registrationRow(t *Task) (dbstore.TaskRow, bool) {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	row := dbstore.TaskRow{Owner: t.Owner, Status: dbstore.TaskRowRunning}
	switch {
	case t.ID == 0 || t.Status == task.TaskCompleted || t.Status == task.TaskKilled:
		return row, false
	case t.Status == task.TaskSuspended && !t.IsExecSuspended:
		row.Status = dbstore.TaskRowSuspended
	}
	return row, true
}

// NewTaskManager creates an empty task manager for one execution engine.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:           make(map[int64]*Task),
		unfinished:      make(map[int64]*Task),
		scheduleChanged: make(chan struct{}, 1),
	}
}

// ScheduleChanged requests a fresh readiness scan. Notifications coalesce; the
// catalog is the source of truth. This channel is never closed, so late external
// completions remain safe after the runtime stops listening.
func (m *TaskManager) ScheduleChanged() <-chan struct{} { return m.scheduleChanged }

func (m *TaskManager) NotifyScheduleChange() {
	select {
	case m.scheduleChanged <- struct{}{}:
	default:
	}
}

// GetTask retrieves a task by ID
func (m *TaskManager) GetTask(id int64) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[id]
}

// RegisterTask registers an externally created task with the manager
// This allows builtins to find tasks created by the execution runtime.
func (m *TaskManager) RegisterTask(t *Task) {
	m.registerTask(t, true)
}

// registerTask registers t, giving it its row now when withRow is set. A
// forked child is registered without one: its row is written by its parent's
// commit.
func (m *TaskManager) registerTask(t *Task, withRow bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if replaced := m.tasks[t.ID]; replaced != nil && replaced != t {
		m.forget(replaced)
	}
	if row, ok := registrationRow(t); ok && withRow && m.rows != nil {
		t.NoteRowVersion(m.rows.PutTaskRow(t.ID, row))
	}
	m.tasks[t.ID] = t
	t.Mu.Lock()
	t.BindSchedulingLocked(t, m.scheduleChanged)
	t.catalog = m
	m.noteFinished(t, t.SettledLocked())
	t.Mu.Unlock()
	m.NotifyScheduleChange()
}

// noteFinished records that scheduling is done with t, or that it is not. The
// caller holds t's lock. A finished task's row goes: one whose last commit did
// not remove it (a panic, a failed commit, a kill) must not be found by
// kill_task() or resume().
func (m *TaskManager) noteFinished(t *Task, finished bool) {
	if finished && m.rows != nil && t.ID != 0 {
		m.rows.RemoveTaskRow(t.ID)
	}
	m.unfinishedMu.Lock()
	defer m.unfinishedMu.Unlock()
	if !finished {
		m.unfinished[t.ID] = t
	} else if m.unfinished[t.ID] == t {
		delete(m.unfinished, t.ID)
	}
}

// forget detaches a task that has left the catalog, so a later state change
// does not put it back among the unfinished. The caller holds m.mu.
func (m *TaskManager) forget(t *Task) {
	t.Mu.Lock()
	if t.catalog == m {
		t.catalog = nil
	}
	m.noteFinished(t, true)
	t.Mu.Unlock()
}

// RemoveTask removes a task from the manager
func (m *TaskManager) RemoveTask(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.tasks[id]; t != nil {
		m.forget(t)
	}
	delete(m.tasks, id)
}

// RemoveTaskIf removes id only when it still names expected. It is used by
// lifecycle cleanup paths which must not remove a newer task reusing the ID.
func (m *TaskManager) RemoveTaskIf(id int64, expected *Task) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tasks[id] != expected {
		return false
	}
	if expected != nil {
		m.forget(expected)
	}
	delete(m.tasks, id)
	return true
}

// Snapshot returns a stable copy of the manager's current task catalog.
func (m *TaskManager) Snapshot() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tasks := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// Unfinished returns the tasks scheduling still has to consider: every task
// that is neither completed nor killed, and every finished task whose slice is
// still executing, because PendingReadier holds its readier until that slice
// ends. The rest stay in the catalog until the periodic cleanup, and under load
// they outnumber live ones a hundred to one, so a scheduling scan must not walk
// them: this reads the index and takes no task lock.
func (m *TaskManager) Unfinished() []*Task {
	m.unfinishedMu.Lock()
	defer m.unfinishedMu.Unlock()
	tasks := make([]*Task, 0, len(m.unfinished))
	for _, t := range m.unfinished {
		tasks = append(tasks, t)
	}
	return tasks
}

// Len returns the number of tasks in the catalog.
func (m *TaskManager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tasks)
}

// GetAllTasks returns all tasks (for debugging)
func (m *TaskManager) GetAllTasks() []*task.State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	states := make([]*task.State, 0, len(m.tasks))
	for _, t := range m.tasks {
		states = append(states, &t.State)
	}
	return states
}

// GetQueuedTasks returns the tasks Toast's queued_tasks() lists: queued and
// suspended tasks, and running tasks Toast would hold in a background queue.
// The caller excludes its own task.
func (m *TaskManager) GetQueuedTasks() []*task.State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*task.State, 0)
	for _, candidate := range m.tasks {
		state := candidate.GetState()
		if state == task.TaskSuspended {
			candidate.Mu.RLock()
			evalScaffold := candidate.IsForked && candidate.VerbName == "" && !candidate.IsExecSuspended
			candidate.Mu.RUnlock()
			if evalScaffold {
				continue
			}
		}
		if state == task.TaskQueued || state == task.TaskSuspended || candidate.RunningInBackground() {
			tasks = append(tasks, &candidate.State)
		}
	}
	return tasks
}

// FindReadingTask returns the oldest suspended task that is read()ing from the
// given player. QueueSeq provides FIFO order; task ID breaks equal-sequence ties.
// Returns nil if no task is currently reading from that player.
func (m *TaskManager) FindReadingTask(player types.ObjID) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var oldest *Task
	var oldestQueueSeq, oldestID int64
	for _, t := range m.tasks {
		queueSeq, id, ok := t.ReadingOrder(player)
		if !ok {
			continue
		}
		if oldest == nil || queueSeq < oldestQueueSeq || queueSeq == oldestQueueSeq && id < oldestID {
			oldest = t
			oldestQueueSeq = queueSeq
			oldestID = id
		}
	}
	return oldest
}

// CleanupCompletedTasks removes completed and killed tasks
// Should be called periodically
func (m *TaskManager) CleanupCompletedTasks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, candidate := range m.tasks {
		state := candidate.GetState()
		if state == task.TaskCompleted || state == task.TaskKilled {
			// Keep tasks for a while for debugging, but eventually remove them
			// For now, remove immediately
			m.forget(candidate)
			delete(m.tasks, id)
		}
	}
}

// FindTaskState exposes language-visible state to builtin consumers.
func (m *TaskManager) FindTaskState(id int64) *task.State {
	if t := m.GetTask(id); t != nil {
		return &t.State
	}
	return nil
}

// FindReadingTaskState exposes the selected reader's language-visible state.
func (m *TaskManager) FindReadingTaskState(player types.ObjID) *task.State {
	if t := m.FindReadingTask(player); t != nil {
		return &t.State
	}
	return nil
}
