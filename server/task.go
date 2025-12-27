package server

import (
	"barn/vm"
	"barn/parser"
	"barn/types"
	"context"
	"errors"
	"sync"
	"time"
)

// TaskState represents the current state of a task
type TaskState int

const (
	TaskCreated TaskState = iota
	TaskWaiting
	TaskRunning
	TaskSuspended
	TaskCompleted
	TaskAborted
)

// Task represents a MOO task
type Task struct {
	ID            int64
	State         TaskState
	Player        types.ObjID
	Programmer    types.ObjID // Permission context
	StartTime     time.Time
	TicksUsed     int
	TickLimit     int
	TimeLimit     time.Duration
	Deadline      time.Time
	TaskLocal     map[types.Value]types.Value
	WakeChannel   chan types.Value // For suspension/resumption
	Context       *types.TaskContext
	Evaluator     *vm.Evaluator
	Code          []parser.Stmt // Code to execute
	Result        types.Result
	mu            sync.Mutex
	cancelFunc    context.CancelFunc

	// Verb context (set for verb tasks)
	VerbName string
	This     types.ObjID // Object where verb was found
	Caller   types.ObjID // Object that invoked the verb
	Argstr   string      // Full argument string
	Args     []string    // Arguments as word list
	Dobjstr  string      // Direct object string
	Dobj     types.ObjID // Direct object
	Prepstr  string      // Preposition string
	Iobjstr  string      // Indirect object string
	Iobj     types.ObjID // Indirect object
}

// NewTask creates a new task
func NewTask(id int64, player types.ObjID, code []parser.Stmt, tickLimit int, timeLimit time.Duration) *Task {
	ctx := types.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.TicksRemaining = int64(tickLimit)

	return &Task{
		ID:          id,
		State:       TaskCreated,
		Player:      player,
		Programmer:  player,
		TicksUsed:   0,
		TickLimit:   tickLimit,
		TimeLimit:   timeLimit,
		Deadline:    time.Now().Add(timeLimit),
		TaskLocal:   make(map[types.Value]types.Value),
		WakeChannel: make(chan types.Value, 1),
		Context:     ctx,
		Code:        code,
	}
}

// Run executes the task
func (t *Task) Run(ctx context.Context, evaluator *vm.Evaluator) error {
	t.mu.Lock()
	if t.State != TaskWaiting && t.State != TaskCreated {
		t.mu.Unlock()
		return nil // Already running or completed
	}
	t.State = TaskRunning
	t.Evaluator = evaluator
	t.mu.Unlock()

	// Set up verb context if this is a verb task
	if t.VerbName != "" {
		evaluator.SetVerbContext(&vm.VerbContext{
			Player:  t.Player,
			This:    t.This,
			Caller:  t.Caller,
			Verb:    t.VerbName,
			Args:    t.Args,
			Argstr:  t.Argstr,
			Dobj:    t.Dobj,
			Dobjstr: t.Dobjstr,
			Iobj:    t.Iobj,
			Iobjstr: t.Iobjstr,
			Prepstr: t.Prepstr,
		})
		// Also update TaskContext for permissions and builtins
		t.Context.ThisObj = t.This
		t.Context.Verb = t.VerbName
	}

	// Set up cancellation
	taskCtx, cancel := context.WithDeadline(ctx, t.Deadline)
	t.cancelFunc = cancel
	defer cancel()

	// Execute code
	for _, stmt := range t.Code {
		select {
		case <-taskCtx.Done():
			t.mu.Lock()
			t.State = TaskAborted
			t.mu.Unlock()
			return taskCtx.Err()
		default:
		}

		// Check tick limit
		if t.Context.TicksRemaining <= 0 {
			t.mu.Lock()
			t.State = TaskAborted
			t.mu.Unlock()
			return ErrTicksExceeded
		}

		// Execute statement
		result := evaluator.EvalStmt(stmt, t.Context)
		t.Result = result

		// Handle control flow
		if result.Flow == types.FlowReturn || result.Flow == types.FlowException {
			t.mu.Lock()
			if result.Flow == types.FlowException {
				t.State = TaskAborted
			} else {
				t.State = TaskCompleted
			}
			t.mu.Unlock()
			return nil
		}
	}

	t.mu.Lock()
	t.State = TaskCompleted
	t.mu.Unlock()
	return nil
}

// Suspend suspends the task for a duration or indefinitely
func (t *Task) Suspend(duration time.Duration) types.Value {
	t.mu.Lock()
	t.State = TaskSuspended
	t.mu.Unlock()

	if duration > 0 {
		select {
		case value := <-t.WakeChannel:
			t.mu.Lock()
			t.State = TaskRunning
			t.mu.Unlock()
			return value
		case <-time.After(duration):
			t.mu.Lock()
			t.State = TaskRunning
			t.mu.Unlock()
			return types.NewInt(0)
		}
	} else {
		// Wait forever
		value := <-t.WakeChannel
		t.mu.Lock()
		t.State = TaskRunning
		t.mu.Unlock()
		return value
	}
}

// Resume resumes a suspended task
func (t *Task) Resume(value types.Value) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.State != TaskSuspended {
		return ErrNotSuspended
	}

	select {
	case t.WakeChannel <- value:
		return nil
	default:
		return ErrResumeFailed
	}
}

// Kill aborts the task
func (t *Task) Kill() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancelFunc != nil {
		t.cancelFunc()
	}
	t.State = TaskAborted
}

// GetState returns the current task state (thread-safe)
func (t *Task) GetState() TaskState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.State
}

// Errors
var (
	ErrTicksExceeded = errors.New("tick limit exceeded")
	ErrNotSuspended  = errors.New("task not suspended")
	ErrResumeFailed  = errors.New("failed to resume task")
	ErrPermission    = errors.New("permission denied")
)
