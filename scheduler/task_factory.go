package scheduler

import (
	"container/heap"
	"fmt"
	"log/slog"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func foregroundTaskLimits() (int64, float64) {
	return builtins.GetTaskLimits(false)
}

func backgroundTaskLimits() (int64, float64) {
	return builtins.GetTaskLimits(true)
}

func configureVMStackLimit(machine *vm.VM) {
	machine.MaxStackDepth = builtins.GetMaxStackDepth(machine.Store)
}

// QueueTask adds a task to the scheduler
func (s *Scheduler) QueueTask(t *task.Task) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.SetState(task.TaskQueued)
	s.tasks[t.ID] = t
	s.queueSeq++
	t.QueueSeq = s.queueSeq
	heap.Push(s.waiting, t)

	// Register with this scheduler's task manager so builtins can find it.
	s.taskManager.RegisterTask(t)

	return t.ID
}

// CreateForegroundTask creates a foreground task (user command)
func (s *Scheduler) CreateForegroundTask(player types.ObjID, program *bytecode.Program) int64 {
	taskID := s.newTaskID()
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, program, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now()
	t.ForkCreator = s // Give task access to scheduler for forks
	// Set wizard flag based on player
	t.Context.IsWizard = s.isWizard(player)
	return s.QueueTask(t)
}

// RunServerVerbTask runs a server-initiated hook verb through the normal
// scheduler/task machinery until it completes or reaches its first suspend.
func (s *Scheduler) RunServerVerbTask(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) (types.Result, error) {
	verb, defObjID, err := s.store.FindVerb(objID, verbName)
	if err != nil {
		return types.Result{}, fmt.Errorf("find verb %s on #%d: %w", verbName, objID, err)
	}

	program, diagnostics := compiler.CompileMOOWithKey(verb.Code, verb.CodeKey, s.registry)
	if len(diagnostics) > 0 {
		return types.Result{}, fmt.Errorf("compile %s on #%d: %s", verbName, defObjID, diagnostics[0].Error())
	}
	if len(verb.Code) == 0 {
		return types.Result{}, fmt.Errorf("verb %s on #%d has no code", verbName, defObjID)
	}

	taskID := s.newTaskID()
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, program, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now()
	t.Programmer = verb.Owner
	t.Context.Programmer = verb.Owner
	t.Context.IsWizard = s.isWizard(verb.Owner)
	t.Context.ServerInitiated = true
	t.VerbName = verbName
	t.VerbLoc = defObjID
	t.This = objID
	t.Caller = types.ObjNothing
	t.VerbArgsValues = append([]types.Value(nil), args...)
	t.ForkCreator = s

	t.SetState(task.TaskQueued)
	s.mu.Lock()
	s.tasks[t.ID] = t
	s.mu.Unlock()
	s.taskManager.RegisterTask(t)

	if err := s.runTask(t); err != nil {
		return t.Result, err
	}
	if s.taskOutputFlusher != nil {
		s.taskOutputFlusher(t.Owner, t.CommandOutputSuffix)
	}
	return t.Result, nil
}

// CreateLoginHookTask runs a login-hook verb (do_login_command and friends) as
// a real, registered, resumable scheduler task. Unlike CallVerbWithArgstr's
// throwaway task, this task participates in the normal task machinery, so a
// read() inside the login flow suspends and resumes correctly across an
// arbitrary number of input round-trips (username, password, ...). The task is
// run synchronously on the caller's goroutine to completion or to its first
// read() suspend (so login state is settled before the next input line is
// read); a suspended task stays registered and resumes when the next line is
// delivered via deliverToReadingTask. When the task finally returns, onComplete
// is invoked with the terminal result so the caller can log the player in.
// Returns the task ID, or an error if the verb cannot be found/compiled (e.g.
// no do_login_command on the listener).
// onStart, if non-nil, is invoked with the task ID after the task is
// registered but before it runs, so callers can record the in-flight task ID
// (for disconnect cancellation) before onComplete can clear it on a task that
// completes synchronously without suspending.
func (s *Scheduler) CreateLoginHookTask(objID types.ObjID, verbName string, args []types.Value, player types.ObjID, argstr string, onStart func(int64), onComplete func(types.Result)) (int64, error) {
	verb, defObjID, err := s.store.FindVerb(objID, verbName)
	if err != nil {
		return 0, fmt.Errorf("find verb %s on #%d: %w", verbName, objID, err)
	}

	program, diagnostics := compiler.CompileMOOWithKey(verb.Code, verb.CodeKey, s.registry)
	if len(diagnostics) > 0 {
		return 0, fmt.Errorf("compile %s on #%d: %s", verbName, defObjID, diagnostics[0].Error())
	}

	taskID := s.newTaskID()
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, program, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now()
	// Login task runs with verb-owner permissions, matching the throwaway
	// CallVerbWithArgstr path it replaces (programmer = verb owner).
	t.Programmer = verb.Owner
	t.Context.Programmer = verb.Owner
	t.Context.IsWizard = s.isWizard(verb.Owner)
	t.Context.ServerInitiated = true
	t.VerbName = verbName
	t.VerbLoc = defObjID
	t.This = objID
	t.Caller = player
	t.Argstr = argstr
	t.VerbArgsValues = append([]types.Value(nil), args...)
	t.ForkCreator = s
	t.OnComplete = onComplete

	// Register the task so a read() suspend leaves it discoverable by
	// FindReadingTask/deliverToReadingTask, then run it synchronously on the
	// caller's (scheduler/input) goroutine. The task runs until it returns
	// (OnComplete fires) or suspends on read() (VM saved; it stays registered
	// and resumes when the next input line is delivered). Running synchronously
	// here — rather than queuing for the ticker — ensures the login state is
	// settled before the I/O loop reads the next line, matching ToastStunt's
	// run-to-suspend-or-completion login semantics.
	t.SetState(task.TaskQueued)
	s.mu.Lock()
	s.tasks[t.ID] = t
	s.mu.Unlock()
	s.taskManager.RegisterTask(t)

	if onStart != nil {
		onStart(t.ID)
	}

	if err := s.runTask(t); err != nil {
		return t.ID, err
	}
	return t.ID, nil
}

// CreateBackgroundTask creates a background task (fork)
func (s *Scheduler) CreateBackgroundTask(player types.ObjID, program *bytecode.Program, delay time.Duration) int64 {
	taskID := s.newTaskID()
	ticks, seconds := backgroundTaskLimits()
	t := task.NewTaskFull(taskID, player, program, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now().Add(delay)
	t.ForkCreator = s // Give task access to scheduler for forks
	// Set wizard flag based on player
	t.Context.IsWizard = s.isWizard(player)
	return s.QueueTask(t)
}

// Fork creates a forked task with a delay
func (s *Scheduler) Fork(ctx *kernel.TaskContext, program *bytecode.Program, delay time.Duration) int64 {
	return s.CreateBackgroundTask(ctx.Player, program, delay)
}

// CreateForkedTask creates a forked child task from a bytecode VM fork yield.
// Implements task.ForkCreator interface.
func (s *Scheduler) CreateForkedTask(parent *task.Task, forkInfo *types.ForkInfo) int64 {
	taskID := s.newTaskID()
	programmer := parent.Programmer
	if parent.Context != nil {
		programmer = parent.Context.Programmer
	}

	var t *task.Task
	firstLine := 1

	if bcFork, ok := forkInfo.Body.([3]interface{}); ok {
		parentProg, ok1 := bcFork[0].(*bytecode.Program)
		bodyIP, ok2 := bcFork[1].(int)
		bodyLen, ok3 := bcFork[2].(int)
		if !ok1 || !ok2 || !ok3 {
			return 0 // Invalid fork info
		}

		// Extract the fork body as a sub-program
		forkProg := parentProg.ExtractForkBody(bodyIP, bodyLen)
		if line := forkProg.LineForIP(0); line > 0 {
			firstLine = line
		}

		ticks, seconds := backgroundTaskLimits()
		t = task.NewTaskFull(taskID, forkInfo.Player, nil, ticks, seconds)
		s.populateTaskContextDependencies(t.Context)

		// Create a pre-configured VM for the child
		childVM := vm.NewVM(s.store, s.registry)
		childVM.TickLimit = ticks
		configureVMStackLimit(childVM)

		// Set up the child frame with inherited variables
		frame := childVM.PrepareVerbFrame(forkProg,
			forkInfo.ThisObj, forkInfo.Player, forkInfo.Caller,
			forkInfo.Verb, forkInfo.VerbLoc, nil)
		// Mark as verb-call so syncTaskLineNumbers includes this frame
		// when syncing line numbers to the task's CallStack.
		frame.IsVerbCall = true
		// Inherit verb debug flag from the parent verb
		if forkVerb, _, vErr := s.store.FindVerb(forkInfo.ThisObj, forkInfo.Verb); vErr == nil {
			frame.VerbDebug = forkVerb.Perms.Has(dbstore.VerbDebug)
		}

		// Copy inherited variable values from the parent
		for varName, varVal := range forkInfo.Variables {
			vm.SetLocalByName(frame, forkProg, varName, varVal)
		}

		t.SetBytecodeVM(childVM)
	} else {
		return 0 // Unknown fork body type
	}

	t.StartTime = time.Now().Add(forkInfo.Delay)
	t.Kind = task.TaskForked
	t.IsForked = true
	t.ForkInfo = forkInfo
	t.Programmer = programmer
	t.This = forkInfo.ThisObj
	t.Caller = forkInfo.Caller
	t.VerbName = forkInfo.Verb
	t.VerbLoc = forkInfo.VerbLoc
	t.ForkCreator = s
	t.TaskLocal = types.NewEmptyMap()

	// Set up child's context
	t.Context.ThisObj = forkInfo.ThisObj
	t.Context.ThisValue = forkInfo.ThisValue
	t.Context.Player = forkInfo.Player
	t.Context.Programmer = programmer
	t.Context.Verb = forkInfo.Verb
	t.Context.IsWizard = s.isWizard(programmer)
	t.Context.Task = t // Attach task to context for task_local access

	// Push initial activation frame for the fork body.
	// This matches Toast: forked tasks include a frame for the verb
	// context in which the fork statement appeared.
	t.PushFrame(task.ActivationFrame{
		This:       forkInfo.ThisObj,
		ThisValue:  forkInfo.ThisValue,
		Player:     forkInfo.Player,
		Programmer: programmer,
		Caller:     forkInfo.Caller,
		Verb:       forkInfo.Verb,
		VerbLoc:    forkInfo.VerbLoc,
		LineNumber: firstLine,
	})

	childID := s.QueueTask(t)
	if parent != nil {
		parent.CreatedForks = append(parent.CreatedForks, childID)
	}
	return childID
}

// ResumeTask resumes a suspended task
func (s *Scheduler) ResumeTask(taskID int64, value types.Value) error {
	s.mu.Lock()
	t, exists := s.tasks[taskID]
	s.mu.Unlock()

	if !exists {
		return ErrNotSuspended
	}

	if !t.Resume(value) {
		return ErrNotSuspended
	}
	return nil
}

// KillTask kills a running task
func (s *Scheduler) KillTask(taskID int64, killerID types.ObjID) error {
	s.mu.Lock()
	t, exists := s.tasks[taskID]
	s.mu.Unlock()

	if !exists {
		return ErrNotSuspended
	}

	// Permission check
	if t.Owner != killerID && !s.isWizard(killerID) {
		return ErrPermission
	}

	t.Kill()
	return nil
}

// CancelLoginTasksFor forcibly kills and removes EVERY task associated with a
// disconnecting (pre-login) connection, identified by its negative connID
// player. This covers both the currently tracked login task and any task left
// suspended on read() from this connection — so no dangling read()-suspended
// task can swallow input for a connID later reused by another connection. The
// task manager is matched by ReadingPlayer, so cancelling by connID (not a
// single tracked ID) is required for robustness.
// No permission check — this is a server-internal lifecycle op.
func (s *Scheduler) CancelLoginTasksFor(connID types.ObjID) {
	// Collect candidates: tasks owned by this connID (login-hook tasks run with
	// Owner == connID) and any task currently reading from this connID.
	var victims []*task.Task
	s.mu.Lock()
	for id, t := range s.tasks {
		if t == nil {
			continue
		}
		if t.Owner == connID || t.ReadingPlayer == connID {
			victims = append(victims, t)
			delete(s.tasks, id)
		}
	}
	s.mu.Unlock()

	// Also sweep the owned manager for read()-suspended tasks bound to this
	// connID that the scheduler may not own (defense in depth).
	for _, t := range s.taskManager.GetAllTasks() {
		if t != nil && t.ReadingPlayer == connID {
			victims = append(victims, t)
		}
	}

	for _, t := range victims {
		t.ReadingPlayer = types.ObjNothing
		t.OnComplete = nil // Don't run login completion for a dead connection.
		t.Kill()
		s.taskManager.RemoveTask(t.ID)
	}
}

// ResumeReadingTask delivers an input line to a task currently suspended on
// read() from the given player and runs it synchronously to completion or to
// its next read() suspend. Returns true if a reading task was found and
// resumed. Running synchronously (rather than flipping the task to TaskQueued
// and waiting for the ticker) closes the timing window in which a follow-up
// input line, arriving before the ticker re-ran the task, would not be found by
// FindReadingTask and would spawn a parallel do_login_command.
func (s *Scheduler) ResumeReadingTask(player types.ObjID, line string) bool {
	t := s.taskManager.FindReadingTask(player)
	if t == nil {
		return false
	}
	t.ReadingPlayer = types.ObjNothing
	if !t.Resume(types.NewStr(line)) {
		return false
	}
	if err := s.runTask(t); err != nil {
		slog.Error("task resume error",
			slog.Int64("task_id", t.ID),
			slog.Int64("this", int64(t.This)),
			slog.String("verb", t.VerbName),
			slog.Any("err", err))
	}
	if s.taskOutputFlusher != nil {
		s.taskOutputFlusher(t.Owner, t.CommandOutputSuffix)
	}
	return true
}

// CleanupFinishedTasks removes completed and killed tasks from both the
// scheduler's own table and its task manager. Without this, every task
// (including unauthenticated pre-login do_login_command tasks) accumulates
// forever — an unbounded-growth / DoS vector on the pre-auth path. Only
// terminal tasks are removed; suspended/queued/running tasks are left intact.
func (s *Scheduler) CleanupFinishedTasks() {
	var finished []int64
	s.mu.Lock()
	for id, t := range s.tasks {
		if t == nil {
			delete(s.tasks, id)
			continue
		}
		st := t.GetState()
		if st == task.TaskCompleted || st == task.TaskKilled {
			finished = append(finished, id)
			delete(s.tasks, id)
		}
	}
	s.mu.Unlock()

	mgr := s.taskManager
	for _, id := range finished {
		mgr.RemoveTask(id)
	}
}

// IsTaskLive reports whether a task with the given ID exists and is neither
// completed nor killed. Used to decide whether a connection already has an
// in-flight login task (so a second do_login_command must not be spawned).
func (s *Scheduler) IsTaskLive(taskID int64) bool {
	if taskID == 0 {
		return false
	}
	s.mu.Lock()
	t := s.tasks[taskID]
	s.mu.Unlock()
	if t == nil {
		return false
	}
	st := t.GetState()
	return st != task.TaskCompleted && st != task.TaskKilled
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(taskID int64) *task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[taskID]
}

// TaskSnapshots returns immutable task snapshots for checkpoint serialization.
func (s *Scheduler) TaskSnapshots() (queued []task.Snapshot, suspended []task.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range s.tasks {
		snapshot := t.PersistenceSnapshot()
		if snapshot.State == task.TaskCompleted || snapshot.State == task.TaskKilled {
			continue
		}
		if snapshot.VM != nil {
			if snapshot.State == task.TaskSuspended && !t.WakeTime.IsZero() {
				snapshot.StartTime = t.WakeTime
			}
			suspended = append(suspended, snapshot)
			continue
		}
		switch snapshot.State {
		case task.TaskQueued:
			queued = append(queued, snapshot)
		case task.TaskSuspended:
			suspended = append(suspended, snapshot)
		}
	}
	return queued, suspended
}
