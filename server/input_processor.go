package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"barn/builtins"
	"barn/bytecode"
	"barn/command"
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
}

func NewInputProcessor(store *dbstore.Store, runtimeScheduler *runtime.Scheduler) *InputProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	return &InputProcessor{
		store:      store,
		runtime:    runtimeScheduler,
		inputQueue: make(chan command.InputEvent, 256),
		ctx:        ctx,
		cancel:     cancel,
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
}

func (p *InputProcessor) EnqueueInput(evt command.InputEvent) {
	p.inputQueue <- evt
}

func (p *InputProcessor) run() {
	defer p.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case input := <-p.inputQueue:
			p.processInput(input)
		case <-ticker.C:
			p.runtime.ProcessReadyTasks()
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

func (p *InputProcessor) deliverToReadingTask(player types.ObjID, line string) bool {
	mgr := task.GetManager()
	t := mgr.FindReadingTask(player)
	if t == nil {
		return false
	}
	t.ReadingPlayer = types.ObjNothing
	t.Resume(types.NewStr(line))
	return true
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
	cm.detachOutboundClient(conn.ID)
	builtins.CloseHeldHTTPInput(player)

	if wasLoggedIn {
		trace.Connection("DISCONNECT", conn.ID, player, "")
	} else {
		trace.Connection("DISCONNECT", conn.ID, types.ObjID(-conn.ID), "unlogged")
	}

	if wasLoggedIn {
		p.callUserClientDisconnected(handler, player)
	}

	log.Printf("Connection %d closed", conn.ID)
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
		line = ""
	}

	if !proxyLine && !p.shouldCallDoLoginCommand(conn, line) {
		return
	}

	maxBeforeLogin := p.store.MaxObject()
	player, _ := p.callDoLoginCommand(conn, line)
	if player > 0 {
		p.loginPlayer(conn, player, player > maxBeforeLogin)
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

	if strings.HasPrefix(input.Line, "#$#") && !builtins.ConnectionOptionTruthy(player, "disable-oob") {
		words := command.CommandWordList(input.Line)
		args := make([]types.Value, len(words))
		for i, word := range words {
			args[i] = types.NewStr(word)
		}
		result := p.runtime.CallVerbWithArgstr(conn.ListenerObject(), "do_out_of_band_command", args, player, input.Line)
		if result.Flow == types.FlowException && result.Error != types.E_VERBNF {
			var stack []task.ActivationFrame
			if result.CallStack != nil {
				if st, ok := result.CallStack.([]task.ActivationFrame); ok {
					stack = st
				}
			}
			p.runtime.SendTracebackToPlayer(player, result.Error, stack)
		}
		return
	}

	cmd := command.ParseCommand(input.Line)
	if cmd.Verb == "" {
		return
	}

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
		p.startProgrammingMode(conn, player, location, cmd.Argstr)
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

	if cmd.Dobjstr != "" {
		cmd.Dobj = command.MatchObject(p.store, player, location, cmd.Dobjstr)
	}
	if cmd.Iobjstr != "" {
		cmd.Iobj = command.MatchObject(p.store, player, location, cmd.Iobjstr)
	}

	match := command.FindVerb(p.store, player, location, cmd)
	if match == nil {
		if verbUpper == "EVAL" {
			code := strings.TrimSpace(cmd.Argstr)
			if code != "" {
				for _, line := range p.runtime.EvalCommandOutput(player, code, conn.GetOutputPrefix(), conn.GetOutputSuffix()) {
					_ = conn.Send(line)
				}
			}
			if outputSuffix != "" {
				_ = conn.Send(outputSuffix)
			}
			return
		}

		usePlayerHuh := false
		if option, ok := p.getServerOption(0, "player_huh"); ok {
			usePlayerHuh = option.Truthy()
		}

		huhTarget := location
		if usePlayerHuh {
			huhTarget = player
		}

		if huhVerb, huhVerbLoc, err := p.store.FindVerb(huhTarget, "huh"); err == nil {
			huhMatch := &command.VerbMatch{
				Verb:    huhVerb,
				This:    huhTarget,
				VerbLoc: huhVerbLoc,
			}

			if huhMatch.Statements == nil && len(huhMatch.Verb.Code) > 0 {
				program, errors := bytecode.CompileVerb(huhMatch.Verb.Code)
				if len(errors) > 0 {
					conn.Send(fmt.Sprintf("Verb compile error: %s", errors[0]))
					if outputSuffix != "" {
						_ = conn.Send(outputSuffix)
					}
					return
				}
				huhMatch.Statements = program.Statements
			}

			if len(huhMatch.Statements) == 0 {
				conn.Send("I couldn't understand that.")
				if outputSuffix != "" {
					_ = conn.Send(outputSuffix)
				}
				return
			}

			p.runtime.ExecuteVerbTaskSync(player, huhMatch, cmd, outputSuffix)
			return
		}
		conn.Send("I couldn't understand that.")
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	if match.Statements == nil && len(match.Verb.Code) > 0 {
		program, errors := bytecode.CompileVerb(match.Verb.Code)
		if len(errors) > 0 {
			conn.Send(fmt.Sprintf("Verb compile error: %s", errors[0]))
			if outputSuffix != "" {
				_ = conn.Send(outputSuffix)
			}
			return
		}
		match.Statements = program.Statements
	}

	if len(match.Statements) == 0 {
		conn.Send(fmt.Sprintf("[%s has no code]", match.Verb.Name))
		if outputSuffix != "" {
			_ = conn.Send(outputSuffix)
		}
		return
	}

	p.runtime.ExecuteVerbTaskSync(player, match, cmd, outputSuffix)
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
	_, errors := bytecode.CompileVerb(lines)
	if len(errors) > 0 {
		for _, errText := range errors {
			conn.Send(errText)
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
