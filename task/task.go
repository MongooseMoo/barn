package task

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"
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
	CreateForkedTask(parent *State, info *types.ForkInfo) int64
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

// State holds a MOO task's execution context, lifecycle, and language-visible data.
type State struct {
	ID                int64
	Owner             types.ObjID
	Kind              TaskKind // Type of task (input, forked, suspended)
	Status            TaskState
	StartTime         time.Time
	QueueTime         time.Time // When task was queued
	TicksUsed         int64
	TicksLimit        int64
	SecondsUsed       float64
	SecondsLimit      float64
	executionDeadline time.Time
	execReadyAt       time.Time // External completion, retained until its next execution slice.
	CallStack         []types.ActivationFrame
	TaskLocal         types.Value // Task-local storage (set_task_local/task_local)

	// For suspension/resumption
	WakeTime            time.Time
	suspendGen          uint64      // Bumped by every Suspend/SuspendIndefinite; see ResumeGeneration
	QueueSeq            int64       // Monotonic enqueue order for deterministic same-time scheduling
	readier             *State      // Task whose fork or resume() readied this one; retained until physical handoff
	forkFamily          int64       // ID of the unforked task this one descends from; zero for that task itself
	WakeValue           types.Value // Value to return when resumed
	WakeErrorAsValue    bool        // Return an error-typed wake value instead of raising it
	IsExecSuspended     bool        // True if suspended by exec() (can't resume, only kill)
	ExecCommandName     string      // Toast-style executable label while suspended by exec()
	IsHTTPReadSuspended bool        // True while suspended by read_http()
	ReadingPlayer       types.ObjID // Player this task is read()ing from (ObjNothing = not reading)

	// For forked tasks
	ForkInfo     *types.ForkInfo // Fork information (only for forked tasks)
	IsForked     bool            // True if this is a forked task
	CreatedForks []int64         // Forked task IDs created during the current execution slice

	// Execution fields
	Program        *bytecode.Program  // Compiled program ready for execution
	BytecodeVM     interface{}        // *vm.VM - bytecode VM for execution (saved across suspend/resume)
	Context        *Context           // Task execution context
	Result         types.Result       // Last execution result
	ForkCreator    ForkCreator        // For creating forked tasks
	CancelFunc     context.CancelFunc // For cancellation (exported for the engine)
	ExecCancelFunc context.CancelFunc // For cancelling an exec() subprocess
	StmtIndex      int                // Current statement index (for suspend/resume)

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
	IntrinsicEval       bool          // Result is returned to the eval caller, not the uncaught-error hook
	Done                chan struct{} // Closed when task finishes; nil if fire-and-forget

	// OnComplete, when set, is invoked exactly once with the task's terminal
	// result after the task finishes (returns or raises) — never on a suspend
	// or fork re-queue. Used to defer server-hook completion (e.g. logging a
	// player in once a read()-based do_login_command finally returns a player).
	OnComplete func(Result types.Result)
	onFailure  func(error)

	Programmer types.ObjID // Permission context (usually same as Owner)

	doneClosed       bool // guards Done against double-close
	scheduleChanged  chan<- struct{}
	executionActive  bool // runtime-owned physical execution lease
	admissionPending bool // selected by one dispatcher, not yet executing
	pendingInput     int  // injected events that have not finished their first slice
	waitingForInput  bool // only a zero-delay continuation observes this fence
	cancelRequested  bool // explicit kill, distinct from an uncaught MOO exception
	// wokenUnrun marks a queued task woken from a suspension whose slice has not
	// started. Toast still finds it suspended (tasks.cc do_resume searches the
	// background queues), so resume() only replaces its WakeValue.
	wokenUnrun bool

	// catalog is the manager the task is registered with. It is told when the
	// task finishes, and when a finished task is made runnable again.
	catalog *Manager

	// Mu protects state and, when embedded in an executable task, its saved VM.
	Mu sync.RWMutex
}

// ReadingPlayerValue returns the player whose input this task is waiting for.
func (t *State) ReadingPlayerValue() types.ObjID {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.ReadingPlayer
}

// SetReadingPlayer records the player whose input this task is waiting for.
func (t *State) SetReadingPlayer(player types.ObjID) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.ReadingPlayer = player
}

// IsReadingFrom reports whether the task is suspended waiting for player input.
func (t *State) IsReadingFrom(player types.ObjID) bool {
	_, _, ok := t.ReadingOrder(player)
	return ok
}

// ReadingOrder atomically snapshots the deterministic order of a matching
// read()-suspended task. QueueSeq is the primary FIFO key; ID makes selection
// deterministic for tasks that have the same (including default-zero) sequence.
func (t *State) ReadingOrder(player types.ObjID) (queueSeq, id int64, ok bool) {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	if t.Status != TaskSuspended || t.ReadingPlayer != player {
		return 0, 0, false
	}
	return t.QueueSeq, t.ID, true
}

// SetOnComplete installs the callback invoked when the task terminates.
func (t *State) SetOnComplete(onComplete func(Result types.Result)) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.OnComplete = onComplete
}

// TakeOnComplete atomically removes and returns the terminal callback.
// A nil return means that no callback is pending.
func (t *State) TakeOnComplete() func(Result types.Result) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	onComplete := t.OnComplete
	t.OnComplete = nil
	return onComplete
}

func (t *State) SetOnFailure(callback func(error)) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.onFailure = callback
}

func (t *State) TakeOnFailure() func(error) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	callback := t.onFailure
	t.onFailure = nil
	return callback
}

// CloseDone closes the task's Done channel exactly once. It is a no-op when
// Done is nil or has already been closed, so callers may invoke it on every
// terminal-state transition without risking a close-of-closed-channel panic.
func (t *State) CloseDone() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Done == nil || t.doneClosed {
		return
	}
	t.doneClosed = true
	close(t.Done)
}

// NewState creates task state with the standard execution budgets and defaults.
func NewState(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) *State {
	t := &State{}
	t.Init(id, owner, tickLimit, secondsLimit)
	return t
}

// Init initializes fresh, unpublished state in place. This lets its executable
// owner embed State without allocating a second object or copying a used mutex.
// It must not be called after the state has been shared or used.
func (t *State) Init(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64) {
	now := time.Now()
	*t = State{
		ID:            id,
		Owner:         owner,
		Programmer:    owner,     // Default programmer is owner
		Kind:          TaskInput, // Default to input task
		Status:        TaskCreated,
		StartTime:     now,
		QueueTime:     now,
		TicksUsed:     0,
		TicksLimit:    tickLimit,
		SecondsUsed:   0,
		SecondsLimit:  secondsLimit,
		CallStack:     make([]types.ActivationFrame, 0),
		TaskLocal:     types.NewEmptyMap(), // Default task_local is empty map (matches ToastStunt)
		WakeValue:     types.NewInt(0),     // Default wake value is 0 (matches LambdaMOO)
		ReadingPlayer: types.ObjNothing,
		Dobj:          types.ObjNothing, // Default to #-1 (NOTHING), matching Toast
		Iobj:          types.ObjNothing, // Default to #-1 (NOTHING), matching Toast
	}
}

// NewStateFull creates a task with full execution context.
func NewStateFull(id int64, owner types.ObjID, program *bytecode.Program, tickLimit int64, secondsLimit float64) *State {
	ctx := NewContext()
	ctx.Player = owner
	ctx.Programmer = owner
	ctx.TicksRemaining = tickLimit

	t := NewState(id, owner, tickLimit, secondsLimit)
	t.Program = program
	t.Context = ctx
	return t
}

// GetState returns the current state (thread-safe)
func (t *State) GetState() TaskState {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.Status
}

// SetState sets the state (thread-safe)
func (t *State) SetState(state TaskState) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	// Count the transition, not the assignment: a task can be marked killed more
	// than once as an error unwinds, and it only died once.
	if state == TaskKilled && t.Status != TaskKilled {
		metrics.TasksKilled.Add(1)
	}
	t.setStateLocked(state)
	if state != TaskRunning {
		t.notifyScheduleLocked()
	}
}

func (s TaskState) finished() bool {
	return s == TaskCompleted || s == TaskKilled
}

// settledLocked reports whether scheduling is done with t: it has finished and
// its slice is no longer executing. A task that dies with an error is killed
// before its uncaught-error hook runs, and the task that forked it stays held
// until that slice ends, so the state alone does not settle it.
func (t *State) settledLocked() bool {
	return t.Status.finished() && !t.executionActive
}

// setStateLocked is the only place a registered task's state changes, so the
// manager's index of unfinished tasks stays exact. The caller holds t.mu.
func (t *State) setStateLocked(state TaskState) {
	was := t.settledLocked()
	t.Status = state
	if state != TaskQueued {
		t.wokenUnrun = false
	}
	t.noteSettledLocked(was)
}

// setExecutionActiveLocked is the only place the execution lease changes, for
// the same reason. The caller holds t.mu.
func (t *State) setExecutionActiveLocked(active bool) {
	was := t.settledLocked()
	t.executionActive = active
	t.noteSettledLocked(was)
}

func (t *State) noteSettledLocked(was bool) {
	if now := t.settledLocked(); now != was && t.catalog != nil {
		t.catalog.noteFinished(t, now)
	}
}

// Only a nonblocking channel send runs under the task lock; no scheduler or
// runtime callback may acquire locks in the reverse direction.
func (t *State) notifyScheduleLocked() {
	select {
	case t.scheduleChanged <- struct{}{}:
	default:
	}
}

// ReadinessSnapshot explains the task-local gates without changing scheduling.
type ReadinessSnapshot struct {
	State             string    `json:"state"`
	ExecutionActive   bool      `json:"execution_active"`
	AdmissionPending  bool      `json:"admission_pending"`
	WaitingForInput   bool      `json:"waiting_for_input"`
	ExternalReadyAt   time.Time `json:"external_ready_at"`
	SuspendGeneration uint64    `json:"suspend_generation"`
}

func (t *State) ReadinessSnapshot() ReadinessSnapshot {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return ReadinessSnapshot{State: t.Status.String(), ExecutionActive: t.executionActive,
		AdmissionPending: t.admissionPending, WaitingForInput: t.waitingForInput,
		ExternalReadyAt: t.execReadyAt, SuspendGeneration: t.suspendGen}
}

// ReadyDeadline reports the next time this task could be selected. Zero means
// no scheduled wake, including VMs whose physical execution has not ended.
func (t *State) ReadyDeadline(now time.Time) time.Time {
	if t.awaitingReadier() {
		return time.Time{}
	}
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.readyDeadlineLocked(now)
}

func (t *State) readyDeadlineLocked(now time.Time) time.Time {
	if t.executionActive || t.admissionPending || t.waitingForInput {
		return time.Time{}
	}
	if t.Status == TaskSuspended {
		return t.WakeTime
	}
	if t.Status != TaskQueued {
		return time.Time{}
	}
	at := t.StartTime
	if t.WakeTime.After(at) {
		at = t.WakeTime
	}
	if at.Before(now) {
		return now
	}
	return at
}

// SetExecutionActive brackets physical VM execution, which can outlast its
// logical running state. Publishing the handoff must wake a sleeping selector.
func (t *State) SetExecutionActive(active bool) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.setExecutionActiveLocked(active)
	if !active {
		t.readier = nil
		t.notifyScheduleLocked()
	}
}

// StartExecution atomically checks cancellation and publishes VM ownership.
func (t *State) StartExecution() bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status == TaskKilled {
		return false
	}
	t.setExecutionActiveLocked(true)
	t.setStateLocked(TaskRunning)
	return true
}

// TryClaimQueued atomically takes the execution claim for a queued task.
// Dispatchers must claim a task before handing it to runTask so two concurrent
// scheduling paths cannot execute the same saved VM.
func (t *State) TryClaimQueued() bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status != TaskQueued || t.executionActive || t.waitingForInput {
		return false
	}
	t.setStateLocked(TaskRunning)
	return true
}

// ReserveAdmission keeps an unstarted VM inspectable while preventing duplicate
// dispatch by the input and background scheduling paths.
func (t *State) ReserveAdmission() bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status != TaskQueued || t.executionActive || t.admissionPending || t.waitingForInput {
		return false
	}
	t.admissionPending = true
	return true
}

func (t *State) ReleaseAdmission() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.admissionPending = false
	t.notifyScheduleLocked()
}

// PushFrame pushes an activation frame onto the call stack
func (t *State) PushFrame(frame types.ActivationFrame) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.CallStack = append(t.CallStack, frame)
}

// PopFrame pops an activation frame from the call stack
func (t *State) PopFrame() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if len(t.CallStack) > 0 {
		t.CallStack = t.CallStack[:len(t.CallStack)-1]
	}
}

// GetCallStack returns a copy of the call stack (thread-safe)
func (t *State) GetCallStack() []types.ActivationFrame {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	// Make a copy
	stack := make([]types.ActivationFrame, len(t.CallStack))
	copy(stack, t.CallStack)
	return stack
}

// CallStackProgrammer returns the programmer of the frame depth below the top
// (0 is the running frame) and the number of frames, without copying the
// stack. With fewer than depth+1 frames the programmer is ObjNothing.
func (t *State) CallStackProgrammer(depth int) (programmer types.ObjID, frames int) {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	frames = len(t.CallStack)
	if depth < 0 || depth >= frames {
		return types.ObjNothing, frames
	}
	return t.CallStack[frames-1-depth].Programmer, frames
}

// ClearCallStack removes every activation frame atomically.
func (t *State) ClearCallStack() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.CallStack = nil
}

// RetryStateSnapshot returns the task fields needed to reconstruct a failed
// optimistic execution attempt. Slice storage is copied before releasing the
// lock so a concurrent observer never aliases a changing CallStack.
type RetrySnapshot struct {
	Context                              *Context
	CallStack                            []types.ActivationFrame
	TaskLocal, WakeValue                 types.Value
	WakeErrorAsValue                     bool
	TicksLimit                           int64
	SecondsLimit                         float64
	WakeTime                             time.Time
	ReadingPlayer                        types.ObjID
	IsExecSuspended, IsHTTPReadSuspended bool
	ExecCommandName                      string
}

func (t *State) RetryStateSnapshot() RetrySnapshot {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return RetrySnapshot{
		Context: t.Context, CallStack: append([]types.ActivationFrame(nil), t.CallStack...),
		TaskLocal: t.TaskLocal, WakeValue: t.WakeValue, WakeErrorAsValue: t.WakeErrorAsValue,
		TicksLimit: t.TicksLimit, SecondsLimit: t.SecondsLimit, WakeTime: t.WakeTime,
		ReadingPlayer: t.ReadingPlayer, IsExecSuspended: t.IsExecSuspended,
		IsHTTPReadSuspended: t.IsHTTPReadSuspended, ExecCommandName: t.ExecCommandName,
	}
}

// RestoreRetryState atomically publishes all task-owned state for a retry.
func (t *State) RestoreRetryState(saved RetrySnapshot) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.Result = types.Result{}
	t.CallStack = saved.CallStack
	t.TaskLocal = saved.TaskLocal
	t.WakeValue = saved.WakeValue
	t.WakeErrorAsValue = saved.WakeErrorAsValue
	t.WakeTime = saved.WakeTime
	t.ReadingPlayer = saved.ReadingPlayer
	t.IsExecSuspended = saved.IsExecSuspended
	t.IsHTTPReadSuspended = saved.IsHTTPReadSuspended
	t.ExecCommandName = saved.ExecCommandName
	t.CreatedForks = nil
	// The runtime replays the instruction budget, but retains this slice's
	// seconds deadline. Only a real wake starts a new background time budget.
	t.Context = saved.Context
}

// ContextValue returns the current execution context pointer safely.
func (t *State) ContextValue() *Context {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.Context
}

// SetTopFrameProgrammer updates the programmer of the current activation frame.
// The update must happen while holding the lock: returning a pointer into
// CallStack would allow a concurrent append to move the slice before the caller
// writes through the pointer.
func (t *State) SetTopFrameProgrammer(who types.ObjID) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if len(t.CallStack) == 0 {
		return false
	}
	t.CallStack[len(t.CallStack)-1].Programmer = who
	return true
}

// UpdateLineNumber updates the line number of the top activation frame
func (t *State) UpdateLineNumber(line int) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if len(t.CallStack) > 0 {
		t.CallStack[len(t.CallStack)-1].LineNumber = line
	}
}

// UpdateCallStackLineNumbers bulk-updates line numbers for all frames in the
// call stack.  lineNumbers[0] corresponds to CallStack[0], etc.  Extra
// entries in either slice are ignored.
func (t *State) UpdateCallStackLineNumbers(lineNumbers []int) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	for i := 0; i < len(lineNumbers) && i < len(t.CallStack); i++ {
		t.CallStack[i].LineNumber = lineNumbers[i]
	}
}

// UpdateCallStackRuntimeVariableSnapshots installs immutable local snapshots.
// Each snapshot corresponds to the activation at the same index.
func (t *State) UpdateCallStackRuntimeVariableSnapshots(snapshots []*types.RuntimeVariableSnapshot) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	for i := 0; i < len(snapshots) && i < len(t.CallStack); i++ {
		t.CallStack[i].RuntimeVariables = types.None
		t.CallStack[i].RuntimeVariableSnapshot = snapshots[i]
	}
}

// TicksLeft returns remaining ticks
func (t *State) TicksLeft() int64 {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.TicksLimit - t.TicksUsed
}

// ResetExecutionBudget starts a fresh execution slice and returns the values
// needed to construct its deadline. Keeping this update atomic prevents task
// introspection from observing a mixture of old and new limits.
func (t *State) ResetExecutionBudget(ticks int64, seconds float64, start time.Time) (time.Time, float64) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.TicksLimit = ticks
	t.TicksUsed = 0
	t.SecondsLimit = seconds
	t.SecondsUsed = 0
	t.StartTime = start
	return t.StartTime, t.SecondsLimit
}

// ExecutionBudget returns a consistent deadline budget snapshot.
func (t *State) ExecutionBudget() (time.Time, float64) {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.StartTime, t.SecondsLimit
}

// SetCancelFunc publishes the cancellation function safely.
func (t *State) SetCancelFunc(cancel context.CancelFunc) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.CancelFunc = cancel
	if t.Status == TaskKilled && cancel != nil {
		cancel()
	}
}

// SetQueueSeq records the runtime's enqueue sequence under the task lock.
func (t *State) SetQueueSeq(sequence int64) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.QueueSeq = sequence
}

// PrepareYieldRequeue stamps a zero-delay suspension and its enqueue order as
// one atomic scheduling update.
func (t *State) PrepareYieldRequeue(sequence int64, now time.Time) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.waitingForInput = t.pendingInput != 0
	if t.WakeTime.IsZero() {
		t.WakeTime = now
	}
	t.QueueSeq = sequence
}

// ForkFamily identifies a task together with every task forked from it,
// directly or through its forks. suspend(0) lets work a task started finish
// its slice first, and that is the work in its family.
func (t *State) ForkFamily() int64 {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	if t.forkFamily != 0 {
		return t.forkFamily
	}
	return t.ID
}

// JoinForkFamily records that t was forked by parent.
func (t *State) JoinForkFamily(parent *State) {
	family := parent.ForkFamily()
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.forkFamily = family
}

// SetReadier records the task whose fork made this task ready.
func (t *State) SetReadier(readier *State) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.readier = readier
}

// PendingReadier identifies a task whose next slice must follow this readied
// slice. Retain the relation through admission and physical execution, including
// retries and logical suspension. A future delayed fork does not hold its parent.
func (t *State) PendingReadier(now time.Time) *State {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	if t.readier == nil || t.readier == t || t.readier.ID == 0 {
		return nil
	}
	if t.executionActive || t.Status == TaskRunning || (t.admissionPending && t.Status == TaskQueued) {
		return t.readier
	}
	if at := t.readyDeadlineLocked(now); !at.IsZero() && !at.After(now) {
		return t.readier
	}
	return nil
}

// awaitingReadier reports whether the slice that forked or resumed this task
// is still executing. Toast runs a readied task only after that slice ends, so
// it observes everything the slice did; until then it stays queued. The
// readier's lease release notifies the scheduler. Lightweight server-hook
// tasks all carry ID 0, and the runtime keys execution leases by task ID, so
// their lease flag is not a reliable end-of-slice signal and is not waited on.
func (t *State) awaitingReadier() bool {
	t.Mu.RLock()
	readier := t.readier
	t.Mu.RUnlock()
	if readier == nil || readier == t || readier.ID == 0 {
		return false
	}
	readier.Mu.RLock()
	defer readier.Mu.RUnlock()
	return readier.executionActive
}

// NewInputReceipt records causally prior input without waiting inside its
// builtin. The receiver resolves it after its first slice (or on discard).
// read() delivery deliberately does not consult this zero-delay-yield fence.
func (t *State) NewInputReceipt() func() {
	t.Mu.Lock()
	t.pendingInput++
	t.Mu.Unlock()
	return sync.OnceFunc(func() {
		t.Mu.Lock()
		defer t.Mu.Unlock()
		t.pendingInput--
		if t.pendingInput == 0 && t.waitingForInput {
			t.waitingForInput = false
			t.notifyScheduleLocked()
		}
	})
}

// SchedulingSnapshot returns immutable heap ordering keys.
func (t *State) SchedulingSnapshot() (time.Time, time.Time, int64) {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.StartTime, t.WakeTime, t.QueueSeq
}

// SecondsLeft returns remaining seconds
func (t *State) SecondsLeft() float64 {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	if !t.executionDeadline.IsZero() {
		return math.Max(0, math.Ceil(time.Until(t.executionDeadline).Seconds()))
	}
	return t.SecondsLimit - t.SecondsUsed
}

// SetExecutionDeadline starts the timer only once execution has acquired its
// resources. Queueing and waiting for the commit gate do not consume it.
func (t *State) SetExecutionDeadline(deadline time.Time) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.executionDeadline = deadline
}

// ExcludeExecutionWait keeps server-side contention out of the MOO budget.
func (t *State) ExcludeExecutionWait(wait time.Duration) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if !t.executionDeadline.IsZero() {
		t.executionDeadline = t.executionDeadline.Add(wait)
	}
}

// ConsumeTick increments tick count and returns true if ticks remain
func (t *State) ConsumeTick() bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.TicksUsed++
	return t.TicksUsed < t.TicksLimit
}

// GetTaskLocal returns the task-local value
func (t *State) GetTaskLocal() types.Value {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.TaskLocal
}

// SetTaskLocal sets the task-local value
func (t *State) SetTaskLocal(val types.Value) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.TaskLocal = val
}

// BytecodeVMValue returns the saved bytecode VM handle (thread-safe).
//
// BytecodeVM is read by the engine from goroutines other than the one
// running the task (e.g. liveTaskVMs scanning sibling tasks for orphan-anonymous
// GC, and ProcessReadyTasks' readiness check), so every access must hold t.mu.
// This accessor is a leaf with respect to the runtime's mutex: it never
// acquires any other lock, so the runtime may safely hold that mutex while calling
// it (lock order: s.mu -> task.mu, never the reverse).
func (t *State) BytecodeVMValue() interface{} {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.BytecodeVM
}

// SetBytecodeVM stores (or clears, with nil) the bytecode VM handle (thread-safe).
// See BytecodeVMValue for the locking rationale and ordering.
func (t *State) SetBytecodeVM(machine interface{}) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.BytecodeVM = machine
}

// IndefiniteSuspendStartTime is the sentinel StartTime stamped on an
// indefinitely-suspended task (suspend() with no/negative seconds). It mirrors
// ToastStunt's enqueue_suspended_task, which sets start_tv.tv_sec = INTNUM_MAX
// for an indefinite suspend (tasks.cc:1306-1307) so the task sorts AFTER every
// timed task in queued_tasks(). It is a far-future instant so the ascending
// queued_tasks comparator orders such tasks last. WakeTime is deliberately left
// zero so the runtime never auto-wakes it — only an explicit resume() does.
var IndefiniteSuspendStartTime = time.Unix(1<<62, 0)

// Suspend suspends the task for a duration. A zero duration is a scheduler
// yield with no wake deadline, so any WakeTime left over from an earlier timed
// suspend is cleared rather than letting the scheduler treat the yield as due.
func (t *State) Suspend(duration time.Duration) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.setStateLocked(TaskSuspended)
	t.suspendGen++
	t.WakeTime = time.Time{}
	if duration > 0 {
		t.WakeTime = time.Now().Add(duration)
	}
	t.notifyScheduleLocked()
}

// SuspendIndefinite suspends the task with no wake deadline (suspend() with
// no/negative seconds). The task waits for an explicit resume() and must never
// auto-wake, so WakeTime is cleared: a deadline left over from an earlier timed
// suspend would otherwise make the scheduler wake this suspension with 0 before
// the party it is waiting on delivers its value. StartTime is stamped with the
// far-future IndefiniteSuspendStartTime sentinel so the task sorts LAST in
// queued_tasks() (ascending by start time), matching ToastStunt's INTNUM_MAX
// start_tv.
func (t *State) SuspendIndefinite() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.setStateLocked(TaskSuspended)
	t.suspendGen++
	t.WakeTime = time.Time{}
	t.StartTime = IndefiniteSuspendStartTime
	t.notifyScheduleLocked()
}

// SuspendGeneration identifies the task's current suspension. Every Suspend and
// SuspendIndefinite starts a new generation, so a completion that captured the
// generation at suspend time can tell (via ResumeGeneration) whether the task is
// still waiting on it or has since been retried, resumed by someone else, or
// suspended again.
func (t *State) SuspendGeneration() uint64 {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.suspendGen
}

// ResumeGeneration resumes the task like Resume, but only while it is still in
// the suspension identified by gen. A stale completion (one belonging to an
// abandoned attempt or an earlier suspension) is ignored and returns false.
func (t *State) ResumeGeneration(gen uint64, value types.Value) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.suspendGen != gen {
		return false
	}
	if !t.resumeLocked(value) {
		return false
	}
	// Threaded builtins, like exec(), publish an external readiness event.
	// Keep this under the task lock so the selector cannot consume it early.
	t.execReadyAt = time.Now()
	return true
}

// Resume resumes the task with a value
// Returns false if task is not suspended or is exec-suspended
func (t *State) Resume(value types.Value) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	return t.resumeLocked(value)
}

func (t *State) resumeLocked(value types.Value) bool {
	if t.Status != TaskSuspended {
		return false
	}
	// Can't resume exec-suspended tasks - they must complete on their own or be killed
	if t.IsExecSuspended {
		return false
	}
	t.setStateLocked(TaskQueued)
	t.wokenUnrun = true
	t.notifyScheduleLocked()
	t.WakeValue = value
	t.IsHTTPReadSuspended = false
	// An indefinitely-suspended task carries the far-future
	// IndefiniteSuspendStartTime sentinel so it sorts last in queued_tasks().
	// Once explicitly resumed it must become runnable now, but the runtime's
	// readiness gate keys off StartTime (engine/runtime.go: !StartTime.After(now)),
	// so clear the sentinel back to now. runTask() re-stamps StartTime when the
	// resumed VM actually runs, so this only affects the brief queued window.
	if t.StartTime.Equal(IndefiniteSuspendStartTime) {
		t.StartTime = time.Now()
	}
	return true
}

// ReplaceWakeValue is resume() of a task already woken from a suspension whose
// slice has not started: Toast's do_resume gives it the new value and leaves its
// place in the queue. It returns false for any other task.
func (t *State) ReplaceWakeValue(value types.Value) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status != TaskQueued || !t.wokenUnrun || t.executionActive {
		return false
	}
	t.WakeValue = value
	return true
}

// ResumeFrom is resume() issued by task readier. Toast appends the target to
// the ready queue at once, so the resume is stamped as a readiness event at now
// with enqueue order sequence; an early resume of a timed suspension drops its
// future wake deadline.
func (t *State) ResumeFrom(value types.Value, readier *State, sequence int64, now time.Time) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if !t.resumeLocked(value) {
		return false
	}
	t.readier = readier
	t.WakeTime = now
	t.QueueSeq = sequence
	return true
}

// ResumeAndClaim resumes a suspended task directly into the running state.
// It is used by synchronous resume paths, where publishing TaskQueued even
// briefly would allow the ordinary scheduler to dispatch the same task.
func (t *State) ResumeAndClaim(value types.Value) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status != TaskSuspended || t.IsExecSuspended {
		return false
	}
	t.setStateLocked(TaskRunning)
	t.WakeValue = value
	t.IsHTTPReadSuspended = false
	if t.StartTime.Equal(IndefiniteSuspendStartTime) {
		t.StartTime = time.Now()
	}
	return true
}

// CompleteExec resumes an exec-suspended task with the subprocess result.
// Unlike Resume(), this works even when IsExecSuspended is true.
// Called from the exec goroutine when the subprocess finishes.
func (t *State) CompleteExec(value types.Value) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status != TaskSuspended {
		return false
	}
	t.IsExecSuspended = false
	t.ExecCancelFunc = nil
	t.ExecCommandName = ""
	t.setStateLocked(TaskQueued)
	t.notifyScheduleLocked()
	t.execReadyAt = time.Now()
	t.WakeValue = value
	if t.StartTime.Equal(IndefiniteSuspendStartTime) {
		t.StartTime = time.Now()
	}
	return true
}

// ExecReadyTime returns when an external result made the task ready.
func (t *State) ExecReadyTime() time.Time {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.execReadyAt
}

// TakeExecReadyTime consumes the observation timestamp once, under the task lock.
func (t *State) TakeExecReadyTime() time.Time {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	ready := t.execReadyAt
	t.execReadyAt = time.Time{}
	return ready
}

// WakeDue reports whether a suspended task has a timed wake deadline due.
func (t *State) WakeDue(now time.Time) bool {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.Status == TaskSuspended && !t.WakeTime.IsZero() && !t.WakeTime.After(now)
}

// Kill kills the task
func (t *State) Kill() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	t.cancelRequested = true
	t.setStateLocked(TaskKilled)
	t.notifyScheduleLocked()
	if t.CancelFunc != nil {
		t.CancelFunc()
	}
	// If the task is exec-suspended, cancel the subprocess
	if t.ExecCancelFunc != nil {
		t.ExecCancelFunc()
		t.ExecCancelFunc = nil
	}
	t.IsExecSuspended = false
	t.ExecCommandName = ""
	t.IsHTTPReadSuspended = false
}

func (t *State) CancellationRequested() bool {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.cancelRequested
}

// ToQueuedTaskInfo returns task info for queued_tasks().
// The optional final map is present when queued_tasks(1) requests variables.
// Format: {task_id, start_time, clock_id, bg_ticks, programmer, verb_loc, verb_name, line, this, bytes[, variables]}
// Note: For primitive prototype calls, 'this' is #-1 (matching Toast).
func (t *State) ToQueuedTaskInfo(includeVariables bool) types.Value {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.toQueuedTaskInfoLocked(includeVariables)
}

// QueuedTaskSnapshot atomically captures both a task's sort keys and its
// queued_tasks() representation. Callers can sort these immutable snapshots
// without comparing fields that another worker may be changing.
func (t *State) QueuedTaskSnapshot(includeVariables bool) (time.Time, int64, int64, types.Value) {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return t.StartTime, t.QueueSeq, t.ID, t.toQueuedTaskInfoLocked(includeVariables)
}

func (t *State) toQueuedTaskInfoLocked(includeVariables bool) types.Value {

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
		programmer = t.Programmer
		thisObj = t.This
	}
	if thisVal.IsNone() {
		thisVal = types.NewObj(thisObj)
	}

	// Estimate bytes (0 for now, can be calculated later if needed)
	bytes := int64(0)

	reportedStart := t.StartTime
	if t.Status == TaskSuspended && !t.WakeTime.IsZero() {
		reportedStart = t.WakeTime
	}
	startValue := types.NewInt(reportedStart.Add(500 * time.Millisecond).Unix())
	if t.IsExecSuspended && t.ExecCommandName != "" {
		startValue = types.NewStr(t.ExecCommandName)
	}

	values := []types.Value{
		types.NewInt(t.ID),              // [1] task_id
		startValue,                      // [2] start_time or exec executable label
		types.NewInt(0),                 // [3] obsolete clock ID
		types.NewInt(30000),             // [4] DEFAULT_BG_TICKS (obsolete)
		types.NewObj(programmer),        // [5] programmer
		types.NewObj(verbLoc),           // [6] verb_loc
		types.NewStr(verbName),          // [7] verb_name
		types.NewInt(int64(lineNumber)), // [8] line_number
		thisVal,                         // [9] this
		types.NewInt(bytes),             // [10] bytes
	}
	if includeVariables {
		var variablePairs [][2]types.Value
		if t.ForkInfo != nil {
			variablePairs = make([][2]types.Value, 0, len(t.ForkInfo.Variables))
			for name, value := range t.ForkInfo.Variables {
				variablePairs = append(variablePairs, [2]types.Value{
					types.NewStr(name),
					value,
				})
			}
		}
		values = append(values, types.NewMap(variablePairs))
	}
	return types.NewList(values)
}
