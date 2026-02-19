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
	TaskInput         TaskKind = iota // User command input task
	TaskForked                        // Background forked task
	TaskSuspendedTask                 // Suspended task (for resume)
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
	This            types.ObjID   // Object this verb is called on (prototype for primitives)
	ThisValue       types.Value   // For primitive prototype calls: the actual primitive value
	Player          types.ObjID   // Player who initiated this task
	Programmer      types.ObjID   // Programmer (for permissions)
	Caller          types.ObjID   // Object that called this verb
	Verb            string        // Verb name
	VerbLoc         types.ObjID   // Object where verb is defined
	Args            []types.Value // Arguments passed to verb
	LineNumber      int           // Current line number in verb
	ServerInitiated bool          // True if this is a server-invoked call (do_login_command, etc.)
}

// ToList converts an activation frame to a MOO list for callers()
// Format: {this, verb_name, programmer, verb_loc, player, line_number}
// For primitive/anonymous targets, ThisValue carries the real "this" value.
func (a *ActivationFrame) ToList() types.Value {
	thisVal := types.Value(types.NewObj(a.This))
	if a.ThisValue != nil {
		thisVal = a.ThisValue
	}

	return types.NewList([]types.Value{
		thisVal,
		types.NewStr(a.Verb),
		types.NewObj(a.Programmer),
		types.NewObj(a.VerbLoc),
		types.NewObj(a.Player),
		types.NewInt(int64(a.LineNumber)),
	})
}

// ToMap converts an activation frame to a MOO map for task_stack()
// Keys: "this", "verb", "programmer", "verb_loc", "player", "line_number"
// Note: For primitive prototype calls, 'this' is #-1 (matching Toast).
func (a *ActivationFrame) ToMap() types.Value {
	return types.NewMap([][2]types.Value{
		{types.NewStr("this"), types.NewObj(a.This)}, // Always use object ID (#-1 for primitives)
		{types.NewStr("verb"), types.NewStr(a.Verb)},
		{types.NewStr("programmer"), types.NewObj(a.Programmer)},
		{types.NewStr("verb_loc"), types.NewObj(a.VerbLoc)},
		{types.NewStr("player"), types.NewObj(a.Player)},
		{types.NewStr("line_number"), types.NewInt(int64(a.LineNumber))},
	})
}

// Task represents a MOO task (unit of execution)
type Task struct {
	ID           int64
	Owner        types.ObjID
	Kind         TaskKind // Type of task (input, forked, suspended)
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
	WakeTime        time.Time
	WakeValue       types.Value // Value to return when resumed
	IsExecSuspended bool        // True if suspended by exec() (can't resume, only kill)

	// For forked tasks
	ForkInfo *types.ForkInfo // Fork information (only for forked tasks)
	IsForked bool            // True if this is a forked task

	// Execution fields (use interface{} to avoid circular imports)
	Code        interface{}        // []parser.Stmt - actual code to execute
	Evaluator   interface{}        // *vm.Evaluator - evaluator for execution
	BytecodeVM  interface{}        // *vm.VM - bytecode VM for execution (saved across suspend/resume)
	Context     *types.TaskContext // Task execution context
	Result      types.Result       // Last execution result
	ForkCreator ForkCreator        // For creating forked tasks
	CancelFunc  context.CancelFunc // For cancellation (exported for scheduler)
	StmtIndex   int                // Current statement index (for suspend/resume)

	// Verb context (set for verb tasks)
	VerbName            string
	VerbLoc             types.ObjID // Object where verb is defined (for traceback)
	This                types.ObjID // Object this verb is called on
	Caller              types.ObjID // Object that invoked the verb
	Argstr              string      // Full argument string
	Args                []string    // Arguments as word list
	Dobjstr             string      // Direct object string
	Dobj                types.ObjID // Direct object
	Prepstr             string      // Preposition string
	Iobjstr             string      // Indirect object string
	Iobj                types.ObjID // Indirect object
	CommandOutputSuffix string      // Connection output suffix for raw command framing

	// For compatibility with old server.Task
	Programmer types.ObjID // Permission context (usually same as Owner)

	mu sync.RWMutex
}

// NewTask creates a new task
func NewTask(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) *Task {
	now := time.Now()
	return &Task{
		ID:           id,
		Owner:        owner,
		Programmer:   owner,     // Default programmer is owner
		Kind:         TaskInput, // Default to input task
		State:        TaskCreated,
		StartTime:    now,
		QueueTime:    now,
		TicksUsed:    0,
		TicksLimit:   tickLimit,
		SecondsUsed:  0,
		SecondsLimit: secondsLimit,
		CallStack:    make([]ActivationFrame, 0),
		TaskLocal:    types.NewEmptyMap(), // Default task_local is empty map (matches ToastStunt)
		WakeValue:    types.NewInt(0),     // Default wake value is 0 (matches LambdaMOO)
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
		TaskLocal:    types.NewEmptyMap(), // Default task_local is empty map (matches ToastStunt)
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

// UpdateLineNumber updates the line number of the top activation frame
func (t *Task) UpdateLineNumber(line int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.CallStack) > 0 {
		t.CallStack[len(t.CallStack)-1].LineNumber = line
	}
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
// Returns false if task is not suspended or is exec-suspended
func (t *Task) Resume(value types.Value) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State != TaskSuspended {
		return false
	}
	// Can't resume exec-suspended tasks - they must complete on their own or be killed
	if t.IsExecSuspended {
		return false
	}
	t.State = TaskQueued
	t.WakeValue = value
	return true
}

// WakeDue reports whether a suspended task has a timed wake deadline due.
func (t *Task) WakeDue(now time.Time) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.State == TaskSuspended && !t.WakeTime.IsZero() && !t.WakeTime.After(now)
}

// Kill kills the task
func (t *Task) Kill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.State = TaskKilled
}

// ToQueuedTaskInfo returns task info for queued_tasks()
// Format: {task_id, start_time, clock_id, bg_ticks, programmer, verb_loc, verb_name, line, this, bytes}
// Note: For primitive prototype calls, 'this' is #-1 (matching Toast).
func (t *Task) ToQueuedTaskInfo() types.Value {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Get information from the top frame if call stack exists
	var verbName string
	var verbLoc types.ObjID
	var lineNumber int
	var thisObj types.ObjID
	var programmer types.ObjID

	if len(t.CallStack) > 0 {
		topFrame := t.CallStack[len(t.CallStack)-1]
		verbName = topFrame.Verb
		verbLoc = topFrame.VerbLoc
		lineNumber = topFrame.LineNumber
		programmer = topFrame.Programmer
		thisObj = topFrame.This // Always use object ID (#-1 for primitives)
	} else {
		// Fallback if no call stack
		verbName = t.VerbName
		verbLoc = t.VerbLoc
		lineNumber = 1
		programmer = t.Owner
		thisObj = t.This
	}

	// Estimate bytes (0 for now, can be calculated later if needed)
	bytes := int64(0)

	return types.NewList([]types.Value{
		types.NewInt(t.ID),               // [1] task_id
		types.NewInt(t.QueueTime.Unix()), // [2] start_time
		types.NewInt(0),                  // [3] obsolete clock ID
		types.NewInt(30000),              // [4] DEFAULT_BG_TICKS (obsolete)
		types.NewObj(programmer),         // [5] programmer
		types.NewObj(verbLoc),            // [6] verb_loc
		types.NewStr(verbName),           // [7] verb_name
		types.NewInt(int64(lineNumber)),  // [8] line_number
		types.NewObj(thisObj),            // [9] this (always OBJ, #-1 for primitives)
		types.NewInt(bytes),              // [10] bytes
	})
}
