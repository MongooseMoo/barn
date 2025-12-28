package task

import (
	"barn/types"
	"context"
	"sync"
	"time"
)

// TaskState represents the current state of a task
type TaskState int

const (
	TaskCreated TaskState = iota
	TaskQueued
	TaskRunning
	TaskSuspended
	TaskCompleted
	TaskKilled
)

// TaskKind represents the type/origin of a task
type TaskKind int

const (
	TaskInput TaskKind = iota  // User command input task
	TaskForked                 // Background forked task
	TaskSuspendedTask          // Suspended task (for resume)
)

// ForkCreator interface allows tasks to create forked children without importing server
type ForkCreator interface {
	CreateForkedTask(parent *Task, info *types.ForkInfo) int64
}

func (s TaskState) String() string {
	switch s {
	case TaskCreated:
		return "created"
	case TaskQueued:
		return "queued"
	case TaskRunning:
		return "running"
	case TaskSuspended:
		return "suspended"
	case TaskCompleted:
		return "completed"
	case TaskKilled:
		return "killed"
	default:
		return "unknown"
	}
}

// ActivationFrame represents a single verb call on the call stack
// This is what callers() returns
type ActivationFrame struct {
	This       types.ObjID   // Object this verb is called on
	Player     types.ObjID   // Player who initiated this task
	Programmer types.ObjID   // Programmer (for permissions)
	Caller     types.ObjID   // Object that called this verb
	Verb       string        // Verb name
	VerbLoc    types.ObjID   // Object where verb is defined
	Args       []types.Value // Arguments passed to verb
	LineNumber int           // Current line number in verb
}

// ToList converts an activation frame to a MOO list for callers()
// Format: {this, verb_name, programmer, verb_loc, player, line_number}
func (a *ActivationFrame) ToList() types.Value {
	return types.NewList([]types.Value{
		types.NewObj(a.This),
		types.NewStr(a.Verb),
		types.NewObj(a.Programmer),
		types.NewObj(a.VerbLoc),
		types.NewObj(a.Player),
		types.NewInt(int64(a.LineNumber)),
	})
}

// Task represents a MOO task (unit of execution)
type Task struct {
	ID           int64
	Owner        types.ObjID
	Kind         TaskKind    // Type of task (input, forked, suspended)
	State        TaskState
	StartTime    time.Time
	QueueTime    time.Time // When task was queued
	TicksUsed    int64
	TicksLimit   int64
	SecondsUsed  float64
	SecondsLimit float64
	CallStack    []ActivationFrame
	TaskLocal    types.Value // Task-local storage (set_task_local/task_local)

	// For suspension/resumption
	WakeTime     time.Time
	WakeValue    types.Value // Value to return when resumed

	// For forked tasks
	ForkInfo     *types.ForkInfo // Fork information (only for forked tasks)
	IsForked     bool            // True if this is a forked task

	// Execution fields (use interface{} to avoid circular imports)
	Code         interface{}    // []parser.Stmt - actual code to execute
	Evaluator    interface{}    // *vm.Evaluator - evaluator for execution
	Context      *types.TaskContext // Task execution context
	Result       types.Result    // Last execution result
	ForkCreator  ForkCreator    // For creating forked tasks
	CancelFunc   context.CancelFunc // For cancellation (exported for scheduler)

	// Verb context (set for verb tasks)
	VerbName     string
	This         types.ObjID // Object this verb is called on
	Caller       types.ObjID // Object that invoked the verb
	Argstr       string      // Full argument string
	Args         []string    // Arguments as word list
	Dobjstr      string      // Direct object string
	Dobj         types.ObjID // Direct object
	Prepstr      string      // Preposition string
	Iobjstr      string      // Indirect object string
	Iobj         types.ObjID // Indirect object

	// For compatibility with old server.Task
	Programmer   types.ObjID // Permission context (usually same as Owner)

	mu           sync.RWMutex
}

// NewTask creates a new task
func NewTask(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) *Task {
	now := time.Now()
	return &Task{
		ID:           id,
		Owner:        owner,
		Programmer:   owner, // Default programmer is owner
		Kind:         TaskInput, // Default to input task
		State:        TaskCreated,
		StartTime:    now,
		QueueTime:    now,
		TicksUsed:    0,
		TicksLimit:   tickLimit,
		SecondsUsed:  0,
		SecondsLimit: secondsLimit,
		CallStack:    make([]ActivationFrame, 0),
		TaskLocal:    types.NewInt(0), // Default task_local is 0
		WakeValue:    types.NewInt(0), // Default wake value is 0 (matches LambdaMOO)
	}
}

// NewTaskFull creates a task with full context (code, evaluator, etc)
func NewTaskFull(id int64, owner types.ObjID, code interface{}, tickLimit int64, secondsLimit float64) *Task {
	ctx := types.NewTaskContext()
	ctx.Player = owner
	ctx.Programmer = owner
	ctx.TicksRemaining = tickLimit

	now := time.Now()
	t := &Task{
		ID:           id,
		Owner:        owner,
		Programmer:   owner,
		Kind:         TaskInput,
		State:        TaskCreated,
		StartTime:    now,
		QueueTime:    now,
		TicksUsed:    0,
		TicksLimit:   tickLimit,
		SecondsUsed:  0,
		SecondsLimit: secondsLimit,
		CallStack:    make([]ActivationFrame, 0),
		TaskLocal:    types.NewInt(0),
		WakeValue:    types.NewInt(0),
		Code:         code,
		Context:      ctx,
	}
	// Set ctx.Task to this task so builtins can access it
	if ctx != nil {
		ctx.Task = t
	}
	return t
}

// GetState returns the current state (thread-safe)
func (t *Task) GetState() TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.State
}

// SetState sets the state (thread-safe)
func (t *Task) SetState(state TaskState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.State = state
}

// PushFrame pushes an activation frame onto the call stack
func (t *Task) PushFrame(frame ActivationFrame) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CallStack = append(t.CallStack, frame)
}

// PopFrame pops an activation frame from the call stack
func (t *Task) PopFrame() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.CallStack) > 0 {
		t.CallStack = t.CallStack[:len(t.CallStack)-1]
	}
}

// GetCallStack returns a copy of the call stack (thread-safe)
func (t *Task) GetCallStack() []ActivationFrame {
	t.mu.RLock()
	defer t.mu.RUnlock()
	// Make a copy
	stack := make([]ActivationFrame, len(t.CallStack))
	copy(stack, t.CallStack)
	return stack
}

// GetTopFrame returns the top frame (current verb being executed)
func (t *Task) GetTopFrame() *ActivationFrame {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.CallStack) == 0 {
		return nil
	}
	return &t.CallStack[len(t.CallStack)-1]
}

// TicksLeft returns remaining ticks
func (t *Task) TicksLeft() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.TicksLimit - t.TicksUsed
}

// SecondsLeft returns remaining seconds
func (t *Task) SecondsLeft() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.SecondsLimit - t.SecondsUsed
}

// ConsumeTick increments tick count and returns true if ticks remain
func (t *Task) ConsumeTick() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TicksUsed++
	return t.TicksUsed < t.TicksLimit
}

// GetTaskLocal returns the task-local value
func (t *Task) GetTaskLocal() types.Value {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.TaskLocal
}

// SetTaskLocal sets the task-local value
func (t *Task) SetTaskLocal(val types.Value) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TaskLocal = val
}

// Suspend suspends the task for a duration
func (t *Task) Suspend(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.State = TaskSuspended
	if duration > 0 {
		t.WakeTime = time.Now().Add(duration)
	}
}

// Resume resumes the task with a value
func (t *Task) Resume(value types.Value) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State != TaskSuspended {
		return false
	}
	t.State = TaskQueued
	t.WakeValue = value
	return true
}

// Kill kills the task
func (t *Task) Kill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.State = TaskKilled
}

// ToQueuedTaskInfo returns task info for queued_tasks()
// Format: {task_id, start_time, x, y, z, programmer, ...}
// For now, simplified version
func (t *Task) ToQueuedTaskInfo() types.Value {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// queued_tasks() returns:
	// {task_id, start_time, x, y, z, programmer, verb_loc, verb_name, line, this}
	// We'll return a simplified version with the key fields
	return types.NewList([]types.Value{
		types.NewInt(t.ID),
		types.NewInt(t.QueueTime.Unix()), // start_time as Unix timestamp
		types.NewInt(0), // x (unused)
		types.NewInt(0), // y (unused)
		types.NewInt(0), // z (unused)
		types.NewObj(t.Owner), // programmer
	})
}
