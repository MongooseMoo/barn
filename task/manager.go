package task

import (
	"barn/types"
	"sync"
	"sync/atomic"
	"time"
)

// Manager is a global singleton that manages all tasks
type Manager struct {
	tasks      map[int64]*Task
	nextTaskID int64
	mu         sync.RWMutex
}

var (
	globalManager     *Manager
	globalManagerOnce sync.Once
)

// GetManager returns the global task manager singleton
func GetManager() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = &Manager{
			tasks:      make(map[int64]*Task),
			nextTaskID: 1,
		}
	})
	return globalManager
}

// CreateTask creates a new task and adds it to the manager
func (m *Manager) CreateTask(owner types.ObjID, tickLimit int64, secondsLimit float64) *Task {
	id := atomic.AddInt64(&m.nextTaskID, 1)
	task := NewTask(id, owner, tickLimit, secondsLimit)

	m.mu.Lock()
	m.tasks[id] = task
	m.mu.Unlock()

	return task
}

// GetTask retrieves a task by ID
func (m *Manager) GetTask(id int64) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[id]
}

// RegisterTask registers an externally created task with the manager
// This allows builtins to find tasks created by the scheduler
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
	duration := time.Duration(seconds * float64(time.Second))
	task.Suspend(duration)
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
