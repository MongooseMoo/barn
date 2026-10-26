package engine

import (
	"sync"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// TaskManager tracks the tasks owned by one execution engine.
type TaskManager struct {
	tasks           map[int64]*Task
	mu              sync.RWMutex
	scheduleChanged chan struct{}
	resume          func(t *Task, value types.Value, readier *task.State) bool

	// unfinished is the catalog's tasks that are neither completed nor killed,
	// kept exact by State's settlement notifications. unfinishedMu is a leaf lock: it is
	// taken under a task's lock and never the other way round.
	unfinishedMu sync.Mutex
	unfinished   map[int64]*Task
}

// SetResumeScheduler routes resume() through the runtime scheduler, which
// orders the resumed task among ready work. Without one, resume() only marks
// the task queued.
func (m *TaskManager) SetResumeScheduler(resume func(t *Task, value types.Value, readier *task.State) bool) {
	m.resume = resume
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if replaced := m.tasks[t.ID]; replaced != nil && replaced != t {
		m.forget(replaced)
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
// caller holds t's lock.
func (m *TaskManager) noteFinished(t *Task, finished bool) {
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

// GetQueuedTasks returns all queued (waiting) tasks
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
		if state == task.TaskQueued || state == task.TaskSuspended {
			tasks = append(tasks, &candidate.State)
		}
	}
	return tasks
}

// KillTask kills a task by ID
// Returns ErrorCode if task doesn't exist, already killed, or caller doesn't have permission
func (m *TaskManager) KillTask(taskID int64, killerID types.ObjID, isWizard bool) types.ErrorCode {
	candidate, errCode := m.killable(taskID, killerID, isWizard)
	if errCode != types.E_NONE {
		return errCode
	}
	candidate.Kill()
	return types.E_NONE
}

// CheckKill reports the error KillTask would return without killing anything.
func (m *TaskManager) CheckKill(taskID int64, killerID types.ObjID, isWizard bool) types.ErrorCode {
	_, errCode := m.killable(taskID, killerID, isWizard)
	return errCode
}

func (m *TaskManager) killable(taskID int64, killerID types.ObjID, isWizard bool) (*Task, types.ErrorCode) {
	candidate := m.GetTask(taskID)
	if candidate == nil {
		return nil, types.E_INVARG
	}

	// Check if task is already killed
	if candidate.GetState() == task.TaskKilled {
		return nil, types.E_INVARG
	}

	// Permission check: must be task owner or wizard
	if candidate.Owner != killerID && !isWizard {
		return nil, types.E_PERM
	}
	return candidate, types.E_NONE
}

// ResumeTask resumes a suspended task with a value on behalf of task readier.
func (m *TaskManager) ResumeTask(taskID int64, value types.Value, resumerID types.ObjID, isWizard bool, readier *task.State) types.ErrorCode {
	candidate := m.GetTask(taskID)
	if candidate == nil {
		return types.E_INVARG
	}

	// Permission check: must be task owner or wizard
	if candidate.Owner != resumerID && !isWizard {
		return types.E_PERM
	}

	if candidate.GetState() != task.TaskSuspended {
		if candidate.ReplaceWakeValue(value) {
			return types.E_NONE
		}
		return types.E_INVARG
	}

	resumed := false
	if m.resume != nil {
		resumed = m.resume(candidate, value, readier)
	} else {
		resumed = candidate.Resume(value)
	}
	if !resumed {
		return types.E_INVARG
	}

	return types.E_NONE
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
