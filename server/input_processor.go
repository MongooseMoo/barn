package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
)

type InputProcessor struct {
	store       *dbstore.Store
	runtime     *engine.Runtime
	connManager *ConnectionManager
	inputQueue  chan command.InputEvent
	enqueueMu   sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	// Each connection's input is processed on its own goroutine (keyed by ConnID),
	// so a single connection's lines stay strictly ordered (required by the
	// read()/login classification in processInput) while different connections run
	// concurrently. The main run() loop only demuxes events onto these lanes.
	workersMu sync.Mutex
	workers   map[int64]*inputLane
}

// Transport readers await Done, so their backpressure is per connection.
// Forced input must never block the dispatcher behind a VM awaiting admission.
type inputLane struct {
	queue []command.InputEvent // guarded by workersMu
	ready chan struct{}
}

func NewInputProcessor(store *dbstore.Store, runtime *engine.Runtime) *InputProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	return &InputProcessor{
		store:      store,
		runtime:    runtime,
		inputQueue: make(chan command.InputEvent, 256),
		ctx:        ctx,
		cancel:     cancel,
		workers:    make(map[int64]*inputLane),
	}
}

func (p *InputProcessor) Start() {
	p.wg.Add(1)
	go p.run()
}

func (p *InputProcessor) Stop() {
	p.runtime.CloseInputAdmission()
	p.cancel()
	// Serialize cancellation with enqueue so every accepted transport event
	// either reaches its lane or has its Done closed here.
	p.enqueueMu.Lock()
	draining := true
	for draining {
		select {
		case evt := <-p.inputQueue:
			evt.Complete()
		default:
			draining = false
		}
	}
	p.enqueueMu.Unlock()
	p.wg.Wait()
}

func (p *InputProcessor) SetConnectionManager(cm *ConnectionManager) {
	p.connManager = cm
	if cm != nil {
		cm.setConnectionHandler(p.HandleConnection)
	}
}

func (p *InputProcessor) EnqueueInput(evt command.InputEvent) {
	p.enqueueMu.RLock()
	defer p.enqueueMu.RUnlock()
	if p.ctx.Err() != nil {
		evt.Complete()
		return
	}
	select {
	case p.inputQueue <- evt:
	case <-p.ctx.Done():
		evt.Complete()
	}
}

// HandleConnection reads transport input and serializes it onto the input queue.
// Each connection's worker executes its input in order.
func (p *InputProcessor) HandleConnection(conn *Connection) {
	trace.Connection("NEW", conn.ID, types.ObjID(-conn.ID), conn.RemoteAddr())

	defer func() {
		done := make(chan struct{})
		p.EnqueueInput(command.InputEvent{
			ConnID:       conn.ID,
			IsDisconnect: true,
			Done:         done,
		})
		<-done
		conn.Close()
	}()

	connectTimeout := 5 * time.Minute
	if p.connManager != nil {
		connectTimeout = p.connManager.connectTimeout
	}
	if value, ok := p.getServerOption(0, "connect_timeout"); ok {
		if value.Type() == types.TYPE_INT && value.Int() > 0 {
			connectTimeout = time.Duration(value.Int()) * time.Second
		}
	}

	// Send the initial welcome banner by enqueuing an empty string to the runtime.
	// This matches ToastStunt behavior: new_input_task(h->tasks, "", 0, 0).
	{
		done := make(chan struct{})
		p.EnqueueInput(command.InputEvent{
			ConnID: conn.ID,
			Player: types.ObjID(-conn.ID),
			Line:   "",
			Done:   done,
		})
		<-done
	}

	for {
		select {
		case <-conn.ctx.Done():
			return
		default:
		}

		if deadlineTransport, ok := conn.transport.(interface{ SetReadDeadline(time.Time) error }); ok {
			if conn.IsLoggedIn() {
				_ = deadlineTransport.SetReadDeadline(time.Time{})
			} else {
				now := time.Now()
				deadline := time.Unix(now.Unix()+int64(connectTimeout/time.Second)+1, 0)
				_ = deadlineTransport.SetReadDeadline(deadline)
			}
		}

		player := conn.GetPlayer()
		if !conn.IsLoggedIn() {
			player = types.ObjID(-conn.ID)
		}

		var line string
		var isOutOfBand bool
		var err error
		if conn.IsLoggedIn() && p.runtime.Session().ConnectionOptionTruthy(player, "binary") {
			if binaryTransport, ok := conn.transport.(BinaryTransport); ok {
				line, err = binaryTransport.ReadChunk()
			} else {
				line, err = conn.ReadLine()
			}
		} else if inputTransport, ok := conn.transport.(InputTransport); ok {
			line, isOutOfBand, err = inputTransport.ReadInput()
		} else {
			line, err = conn.ReadLine()
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && conn.IsLoggedIn() {
				continue
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && !conn.IsLoggedIn() {
				done := make(chan struct{})
				p.EnqueueInput(command.InputEvent{
					ConnID:    conn.ID,
					Player:    types.ObjID(-conn.ID),
					IsTimeout: true,
					Done:      done,
				})
				<-done
				if conn.IsLoggedIn() {
					continue
				}
				return
			}
			slog.Warn("read error", slog.Int64("conn_id", conn.ID), slog.Any("err", err))
			return
		}

		done := make(chan struct{})
		p.EnqueueInput(command.InputEvent{
			ConnID:      conn.ID,
			Player:      player,
			Line:        line,
			IsOutOfBand: isOutOfBand,
			Done:        done,
		})
		<-done
	}
}

func (p *InputProcessor) run() {
	defer p.wg.Done()

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var timerC <-chan time.Time
	var changed <-chan struct{}

	cleanupTicker := time.NewTicker(5 * time.Second)
	defer cleanupTicker.Stop()

	// Scan once at startup, including tasks restored before the loop started.
	runtimeDone := p.processRuntimeTick()
	startBatch := func() {
		timer.Stop()
		timerC = nil
		// Leave notifications buffered while a batch runs. Consuming one here
		// could lose an arrival racing the batch's last readiness scan.
		changed = nil
		runtimeDone = p.processRuntimeTick()
	}
	for {
		select {
		case <-p.ctx.Done():
			return
		case input := <-p.inputQueue:
			p.dispatch(input)
		case <-changed:
			startBatch()
		case <-timerC:
			startBatch()
		case count := <-runtimeDone:
			runtimeDone = nil
			// Drain runnable work without a timer delay between bounded batches.
			// An empty selection waits for an arrival or the next due task.
			if count != 0 && p.ctx.Err() == nil {
				startBatch()
			} else {
				changed = p.runtime.ScheduleChanged()
				if at := p.runtime.NextTaskWake(); !at.IsZero() {
					timer.Reset(time.Until(at))
					timerC = timer.C
				}
			}
		case <-cleanupTicker.C:
			// Reclaim completed/killed tasks so the pre-auth login path (and all
			// other tasks) cannot grow unboundedly.
			p.runtime.CleanupFinishedTasks()
		}
	}
}

func (p *InputProcessor) processRuntimeTick() <-chan int {
	// A select chooses randomly when both input and the runtime tick are
	// ready. Recheck the input queue before running another task so a busy
	// runtime cannot repeatedly win that tie and starve socket input.
	// Dispatching input must not suppress the background selection itself.
	select {
	case input := <-p.inputQueue:
		p.dispatch(input)
	default:
	}
	// The scheduler already executes tasks on worker goroutines. Joining a
	// background pass on the input dispatcher prevents even unrelated login
	// events from reaching their connection lanes until that pass completes.
	// Keep just one batch in flight, and join it during Stop via the wait group.
	done := make(chan int, 1)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		done <- p.runtime.ProcessReadyBatch()
	}()
	return done
}

// dispatch routes an input event onto its connection's serial lane, creating the
// lane (and its goroutine) on first use. Per-connection serialization preserves the
// read()/login ordering invariants of processInput; cross-connection events run
// concurrently.
func (p *InputProcessor) dispatch(input command.InputEvent) {
	p.workersMu.Lock()
	defer p.workersMu.Unlock()
	if p.ctx.Err() != nil {
		input.Complete()
		return
	}
	ch, ok := p.workers[input.ConnID]
	if !ok {
		ch = &inputLane{ready: make(chan struct{}, 1)}
		p.workers[input.ConnID] = ch
		p.wg.Add(1)
		go p.connectionWorker(input.ConnID, ch)
	}
	ch.queue = append(ch.queue, input)
	select {
	case ch.ready <- struct{}{}:
	default:
	}
}

func (p *InputProcessor) connectionWorker(connID int64, ch *inputLane) {
	defer p.wg.Done()
	defer func() {
		p.workersMu.Lock()
		defer p.workersMu.Unlock()
		if p.workers[connID] == ch {
			delete(p.workers, connID)
		}
		for _, input := range ch.queue {
			input.Complete()
		}
	}()
	for {
		if p.ctx.Err() != nil {
			return
		}
		p.workersMu.Lock()
		if len(ch.queue) > 0 {
			input := ch.queue[0]
			ch.queue[0] = command.InputEvent{}
			ch.queue = ch.queue[1:]
			p.workersMu.Unlock()
			p.processInput(input)
			if input.IsDisconnect {
				return
			}
			continue
		}
		if connID < 0 {
			// Synthetic force_input login lanes have no transport disconnect.
			delete(p.workers, connID)
			p.workersMu.Unlock()
			return
		}
		p.workersMu.Unlock()
		select {
		case <-p.ctx.Done():
			return
		case <-ch.ready:
		}
	}
}

func (p *InputProcessor) processInput(input command.InputEvent) {
	defer input.Complete()

	if input.IsDisconnect {
		p.processDisconnect(input)
		return
	}
	if input.IsTimeout {
		p.processLoginTimeout(input)
		return
	}
	if input.ConnID < 0 && input.Player < 0 {
		p.forcePhantomLogin(input.Player, input.Line)
		return
	}
	input.Line = unquoteInBandInput(input.Line)

	oob := strings.HasPrefix(input.Line, "#$#")
	disableOOB := p.runtime.Session().ConnectionOptionTruthy(input.Player, "disable-oob")
	if input.IsOutOfBand || (oob && !disableOOB) {
		if !disableOOB {
			p.processOutOfBand(input)
		}
		return
	}
	if !(oob && !disableOOB) {
		handled, flushed := p.runtime.Session().HandleHeldInput(input.Player, input.Line, false)
		if handled {
			if flushed != nil && p.connManager != nil {
				if conn := p.connManager.getConnectionByConnID(input.ConnID); conn != nil {
					_ = conn.Send(">> Flushing the following pending input:")
					for _, line := range flushed {
						_ = conn.Send(">>     " + line)
					}
					_ = conn.Send(">> (Done flushing)")
				}
			}
			return
		}
	}

	if p.deliverToReadingTask(input.Player, input.Line) {
		return
	}

	if input.Player < 0 {
		p.processPreLogin(input)
		return
	}

	p.processCommand(input)
}

func unquoteInBandInput(line string) string {
	if strings.HasPrefix(line, "#$\"") {
		return line[3:]
	}
	return line
}

func (p *InputProcessor) processLoginTimeout(input command.InputEvent) {
	cm := p.connManager
	if cm == nil {
		return
	}
	conn := cm.getConnectionByConnID(input.ConnID)
	if conn == nil || conn.IsLoggedIn() {
		return
	}
	_ = conn.Send("*** Timed-out waiting for login. ***")
	p.callUserHook(conn.ListenerObject(), "user_disconnected", types.ObjID(-conn.ID))
}

func (p *InputProcessor) processOutOfBand(input command.InputEvent) {
	cm := p.connManager
	if cm == nil {
		return
	}

	conn := cm.getConnectionByConnID(input.ConnID)
	if conn == nil {
		return
	}

	words := command.CommandWordList(input.Line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}
	result := p.runtime.CallVerbWithArgstr(conn.ListenerObject(), "do_out_of_band_command", args, input.Player, input.Line)
	if result.Flow == types.FlowException && result.Error != types.E_VERBNF {
		p.runtime.SendTracebackToPlayer(input.Player, result.Error, result.CallStack)
	}
}

func (p *InputProcessor) deliverToReadingTask(player types.ObjID, line string) bool {
	// Deliver the line to the read()-suspended task and run it synchronously to
	// completion or to its next read() suspend. Running synchronously here (on
	// the single input goroutine) closes the window in which a follow-up line,
	// arriving before the runtime ticker re-ran the resumed task, would not be
	// found by FindReadingTask and would spawn a parallel do_login_command.
	return p.runtime.ResumeReadingTask(player, line)
}

func (p *InputProcessor) ForceInput(player types.ObjID, line string, atFront bool, onProcessed func()) {
	oob := strings.HasPrefix(line, "#$#")
	disableOOB := p.runtime.Session().ConnectionOptionTruthy(player, "disable-oob")
	if !(oob && !disableOOB) {
		handled, _ := p.runtime.Session().HandleHeldInput(player, line, atFront)
		if handled {
			if onProcessed != nil {
				onProcessed()
			}
			return
		}
	}

	// Toast's bf_force_input calls enqueue_input_task: the line joins the
	// connection's input queue and is processed by the server loop like a line
	// that arrived over the network, after the calling task's slice. It is never
	// delivered on the caller's goroutine. Doing so here ran a read()-suspended
	// task inline inside the calling task's builtin; once the caller held the
	// commit gate (its irreversible-effect boundary) and the resumed slice took
	// the gate too, the server deadlocked and no connection could log in again.
	connID := int64(0)
	if p.connManager != nil {
		if conn := p.connManager.GetConnection(player); conn != nil {
			if c, ok := conn.(*Connection); ok {
				connID = c.ID
			}
		}
	}
	if player < 0 && connID == 0 {
		connID = int64(player)
	}
	p.EnqueueInput(command.InputEvent{
		ConnID:      connID,
		Player:      player,
		Line:        line,
		OnProcessed: onProcessed,
	})
}

func (p *InputProcessor) forcePhantomLogin(player types.ObjID, line string) {
	words := command.CommandWordList(line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}
	p.runtime.CallVerbWithArgstr(types.ObjID(0), "do_login_command", args, player, line)
}

func (p *InputProcessor) processDisconnect(input command.InputEvent) {
	cm := p.connManager
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
	replacementActive := false
	if wasLoggedIn {
		if mapped := cm.playerConns[player]; mapped == conn {
			delete(cm.playerConns, player)
			cm.restorePreviousPlayerConnLocked(player, conn)
		} else {
			replacementActive = mapped != nil
			cm.removePlayerHistoryConnLocked(player, conn)
		}
	} else if mapped := cm.playerConns[types.ObjID(-conn.ID)]; mapped == conn {
		delete(cm.playerConns, types.ObjID(-conn.ID))
	}
	cm.mu.Unlock()

	// Kill EVERY login task tied to this connection — the tracked one and any
	// task left suspended on read() from this (negative) connID — so no orphan
	// lingers to swallow input for a future connection that reuses this connID.
	// The task manager matches reading tasks purely by ReadingPlayer, so a
	// per-connID sweep (not a single tracked ID) is required for correctness.
	conn.SetLoginTaskID(0)
	p.runtime.CancelLoginTasksFor(types.ObjID(-conn.ID))

	cm.detachOutboundClient(conn.ID)
	// A superseded connection can close after a replacement has already assumed
	// the same player. Held input belongs to that active player connection, so the
	// stale physical close must not discard its queued commands or HTTP waiter.
	if !replacementActive {
		p.runtime.Session().CloseHeldHTTPInput(player)
	}

	if wasLoggedIn {
		trace.Connection("DISCONNECT", conn.ID, player, "")
	} else {
		trace.Connection("DISCONNECT", conn.ID, types.ObjID(-conn.ID), "unlogged")
	}

	if wasLoggedIn {
		p.callUserHook(handler, "user_client_disconnected", player)
	}

	slog.Info("connection closed", slog.Int64("conn_id", conn.ID))
}

func (p *InputProcessor) processPreLogin(input command.InputEvent) {
	cm := p.connManager
	if cm == nil {
		return
	}

	conn := cm.getConnectionByConnID(input.ConnID)
	if conn == nil {
		return
	}

	line := input.Line
	proxyLine := p.isTrustedProxyConnection(conn) && strings.HasPrefix(line, "PROXY ")
	if proxyLine {
		// PROXY protocol v1: "PROXY TCP4 <src-ip> <dst-ip> <src-port> <dst-port>".
		// Match ToastStunt proxy_rewrite: adopt the announced client IP as the
		// connection's name and address (real remote port preserved), so
		// connection_name() and the trusted-proxy check see the client from
		// here on. The prelude itself is consumed as the connect-time blank.
		if fields := strings.Fields(line); len(fields) >= 3 && net.ParseIP(fields[2]) != nil {
			srcIP := fields[2]
			conn.SetProxiedIP(srcIP)
			conn.SetResolvedName(srcIP)
			slog.Info("proxy name rewritten", slog.Int64("conn_id", conn.ID), slog.String("addr", srcIP))
		}
		line = ""
	}

	// One login task per connection. If a login task is already in flight, do
	// NOT spawn a parallel do_login_command. A read()-suspended login task would
	// have already consumed this line via deliverToReadingTask (run earlier in
	// processInput), so reaching here with a live login task means it is
	// suspended on something other than read() (e.g. suspend()) — the line is
	// dropped rather than starting a competing login. This is the explicit guard
	// against the parallel-spawn race that otherwise orphans the first task.
	if id := conn.GetLoginTaskID(); id != 0 {
		if p.runtime.IsTaskLive(id) {
			return
		}
		conn.SetLoginTaskID(0) // Stale ID for a task that already finished.
	}

	if !proxyLine && !p.shouldCallDoLoginCommand(conn, line) {
		return
	}

	p.dispatchLoginCommand(conn, line)
}

// dispatchLoginCommand runs the listener's do_login_command as a registered,
// resumable engine task. The login verb may call read() any number of times
// (username, password, ...); each read() suspends and resumes the same task.
// When the task finally returns, the completion callback interprets the result
// and logs the player in. Falls back to the synchronous helper when there is no
// do_login_command verb (or it cannot be dispatched).
func (p *InputProcessor) dispatchLoginCommand(conn *Connection, line string) {
	handler := conn.ListenerObject()
	if errCode := p.store.DirectTxn().ObjectExists(handler); errCode != types.E_NONE {
		return
	}

	// No login handler: preserve the existing synchronous fallback.
	if !p.store.HasLocalVerb(handler, "do_login_command") {
		maxBeforeLogin := p.store.DirectTxn().MaxObject()
		player, _ := p.callDoLoginCommand(conn, line)
		if player > 0 {
			p.loginPlayer(conn, player, player > maxBeforeLogin)
		}
		return
	}

	connID := types.ObjID(-conn.ID)
	words := command.CommandWordList(line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}

	maxBeforeLogin := p.store.DirectTxn().MaxObject()
	onStart := func(taskID int64) {
		conn.SetLoginTaskID(taskID)
	}
	onComplete := func(result types.Result) {
		conn.SetLoginTaskID(0)
		// Don't log in a connection that has since disconnected (the live conn
		// for this connID was removed/replaced): that would resurrect a dead
		// connection or hijack a recycled connID.
		if p.connManager == nil || p.connManager.getConnectionByConnID(conn.ID) != conn {
			return
		}
		player := p.interpretLoginResult(conn, result)
		if player > 0 {
			p.loginPlayer(conn, player, player > maxBeforeLogin)
		}
	}

	_, err := p.runtime.CreateLoginHookTask(handler, "do_login_command", args, connID, line, onStart, onComplete)
	if err != nil {
		// The verb exists (checked above) but could not be compiled/dispatched.
		// Do NOT fall back to the synchronous callDoLoginCommand path: that path
		// runs the verb without read() support and would silently regress the
		// very bug this change fixes. Surface the failure instead.
		conn.SetLoginTaskID(0)
		slog.Warn("login task dispatch failed",
			slog.Int64("this", int64(handler)),
			slog.String("verb", "do_login_command"),
			slog.Int64("conn_id", conn.ID),
			slog.Any("err", err))
		return
	}
}

func (p *InputProcessor) processCommand(input command.InputEvent) {
	cm := p.connManager
	if cm == nil {
		return
	}

	conn := cm.getConnectionByConnID(input.ConnID)
	if conn == nil {
		return
	}

	player := conn.GetPlayer()
	location, errCode := p.store.DirectTxn().Location(player)
	if errCode != types.E_NONE {
		return
	}

	if p.processProgrammingInput(conn, input.Line) {
		return
	}

	cmd := command.ParsePlayerCommand(p.store, player, location, input.Line)
	if cmd.Verb == "" {
		return
	}

	if p.executeBeforeDoCommandIntrinsic(conn, player, location, cmd) {
		return
	}

	outputPrefix := conn.GetOutputPrefix()
	outputSuffix := conn.GetOutputSuffix()
	if outputPrefix != "" {
		_ = conn.Send(outputPrefix)
	}

	commandWords := cmd.Words
	if len(commandWords) == 0 {
		commandWords = append([]string{cmd.Verb}, cmd.Args...)
	}
	handled, _ := p.callDoCommand(conn.ListenerObject(), player, commandWords, input.Line, conn.SetLastInputTaskID)
	if handled {
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	match := command.FindVerb(p.store, player, location, cmd)
	if match == nil {
		if p.executeAfterVerbMissIntrinsic(conn, player, cmd, outputSuffix) {
			return
		}

		usePlayerHuh := false
		if option, ok := p.getServerOption(0, "player_huh"); ok {
			usePlayerHuh = option.Truthy()
		}

		if huhMatch := command.FindHuhVerb(p.store, player, location, usePlayerHuh); huhMatch != nil {
			p.executeCommandMatch(conn, player, cmd, huhMatch, outputSuffix, "I couldn't understand that.")
			return
		}
		conn.SetLastInputTaskID(0)
		conn.Send("I couldn't understand that.")
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	p.executeCommandMatch(conn, player, cmd, match, outputSuffix, fmt.Sprintf("[%s has no code]", match.Verb.Name))
}

func (p *InputProcessor) executeCommandMatch(conn *Connection, player types.ObjID, cmd *command.ParsedCommand, match *command.VerbMatch, outputSuffix string, emptyMessage string) {
	err := p.runtime.ExecuteVerbTaskSyncWithStart(player, match, cmd, outputSuffix, conn.SetLastInputTaskID)
	if errors.Is(err, engine.ErrCommandVerbNoCode) {
		conn.Send(emptyMessage)
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}
	if err != nil {
		conn.Send(err.Error())
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
	}
}

func (p *InputProcessor) executeBeforeDoCommandIntrinsic(conn *Connection, player, location types.ObjID, cmd *command.ParsedCommand) bool {
	switch command.LookupIntrinsic(cmd.Verb, command.IntrinsicBeforeDoCommand) {
	case command.IntrinsicProgram:
		p.startProgrammingMode(conn, player, location, cmd.Argstr)
		return true
	case command.IntrinsicPrefix:
		conn.mu.Lock()
		conn.outputPrefix = cmd.Argstr
		conn.mu.Unlock()
		return true
	case command.IntrinsicSuffix:
		conn.mu.Lock()
		conn.outputSuffix = cmd.Argstr
		conn.mu.Unlock()
		return true
	default:
		return false
	}
}

func (p *InputProcessor) executeAfterVerbMissIntrinsic(conn *Connection, player types.ObjID, cmd *command.ParsedCommand, outputSuffix string) bool {
	switch command.LookupIntrinsic(cmd.Verb, command.IntrinsicAfterVerbMiss) {
	case command.IntrinsicEval:
		code := strings.TrimSpace(cmd.Argstr)
		if code != "" {
			p.runtime.StartEval(player, strings.Split(code, "\n"), func(out engine.EvalOutcome) {
				_ = conn.Send(out.CommandOutput())
				if outputSuffix != "" {
					_ = conn.Send(outputSuffix)
				}
			})
		} else if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return true
	default:
		return false
	}
}

func (p *InputProcessor) processProgrammingInput(conn *Connection, line string) bool {
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

	if !p.store.FindLocalVerbForProgramming(target, verbName) {
		conn.Send("Verb not found")
		return true
	}
	_, diagnostics := p.runtime.Registry().Compiler().CompileMOO(lines)
	if len(diagnostics) > 0 {
		for _, diagnostic := range diagnostics {
			conn.Send(diagnostic.Error())
		}
		return true
	}
	if errCode := p.store.DirectTxn().SetVerbCode(target, verbName, lines); errCode != types.E_NONE {
		conn.Send("Verb not found")
		return true
	}
	return true
}

func (p *InputProcessor) startProgrammingMode(conn *Connection, player, location types.ObjID, spec string) {
	target, verbName, ok := p.parseProgramTarget(player, location, spec)
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

func (p *InputProcessor) parseProgramTarget(player, location types.ObjID, spec string) (types.ObjID, string, bool) {
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

	target := types.ObjFailedMatch
	if strings.HasPrefix(objText, "$") && len(objText) > 1 {
		if value, errCode := p.store.DirectTxn().PropertyValue(0, objText[1:]); errCode == types.E_NONE &&
			(value.Type() == types.TYPE_OBJ || value.Type() == types.TYPE_ANON) {
			target = value.ID()
		}
	} else {
		target = command.MatchObject(p.store, player, location, objText)
	}
	if target < 0 {
		return types.ObjNothing, "", false
	}
	if !p.store.FindLocalVerbForProgramming(target, verbName) {
		return types.ObjNothing, "", false
	}
	return target, verbName, true
}

func (p *InputProcessor) YieldReadyTasks() int {
	return p.runtime.ProcessReadyTasks()
}
