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

	"barn/builtins"
	"barn/command"
	"barn/compiler"
	dbstore "barn/db/store"
	runtime "barn/scheduler"
	"barn/task"
	"barn/trace"
	"barn/types"
)

type InputProcessor struct {
	store       *dbstore.Store
	runtime     *runtime.Scheduler
	connManager *ConnectionManager
	inputQueue  chan command.InputEvent
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	// Each connection's input is processed on its own goroutine (keyed by ConnID),
	// so a single connection's lines stay strictly ordered (required by the
	// read()/login classification in processInput) while different connections run
	// concurrently. The main run() loop only demuxes events onto these lanes.
	workersMu sync.Mutex
	workers   map[int64]chan command.InputEvent
}

func NewInputProcessor(store *dbstore.Store, runtimeScheduler *runtime.Scheduler) *InputProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	return &InputProcessor{
		store:      store,
		runtime:    runtimeScheduler,
		inputQueue: make(chan command.InputEvent, 256),
		ctx:        ctx,
		cancel:     cancel,
		workers:    make(map[int64]chan command.InputEvent),
	}
}

func (p *InputProcessor) Start() {
	p.wg.Add(1)
	go p.run()
}

func (p *InputProcessor) Stop() {
	p.cancel()
	p.wg.Wait()
}

func (p *InputProcessor) SetConnectionManager(cm *ConnectionManager) {
	p.connManager = cm
	if cm != nil {
		cm.setConnectionHandler(p.HandleConnection)
	}
}

func (p *InputProcessor) EnqueueInput(evt command.InputEvent) {
	p.inputQueue <- evt
}

// HandleConnection reads transport input and serializes it onto the input queue.
// All MOO verb execution remains on the scheduler/input goroutine.
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

	// Send initial welcome banner by enqueuing empty string to scheduler.
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
		if conn.IsLoggedIn() && builtins.ConnectionOptionTruthy(player, "binary") {
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
				conn.Send("*** Timed-out waiting for login. ***")
				p.callUserHook(conn.ListenerObject(), "user_disconnected", types.ObjID(-conn.ID))
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

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(5 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case input := <-p.inputQueue:
			p.dispatch(input)
		case <-ticker.C:
			p.processSchedulerTick()
		case <-cleanupTicker.C:
			// Reclaim completed/killed tasks so the pre-auth login path (and all
			// other tasks) cannot grow unboundedly.
			p.runtime.CleanupFinishedTasks()
		}
	}
}

func (p *InputProcessor) processSchedulerTick() {
	// A select chooses randomly when both input and the scheduler tick are
	// ready. Recheck the input queue before running another task so a busy
	// scheduler cannot repeatedly win that tie and starve socket input.
	select {
	case input := <-p.inputQueue:
		p.dispatch(input)
	default:
		p.runtime.ProcessReadyTasks()
	}
}

// dispatch routes an input event onto its connection's serial lane, creating the
// lane (and its goroutine) on first use. Per-connection serialization preserves the
// read()/login ordering invariants of processInput; cross-connection events run
// concurrently.
func (p *InputProcessor) dispatch(input command.InputEvent) {
	p.workersMu.Lock()
	ch, ok := p.workers[input.ConnID]
	if !ok {
		ch = make(chan command.InputEvent, 64)
		p.workers[input.ConnID] = ch
		p.wg.Add(1)
		go p.connectionWorker(input.ConnID, ch)
	}
	p.workersMu.Unlock()

	select {
	case ch <- input:
	case <-p.ctx.Done():
	}
}

func (p *InputProcessor) connectionWorker(connID int64, ch chan command.InputEvent) {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case input := <-ch:
			p.processInput(input)
			if input.IsDisconnect {
				// The connection is gone; retire its lane. A later event for a reused
				// ConnID will spin up a fresh lane.
				p.workersMu.Lock()
				if p.workers[connID] == ch {
					delete(p.workers, connID)
				}
				p.workersMu.Unlock()
				return
			}
		}
	}
}

func (p *InputProcessor) processInput(input command.InputEvent) {
	defer func() {
		if input.Done != nil {
			close(input.Done)
		}
	}()

	if input.IsDisconnect {
		p.processDisconnect(input)
		return
	}

	oob := strings.HasPrefix(input.Line, "#$#")
	disableOOB := builtins.ConnectionOptionTruthy(input.Player, "disable-oob")
	if input.IsOutOfBand || (oob && !disableOOB) {
		if !disableOOB {
			p.processOutOfBand(input)
		}
		return
	}
	if !(oob && !disableOOB) && builtins.HandleHeldInput(input.Player, input.Line, false) {
		return
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
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		p.runtime.SendTracebackToPlayer(input.Player, result.Error, stack)
	}
}

func (p *InputProcessor) deliverToReadingTask(player types.ObjID, line string) bool {
	// Deliver the line to the read()-suspended task and run it synchronously to
	// completion or to its next read() suspend. Running synchronously here (on
	// the single input goroutine) closes the window in which a follow-up line,
	// arriving before the scheduler ticker re-ran the resumed task, would not be
	// found by FindReadingTask and would spawn a parallel do_login_command.
	return p.runtime.ResumeReadingTask(player, line)
}

func (p *InputProcessor) ForceInput(player types.ObjID, line string, atFront bool) {
	oob := strings.HasPrefix(line, "#$#")
	disableOOB := builtins.ConnectionOptionTruthy(player, "disable-oob")
	if !(oob && !disableOOB) && builtins.HandleHeldInput(player, line, atFront) {
		return
	}

	if p.deliverToReadingTask(player, line) {
		return
	}

	connID := int64(0)
	if p.connManager != nil {
		if conn := p.connManager.GetConnection(player); conn != nil {
			if c, ok := conn.(*Connection); ok {
				connID = c.ID
			}
		}
	}
	if player < 0 && connID == 0 {
		p.forcePhantomLogin(player, line)
		return
	}

	evt := command.InputEvent{
		ConnID: connID,
		Player: player,
		Line:   line,
	}
	if player < 0 && connID != 0 {
		p.processInput(evt)
		return
	}
	p.inputQueue <- evt
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
	if wasLoggedIn {
		if mapped := cm.playerConns[player]; mapped == conn {
			delete(cm.playerConns, player)
			cm.restorePreviousPlayerConnLocked(player, conn)
		} else {
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
	builtins.CloseHeldHTTPInput(player)

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
// resumable scheduler task. The login verb may call read() any number of times
// (username, password, ...); each read() suspends and resumes the same task.
// When the task finally returns, the completion callback interprets the result
// and logs the player in. Falls back to the synchronous helper when there is no
// do_login_command verb (or it cannot be dispatched).
func (p *InputProcessor) dispatchLoginCommand(conn *Connection, line string) {
	handler := conn.ListenerObject()
	if errCode := p.store.ObjectExists(handler); errCode != types.E_NONE {
		return
	}

	// No login handler: preserve the existing synchronous fallback.
	if !p.store.HasLocalVerb(handler, "do_login_command") {
		maxBeforeLogin := p.store.MaxObject()
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

	maxBeforeLogin := p.store.MaxObject()
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
	location, errCode := p.store.Location(player)
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
	handled, _ := p.callDoCommand(conn.ListenerObject(), player, commandWords, input.Line)
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
		conn.Send("I couldn't understand that.")
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	p.executeCommandMatch(conn, player, cmd, match, outputSuffix, fmt.Sprintf("[%s has no code]", match.Verb.Name))
}

func (p *InputProcessor) executeCommandMatch(conn *Connection, player types.ObjID, cmd *command.ParsedCommand, match *command.VerbMatch, outputSuffix string, emptyMessage string) {
	err := p.runtime.ExecuteVerbTaskSync(player, match, cmd, outputSuffix)
	if errors.Is(err, runtime.ErrCommandVerbNoCode) {
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
			for _, line := range p.runtime.EvalCommandOutput(player, code, conn.GetOutputPrefix(), conn.GetOutputSuffix()) {
				_ = conn.Send(line)
			}
		}
		if outputSuffix != "" {
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
	_, diagnostics := compiler.CompileMOO(lines, p.runtime.Registry())
	if len(diagnostics) > 0 {
		for _, diagnostic := range diagnostics {
			conn.Send(diagnostic.Error())
		}
		return true
	}
	if errCode := p.store.SetVerbCode(target, verbName, lines); errCode != types.E_NONE {
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

	target := command.MatchObject(p.store, player, location, objText)
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
