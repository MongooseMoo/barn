package task

import (
	"context"
	"sync"
	"time"

	"barn/kernel"
	"barn/types"
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
	Verb            string        // Verb name as invoked (callers()/task_stack())
	StoredVerb      string        // Verb's stored name spec incl. wildcards; used by printed tracebacks
	VerbLoc         types.ObjID   // Object where verb is defined
	Args            []types.Value // Arguments passed to verb
	LineNumber      int           // Current line number in verb
	SourceLine      string        // Source text at LineNumber (best-effort, for debugging/logging)
	ServerInitiated bool          // True if this is a server-invoked call (do_login_command, etc.)
	IsEvalFrame     bool          // True if this is an eval() infrastructure frame (excluded from tracebacks)
}

// ToList converts an activation frame to a MOO list for callers()
// Format: {this, verb_name, programmer, verb_loc, player, line_number}
// For primitive/anonymous targets, ThisValue carries the real "this" value.
func (a *ActivationFrame) ToList() types.Value {
	thisVal := types.NewObj(a.This)
	if !a.ThisValue.IsNone() {
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
	QueueSeq        int64       // Monotonic enqueue order for deterministic same-time scheduling
	WakeValue       types.Value // Value to return when resumed
	IsExecSuspended bool        // True if suspended by exec() (can't resume, only kill)
	ReadingPlayer   types.ObjID // Player this task is read()ing from (ObjNothing = not reading)

	// For forked tasks
	ForkInfo *types.ForkInfo // Fork information (only for forked tasks)
	IsForked bool            // True if this is a forked task

	// Execution fields (use interface{} to avoid circular imports)
	Code           interface{}         // []parser.Stmt - parsed code compiled on first run
	BytecodeVM     interface{}         // *vm.VM - bytecode VM for execution (saved across suspend/resume)
	Context        *kernel.TaskContext // Task execution context
	Result         types.Result        // Last execution result
	ForkCreator    ForkCreator         // For creating forked tasks
	CancelFunc     context.CancelFunc  // For cancellation (exported for scheduler)
	ExecCancelFunc context.CancelFunc  // For cancelling an exec() subprocess
	StmtIndex      int                 // Current statement index (for suspend/resume)

	// Verb context (set for verb tasks)
	VerbName            string
	VerbLoc             types.ObjID   // Object where verb is defined (for traceback)
	This                types.ObjID   // Object this verb is called on
	Caller              types.ObjID   // Object that invoked the verb
	Argstr              string        // Full argument string
	Args                []string      // Arguments as word list
	VerbArgsValues      []types.Value // Typed arguments for server-initiated verb calls
	Dobjstr             string        // Direct object string
	Dobj                types.ObjID   // Direct object
	Prepstr             string        // Preposition string
	Iobjstr             string        // Indirect object string
	Iobj                types.ObjID   // Indirect object
	CommandOutputSuffix string        // Connection output suffix for raw command framing
	FromCommand         bool          // True if dispatched by the command parser (top-level command verb)
	Done                chan struct{} // Closed when task finishes; nil if fire-and-forget

	// OnComplete, when set, is invoked exactly once with the task's terminal
	// result after the task finishes (returns or raises) — never on a suspend
	// or fork re-queue. Used to defer server-hook completion (e.g. logging a
	// player in once a read()-based do_login_command finally returns a player).
	OnComplete func(Result types.Result)

	// For compatibility with old server.Task
	Programmer types.ObjID // Permission context (usually same as Owner)

	doneClosed bool // guards Done against double-close

	mu sync.RWMutex
}

// CloseDone closes the task's Done channel exactly once. It is a no-op when
// Done is nil or has already been closed, so callers may invoke it on every
// terminal-state transition without risking a close-of-closed-channel panic.
func (t *Task) CloseDone() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.Done == nil || t.doneClosed {
		return
	}
	t.doneClosed = true
	close(t.Done)
}

// NewTask creates a new task
func NewTask(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) *Task {
	now := time.Now()
	return &Task{
		ID:            id,
		Owner:         owner,
		Programmer:    owner,     // Default programmer is owner
		Kind:          TaskInput, // Default to input task
		State:         TaskCreated,
		StartTime:     now,
		QueueTime:     now,
		TicksUsed:     0,
		TicksLimit:    tickLimit,
		SecondsUsed:   0,
		SecondsLimit:  secondsLimit,
		CallStack:     make([]ActivationFrame, 0),
		TaskLocal:     types.NewEmptyMap(), // Default task_local is empty map (matches ToastStunt)
		WakeValue:     types.NewInt(0),     // Default wake value is 0 (matches LambdaMOO)
		ReadingPlayer: types.ObjNothing,
		Dobj:          types.ObjNothing, // Default to #-1 (NOTHING), matching Toast
		Iobj:          types.ObjNothing, // Default to #-1 (NOTHING), matching Toast
	}
}

// NewTaskFull creates a task with full execution context.
func NewTaskFull(id int64, owner types.ObjID, code interface{}, tickLimit int64, secondsLimit float64) *Task {
	ctx := kernel.NewTaskContext()
	ctx.Player = owner
	ctx.Programmer = owner
	ctx.TicksRemaining = tickLimit

	now := time.Now()
	t := &Task{
		ID:            id,
		Owner:         owner,
		Programmer:    owner,
		Kind:          TaskInput,
		State:         TaskCreated,
		StartTime:     now,
		QueueTime:     now,
		TicksUsed:     0,
		TicksLimit:    tickLimit,
		SecondsUsed:   0,
		SecondsLimit:  secondsLimit,
		CallStack:     make([]ActivationFrame, 0),
		TaskLocal:     types.NewEmptyMap(), // Default task_local is empty map (matches ToastStunt)
		WakeValue:     types.NewInt(0),
		ReadingPlayer: types.ObjNothing,
		Dobj:          types.ObjNothing, // Default to #-1 (NOTHING), matching Toast
		Iobj:          types.ObjNothing, // Default to #-1 (NOTHING), matching Toast
		Code:          code,
		Context:       ctx,
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

// UpdateCallStackLineNumbers bulk-updates line numbers for all frames in the
// call stack.  lineNumbers[0] corresponds to CallStack[0], etc.  Extra
// entries in either slice are ignored.
func (t *Task) UpdateCallStackLineNumbers(lineNumbers []int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := 0; i < len(lineNumbers) && i < len(t.CallStack); i++ {
		t.CallStack[i].LineNumber = lineNumbers[i]
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

// BytecodeVMValue returns the saved bytecode VM handle (thread-safe).
//
// BytecodeVM is read by the scheduler from goroutines other than the one
// running the task (e.g. liveTaskVMs scanning sibling tasks for orphan-anonymous
// GC, and ProcessReadyTasks' readiness check), so every access must hold t.mu.
// This accessor is a leaf with respect to the scheduler's s.mu: it never
// acquires any other lock, so the scheduler may safely hold s.mu while calling
// it (lock order: s.mu -> task.mu, never the reverse).
func (t *Task) BytecodeVMValue() interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.BytecodeVM
}

// SetBytecodeVM stores (or clears, with nil) the bytecode VM handle (thread-safe).
// See BytecodeVMValue for the locking rationale and ordering.
func (t *Task) SetBytecodeVM(machine interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.BytecodeVM = machine
}

// IndefiniteSuspendStartTime is the sentinel StartTime stamped on an
// indefinitely-suspended task (suspend() with no/negative seconds). It mirrors
// ToastStunt's enqueue_suspended_task, which sets start_tv.tv_sec = INTNUM_MAX
// for an indefinite suspend (tasks.cc:1306-1307) so the task sorts AFTER every
// timed task in queued_tasks(). It is a far-future instant so the ascending
// queued_tasks comparator orders such tasks last. WakeTime is deliberately left
// zero so the scheduler never auto-wakes it — only an explicit resume() does.
var IndefiniteSuspendStartTime = time.Unix(1<<62, 0)

// Suspend suspends the task for a duration
func (t *Task) Suspend(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.State = TaskSuspended
	if duration > 0 {
		t.WakeTime = time.Now().Add(duration)
	}
}

// SuspendIndefinite suspends the task with no wake deadline (suspend() with
// no/negative seconds). The task waits for an explicit resume() and must never
// auto-wake, so WakeTime stays zero. StartTime is stamped with the far-future
// IndefiniteSuspendStartTime sentinel so the task sorts LAST in queued_tasks()
// (ascending by start time), matching ToastStunt's INTNUM_MAX start_tv.
func (t *Task) SuspendIndefinite() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.State = TaskSuspended
	t.StartTime = IndefiniteSuspendStartTime
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
	// An indefinitely-suspended task carries the far-future
	// IndefiniteSuspendStartTime sentinel so it sorts last in queued_tasks().
	// Once explicitly resumed it must become runnable now, but the scheduler's
	// readiness gate keys off StartTime (scheduler.go: !StartTime.After(now)),
	// so clear the sentinel back to now. runTask() re-stamps StartTime when the
	// resumed VM actually runs, so this only affects the brief queued window.
	if t.StartTime.Equal(IndefiniteSuspendStartTime) {
		t.StartTime = time.Now()
	}
	return true
}

// CompleteExec resumes an exec-suspended task with the subprocess result.
// Unlike Resume(), this works even when IsExecSuspended is true.
// Called from the exec goroutine when the subprocess finishes.
func (t *Task) CompleteExec(value types.Value) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State != TaskSuspended {
		return false
	}
	t.IsExecSuspended = false
	t.ExecCancelFunc = nil
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
	// If the task is exec-suspended, cancel the subprocess
	if t.ExecCancelFunc != nil {
		t.ExecCancelFunc()
		t.ExecCancelFunc = nil
	}
	t.IsExecSuspended = false
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
	thisVal := types.None // explicit absence; zero Value{} is integer 0, not None
	var programmer types.ObjID

	if len(t.CallStack) > 0 {
		topFrame := t.CallStack[len(t.CallStack)-1]
		verbName = topFrame.Verb
		verbLoc = topFrame.VerbLoc
		lineNumber = topFrame.LineNumber
		programmer = topFrame.Programmer
		thisObj = topFrame.This // Always use object ID (#-1 for primitives)
		thisVal = topFrame.ThisValue
	} else {
		// Fallback if no call stack
		verbName = t.VerbName
		verbLoc = t.VerbLoc
		lineNumber = 1
		programmer = t.Owner
		thisObj = t.This
	}
	if thisVal.IsNone() {
		thisVal = types.NewObj(thisObj)
	}

	// Estimate bytes (0 for now, can be calculated later if needed)
	bytes := int64(0)

	return types.NewList([]types.Value{
		types.NewInt(t.ID),               // [1] task_id
		types.NewInt(t.StartTime.Unix()), // [2] start_time
		types.NewInt(0),                  // [3] obsolete clock ID
		types.NewInt(30000),              // [4] DEFAULT_BG_TICKS (obsolete)
		types.NewObj(programmer),         // [5] programmer
		types.NewObj(verbLoc),            // [6] verb_loc
		types.NewStr(verbName),           // [7] verb_name
		types.NewInt(int64(lineNumber)),  // [8] line_number
		thisVal,                          // [9] this
		types.NewInt(bytes),              // [10] bytes
	})
}
