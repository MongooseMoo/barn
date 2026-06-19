package server

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/trace"
	"barn/types"
	"barn/vm"
)

// InputEvent represents a line of input (or disconnect) from a connection.
// Connection goroutines enqueue these; the scheduler processes them.
type InputEvent struct {
	ConnID       int64
	Player       types.ObjID // negative = pre-login, positive = logged-in
	Line         string
	IsDisconnect bool
	Done         chan struct{} // Closed when processing is complete
}

// Scheduler manages task execution
type Scheduler struct {
	tasks                   map[int64]*task.Task
	waiting                 *TaskQueue
	nextTaskID              int64
	queueSeq                int64
	registry                *builtins.Registry // Shared builtins registry for bytecode VMs
	store                   *dbstore.Store
	connManager             *ConnectionManager
	inputQueue              chan InputEvent
	pendingFinalizationSink func([]types.Value)
	mu                      sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc
	wg                      sync.WaitGroup
}

// NewScheduler creates a new task scheduler
func NewScheduler(store *dbstore.Store) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		tasks:      make(map[int64]*task.Task),
		waiting:    NewTaskQueue(),
		nextTaskID: 1,
		registry:   vm.BuildVMRegistry(),
		store:      store,
		inputQueue: make(chan InputEvent, 256),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Builtins like create()/recycle() need verb callbacks in VM mode.
	// Route builtin CallVerb() through scheduler CallVerb().
	s.registry.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, tc *kernel.TaskContext) types.Result {
		player := types.ObjNothing
		if tc != nil {
			player = tc.Player
			if player == types.ObjNothing {
				player = tc.Programmer
			}
		}
		return s.CallVerb(objID, verbName, args, player)
	})
	builtins.SetRunGCFunc(func(ctx *kernel.TaskContext) error {
		vm.AutoRecycleOrphanAnonymousWith(store, s.registry, ctx)
		return nil
	})

	return s
}

func (s *Scheduler) populateTaskContextDependencies(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.Store = s.store
	ctx.Registry = s.registry
}

// Start begins the scheduler loop
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// SetConnectionManager sets the connection manager for output flushing
func (s *Scheduler) SetConnectionManager(cm *ConnectionManager) {
	s.connManager = cm
}

func (s *Scheduler) SetPendingFinalizationSink(sink func([]types.Value)) {
	s.pendingFinalizationSink = sink
}

// EnqueueInput sends an input event to the scheduler for processing.
// The caller should wait on evt.Done to know when processing is complete.
func (s *Scheduler) EnqueueInput(evt InputEvent) {
	s.inputQueue <- evt
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case input := <-s.inputQueue:
			s.processInput(input)
		case <-ticker.C:
			s.processReadyTasks()
		}
	}
}

// processInput handles an input event from a connection.
// All MOO verb execution (login, command dispatch, disconnect hooks) happens here,
// on the scheduler goroutine, matching Toast's single-threaded execution model.
func (s *Scheduler) processInput(input InputEvent) {
	defer func() {
		if input.Done != nil {
			close(input.Done)
		}
	}()

	if input.IsDisconnect {
		s.processDisconnect(input)
		return
	}

	oob := strings.HasPrefix(input.Line, "#$#")
	disableOOB := builtins.ConnectionOptionTruthy(input.Player, "disable-oob")
	if !(oob && !disableOOB) && builtins.HandleHeldInput(input.Player, input.Line, false) {
		return
	}

	// Check if a task is read()ing from this player — if so, route input there
	if s.deliverToReadingTask(input.Player, input.Line) {
		return
	}

	if input.Player < 0 {
		s.processPreLogin(input)
		return
	}

	s.processCommand(input)
}

// deliverToReadingTask checks whether any suspended task is read()ing from the
// given player. If found, clears the reading flag and resumes the task with the
// input line. Returns true if delivered.
func (s *Scheduler) deliverToReadingTask(player types.ObjID, line string) bool {
	mgr := task.GetManager()
	t := mgr.FindReadingTask(player)
	if t == nil {
		return false
	}
	t.ReadingPlayer = types.ObjNothing
	t.Resume(types.NewStr(line))
	return true
}

// ForceInput implements builtins.InputForcer.
// It injects a line of input for the given player. If a task is currently
// read()ing from that player, the line resumes it directly. Otherwise the
// line is enqueued as a normal InputEvent.
func (s *Scheduler) ForceInput(player types.ObjID, line string, atFront bool) {
	oob := strings.HasPrefix(line, "#$#")
	disableOOB := builtins.ConnectionOptionTruthy(player, "disable-oob")
	if !(oob && !disableOOB) && builtins.HandleHeldInput(player, line, atFront) {
		return
	}

	// Try to deliver to a reading task first
	if s.deliverToReadingTask(player, line) {
		return
	}

	// No reading task — enqueue as normal input
	connID := int64(0)
	if s.connManager != nil {
		if conn := s.connManager.GetConnection(player); conn != nil {
			if c, ok := conn.(*Connection); ok {
				connID = c.ID
			}
		}
	}
	// A pre-login (negative) target with no live connection: ToastStunt's
	// force_input does find_tqueue(conn, 1) — creating a queue on demand — and
	// runs do_login_command for it. Mirror that by driving do_login_command
	// directly for the phantom connection so the line reaches the login path.
	if player < 0 && connID == 0 {
		s.forcePhantomLogin(player, line)
		return
	}

	evt := InputEvent{
		ConnID: connID,
		Player: player,
		Line:   line,
	}
	if player < 0 && connID != 0 {
		s.processInput(evt)
		return
	}
	s.inputQueue <- evt
}

// forcePhantomLogin runs #0:do_login_command for a pre-login target that has no
// live connection, matching ToastStunt's create-on-demand task queue in
// bf_force_input. There is no connection to bind, so a returned player is not
// logged in — only the verb's side effects occur.
func (s *Scheduler) forcePhantomLogin(player types.ObjID, line string) {
	words := commandWordList(line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}
	s.CallVerbWithArgstr(types.ObjID(0), "do_login_command", args, player, line)
}

// processDisconnect handles a disconnect event.
func (s *Scheduler) processDisconnect(input InputEvent) {
	cm := s.connManager
	if cm == nil {
		return
	}

	cm.mu.Lock()
	conn := cm.connections[input.ConnID]
	if conn == nil {
		cm.mu.Unlock()
		return
	}

	wasLoggedIn := conn.IsLoggedIn()
	player := conn.GetPlayer()
	handler := conn.ListenerObject()

	delete(cm.connections, conn.ID)
	if wasLoggedIn {
		if mapped := cm.playerConns[player]; mapped == conn {
			delete(cm.playerConns, player)
			cm.restorePreviousPlayerConnLocked(player, conn)
		} else {
			cm.removePlayerHistoryConnLocked(player, conn)
		}
	} else {
		// Remove pre-login negative ID mapping
		if mapped := cm.playerConns[types.ObjID(-conn.ID)]; mapped == conn {
			delete(cm.playerConns, types.ObjID(-conn.ID))
		}
	}
	cm.mu.Unlock()
	cm.detachOutboundClient(conn.ID)
	builtins.CloseHeldHTTPInput(player)

	// Trace disconnect event
	if wasLoggedIn {
		trace.Connection("DISCONNECT", conn.ID, player, "")
	} else {
		trace.Connection("DISCONNECT", conn.ID, types.ObjID(-conn.ID), "unlogged")
	}

	// Call user_client_disconnected hook on the scheduler goroutine
	if wasLoggedIn {
		s.callUserClientDisconnected(handler, player)
	}

	log.Printf("Connection %d closed", conn.ID)
}

// processPreLogin handles input from an unauthenticated connection.
func (s *Scheduler) processPreLogin(input InputEvent) {
	cm := s.connManager
	if cm == nil {
		return
	}

	conn := cm.getConnectionByConnID(input.ConnID)
	if conn == nil {
		return
	}

	line := input.Line
	proxyLine := s.isTrustedProxyConnection(conn) && strings.HasPrefix(line, "PROXY ")
	if proxyLine {
		line = ""
	}

	if !proxyLine && !s.shouldCallDoLoginCommand(conn, line) {
		return
	}

	maxBeforeLogin := s.store.MaxObject()
	player, _ := s.callDoLoginCommand(conn, line)
	if player > 0 {
		s.loginPlayer(conn, player, player > maxBeforeLogin)
	}
}

// processCommand handles input from an authenticated (logged-in) connection.
func (s *Scheduler) processCommand(input InputEvent) {
	cm := s.connManager
	if cm == nil {
		return
	}

	conn := cm.getConnectionByConnID(input.ConnID)
	if conn == nil {
		return
	}

	player := conn.GetPlayer()
	location, errCode := s.store.Location(player)
	if errCode != types.E_NONE {
		return
	}

	if s.processProgrammingInput(conn, input.Line) {
		return
	}

	if strings.HasPrefix(input.Line, "#$#") && !builtins.ConnectionOptionTruthy(player, "disable-oob") {
		words := commandWordList(input.Line)
		args := make([]types.Value, len(words))
		for i, word := range words {
			args[i] = types.NewStr(word)
		}
		result := s.CallVerbWithArgstr(conn.ListenerObject(), "do_out_of_band_command", args, player, input.Line)
		if result.Flow == types.FlowException && result.Error != types.E_VERBNF {
			var stack []task.ActivationFrame
			if result.CallStack != nil {
				if st, ok := result.CallStack.([]task.ActivationFrame); ok {
					stack = st
				}
			}
			s.sendTracebackToPlayer(player, result.Error, stack)
		}
		return
	}

	// Parse the command
	cmd := ParseCommand(input.Line)
	if cmd.Verb == "" {
		return
	}

	// Handle intrinsic commands (PREFIX, SUFFIX, OUTPUTPREFIX, OUTPUTSUFFIX)
	verbUpper := strings.ToUpper(cmd.Verb)
	switch verbUpper {
	case "PREFIX", "OUTPUTPREFIX":
		conn.mu.Lock()
		conn.outputPrefix = cmd.Argstr
		conn.mu.Unlock()
		return
	case "SUFFIX", "OUTPUTSUFFIX":
		conn.mu.Lock()
		conn.outputSuffix = cmd.Argstr
		conn.mu.Unlock()
		return
	case ".PROGRAM":
		s.startProgrammingMode(conn, player, location, cmd.Argstr)
		return
	}

	// Raw command response framing for conformance transport.
	outputPrefix := conn.GetOutputPrefix()
	outputSuffix := conn.GetOutputSuffix()
	if outputPrefix != "" {
		_ = conn.Send(outputPrefix)
	}

	// Invoke #0:do_command for normal commands
	commandWords := cmd.Words
	if len(commandWords) == 0 {
		commandWords = append([]string{cmd.Verb}, cmd.Args...)
	}
	handled, _ := s.callDoCommand(conn.ListenerObject(), player, commandWords, input.Line)
	if handled {
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	// Resolve direct object
	if cmd.Dobjstr != "" {
		cmd.Dobj = MatchObject(s.store, player, location, cmd.Dobjstr)
	}

	// Resolve indirect object
	if cmd.Iobjstr != "" {
		cmd.Iobj = MatchObject(s.store, player, location, cmd.Iobjstr)
	}

	// Find the verb
	match := FindVerb(s.store, player, location, cmd)
	if match == nil {
		if verbUpper == "EVAL" {
			code := strings.TrimSpace(cmd.Argstr)
			if code != "" {
				s.EvalCommand(player, code, conn)
			}
			if outputSuffix != "" {
				_ = conn.Send(outputSuffix)
			}
			return
		}

		usePlayerHuh := false
		if option, ok := s.getServerOption(0, "player_huh"); ok {
			usePlayerHuh = option.Truthy()
		}

		huhTarget := location
		if usePlayerHuh {
			huhTarget = player
		}

		if huhVerb, huhVerbLoc, err := s.store.FindVerb(huhTarget, "huh"); err == nil && huhVerb != nil {
			huhMatch := &VerbMatch{
				Verb:    huhVerb,
				This:    huhTarget,
				VerbLoc: huhVerbLoc,
			}

			if huhMatch.Verb.Program == nil && len(huhMatch.Verb.Code) > 0 {
				program, errors := dbstore.CompileVerb(huhMatch.Verb.Code)
				if len(errors) > 0 {
					conn.Send(fmt.Sprintf("Verb compile error: %s", errors[0]))
					if outputSuffix != "" {
						_ = conn.Send(outputSuffix)
					}
					return
				}
				huhMatch.Verb.Program = program
			}

			if huhMatch.Verb.Program == nil || len(huhMatch.Verb.Program.Statements) == 0 {
				conn.Send("I couldn't understand that.")
				if outputSuffix != "" {
					_ = conn.Send(outputSuffix)
				}
				return
			}

			// Execute huh() synchronously on the scheduler goroutine
			s.executeVerbTaskSync(player, huhMatch, cmd, outputSuffix)
			return
		}
		conn.Send("I couldn't understand that.")
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	// Compile verb if needed (lazy compilation)
	if match.Verb.Program == nil && len(match.Verb.Code) > 0 {
		program, errors := dbstore.CompileVerb(match.Verb.Code)
		if len(errors) > 0 {
			conn.Send(fmt.Sprintf("Verb compile error: %s", errors[0]))
			if outputSuffix != "" {
				_ = conn.Send(outputSuffix)
			}
			return
		}
		match.Verb.Program = program
	}

	// Execute the verb
	if match.Verb.Program == nil || len(match.Verb.Program.Statements) == 0 {
		conn.Send(fmt.Sprintf("[%s has no code]", match.Verb.Name))
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	// Execute verb synchronously on the scheduler goroutine
	s.executeVerbTaskSync(player, match, cmd, outputSuffix)
}

func (s *Scheduler) processProgrammingInput(conn *Connection, line string) bool {
	conn.mu.Lock()
	mode := conn.programming
	if mode == nil {
		conn.mu.Unlock()
		return false
	}
	if strings.TrimSpace(line) != "." {
		mode.Lines = append(mode.Lines, line)
		conn.mu.Unlock()
		return true
	}
	conn.programming = nil
	lines := append([]string(nil), mode.Lines...)
	target := mode.Target
	verbName := mode.Verb
	conn.mu.Unlock()

	if _, err := s.store.FindLocalVerbForProgramming(target, verbName); err != nil {
		conn.Send("Verb not found")
		return true
	}
	program, errors := dbstore.CompileVerb(lines)
	if len(errors) > 0 {
		for _, errText := range errors {
			conn.Send(errText)
		}
		return true
	}
	if errCode := s.store.SetVerbCode(target, verbName, lines, program); errCode != types.E_NONE {
		conn.Send("Verb not found")
		return true
	}
	return true
}

func (s *Scheduler) startProgrammingMode(conn *Connection, player, location types.ObjID, spec string) {
	target, verbName, ok := s.parseProgramTarget(player, location, spec)
	if !ok {
		conn.Send("Verb not found")
		return
	}
	conn.mu.Lock()
	conn.programming = &programmingMode{
		Target: target,
		Verb:   verbName,
		Lines:  make([]string, 0),
	}
	conn.mu.Unlock()
}

func (s *Scheduler) parseProgramTarget(player, location types.ObjID, spec string) (types.ObjID, string, bool) {
	spec = strings.TrimSpace(spec)
	colon := strings.LastIndex(spec, ":")
	if colon < 0 {
		return types.ObjNothing, "", false
	}
	objText := strings.TrimSpace(spec[:colon])
	verbName := strings.TrimSpace(spec[colon+1:])
	if objText == "" || verbName == "" {
		return types.ObjNothing, "", false
	}

	target := MatchObject(s.store, player, location, objText)
	if target < 0 {
		return types.ObjNothing, "", false
	}
	if _, err := s.store.FindLocalVerbForProgramming(target, verbName); err != nil {
		return types.ObjNothing, "", false
	}
	return target, verbName, true
}

// processReadyTasks executes tasks that are ready to run.
func (s *Scheduler) processReadyTasks() int {
	s.mu.Lock()

	now := time.Now()
	var readyTasks []*task.Task

	// Collect all ready tasks from waiting queue
	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		if t.StartTime.After(now) {
			break // Tasks are ordered by start time
		}
		heap.Pop(s.waiting)
		if t.GetState() != task.TaskQueued {
			// Ignore tasks killed/suspended after enqueue.
			continue
		}
		readyTasks = append(readyTasks, t)
	}

	// Build set of tasks already collected from the waiting heap
	// to avoid double-scheduling them in the resumed scan below.
	heapReady := make(map[int64]bool, len(readyTasks))
	for _, t := range readyTasks {
		heapReady[t.ID] = true
	}

	// Check for suspended/resumed tasks that need to be re-run.
	// These are tasks that were suspended and later resumed via resume() builtin
	for _, t := range s.tasks {
		if heapReady[t.ID] {
			continue // Already collected from waiting heap
		}

		// Timed suspension wake-up: suspend(seconds) resumes after deadline.
		if t.WakeDue(now) {
			if t.Resume(types.NewInt(0)) {
				readyTasks = append(readyTasks, t)
			}
			continue
		}

		// TaskQueued state means it was resumed and is ready to run again
		// We need to check if it's not already in readyTasks and not in waiting queue
		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVM != nil) {
			// This is a resumed task (StmtIndex > 0 or BytecodeVM saved means it was partially executed)
			// Check if wake time has passed (or no wake time was set)
			// Also check StartTime to avoid running delayed forks before their delay.
			if (t.WakeTime.IsZero() || !t.WakeTime.After(now)) && !t.StartTime.After(now) {
				readyTasks = append(readyTasks, t)
			}
		}
	}

	s.mu.Unlock()

	// Execute ready tasks sequentially on the scheduler goroutine.
	// Toast is single-threaded: one task at a time. No concurrent MOO execution.
	for _, t := range readyTasks {
		err := s.runTask(t)
		if err != nil {
			log.Printf("Task %d (#%d:%s) error: %v", t.ID, t.This, t.VerbName, err)
		}

		// Flush output buffer for the player
		if s.connManager != nil {
			if conn := s.connManager.GetConnection(t.Owner); conn != nil {
				conn.Flush()
				// For raw command execution, emit framing suffix after task output.
				if t.CommandOutputSuffix != "" {
					_ = conn.Send(t.CommandOutputSuffix)
				}
			}
		}

		// Signal task completion so callers waiting on Done can proceed
		if t.Done != nil {
			close(t.Done)
		}
	}
	return len(readyTasks)
}

func (s *Scheduler) YieldReadyTasks() int {
	return s.processReadyTasks()
}

func (s *Scheduler) liveTaskVMs(exclude *task.Task) []*vm.VM {
	s.mu.Lock()
	defer s.mu.Unlock()

	var roots []*vm.VM
	for _, queued := range s.tasks {
		if queued == nil || (exclude != nil && queued.ID == exclude.ID) {
			continue
		}
		state := queued.GetState()
		if state == task.TaskCompleted || state == task.TaskKilled {
			continue
		}
		if exec, ok := queued.BytecodeVM.(*vm.VM); ok && exec != nil {
			roots = append(roots, exec)
		}
	}
	return roots
}

// isWizard checks if an object has wizard permissions
func (s *Scheduler) isWizard(objID types.ObjID) bool {
	hasWizard, errCode := s.store.HasObjectFlag(objID, dbstore.FlagWizard)
	return errCode == types.E_NONE && hasWizard
}

// Error definitions
var (
	ErrTicksExceeded = errors.New("tick limit exceeded")
	ErrNotSuspended  = errors.New("task not suspended")
	ErrResumeFailed  = errors.New("failed to resume task")
	ErrPermission    = errors.New("permission denied")
)
