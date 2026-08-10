package task

import (
	"sync"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Manager tracks the tasks owned by one execution engine.
type Manager struct {
	tasks map[int64]*Task
	mu    sync.RWMutex
}

// NewManager creates an empty task manager for one execution engine.
func NewManager() *Manager {
	return &Manager{tasks: make(map[int64]*Task)}
}

// GetTask retrieves a task by ID
func (m *Manager) GetTask(id int64) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[id]
}

// RegisterTask registers an externally created task with the manager
// This allows builtins to find tasks created by the execution runtime.
func (m *Manager) RegisterTask(t *Task) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[t.ID] = t
}

// RemoveTask removes a task from the manager
func (m *Manager) RemoveTask(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
}

// GetAllTasks returns all tasks (for debugging)
func (m *Manager) GetAllTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// GetQueuedTasks returns all queued (waiting) tasks
func (m *Manager) GetQueuedTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0)
	for _, task := range m.tasks {
		state := task.GetState()
		if state == TaskSuspended {
			task.mu.RLock()
			evalScaffold := task.IsForked && task.VerbName == "" && !task.IsExecSuspended
			task.mu.RUnlock()
			if evalScaffold {
				continue
			}
		}
		if state == TaskQueued || state == TaskSuspended {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

// KillTask kills a task by ID
// Returns ErrorCode if task doesn't exist, already killed, or caller doesn't have permission
func (m *Manager) KillTask(taskID int64, killerID types.ObjID, isWizard bool) types.ErrorCode {
	task := m.GetTask(taskID)
	if task == nil {
		return types.E_INVARG
	}

	// Check if task is already killed
	if task.GetState() == TaskKilled {
		return types.E_INVARG
	}

	// Permission check: must be task owner or wizard
	if task.Owner != killerID && !isWizard {
		return types.E_PERM
	}

	task.Kill()
	return types.E_NONE
}

// ResumeTask resumes a suspended task with a value
func (m *Manager) ResumeTask(taskID int64, value types.Value, resumerID types.ObjID, isWizard bool) types.ErrorCode {
	task := m.GetTask(taskID)
	if task == nil {
		return types.E_INVARG
	}

	// Permission check: must be task owner or wizard
	if task.Owner != resumerID && !isWizard {
		return types.E_PERM
	}

	if task.GetState() != TaskSuspended {
		return types.E_INVARG
	}

	if !task.Resume(value) {
		return types.E_INVARG
	}

	return types.E_NONE
}

// SuspendTask suspends a task for a duration
func (m *Manager) SuspendTask(task *Task, seconds float64) {
	switch {
	case seconds < 0:
		// Indefinite suspension (requires explicit resume()). Stamp a
		// far-future StartTime sentinel so the task sorts LAST in
		// queued_tasks(), mirroring ToastStunt's INTNUM_MAX start_tv for an
		// indefinite suspend (tasks.cc:1306-1307). WakeTime stays zero so it
		// never auto-wakes — only an explicit resume() wakes it.
		task.SuspendIndefinite()
	case seconds == 0:
		// suspend(0) is a scheduler yield point; queue immediately.
		task.Suspend(0)
		_ = task.Resume(types.NewInt(0))
	default:
		duration := time.Duration(seconds * float64(time.Second))
		task.Suspend(duration)
	}
}

// FindReadingTask returns the oldest suspended task that is read()ing from the
// given player. QueueSeq provides FIFO order; task ID breaks equal-sequence ties.
// Returns nil if no task is currently reading from that player.
func (m *Manager) FindReadingTask(player types.ObjID) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var oldest *Task
	var oldestQueueSeq, oldestID int64
	for _, t := range m.tasks {
		queueSeq, id, ok := t.readingOrder(player)
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
func (m *Manager) CleanupCompletedTasks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, task := range m.tasks {
		state := task.GetState()
		if state == TaskCompleted || state == TaskKilled {
			// Keep tasks for a while for debugging, but eventually remove them
			// For now, remove immediately
			delete(m.tasks, id)
		}
	}
}
