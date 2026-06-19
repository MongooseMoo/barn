package server

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	dbstore "barn/db/store"
	"barn/task"
	"barn/trace"
	"barn/types"
)

// shouldCallDoLoginCommand checks whether do_login_command should be called
// for the given input. Trusted proxy blank lines route through do_blank_command first.
func (s *Scheduler) shouldCallDoLoginCommand(conn *Connection, line string) bool {
	// A trusted-proxy blank line routes through do_blank_command first.
	if line == "" && s.isTrustedProxyConnection(conn) {
		allowLogin, err := s.callDoBlankCommand(conn, line)
		if err != nil {
			log.Printf("do_blank_command failed: %v", err)
			return false
		}
		return allowLogin
	}

	// Otherwise always call do_login_command — including for the empty line the
	// connection manager enqueues on connect, matching ToastStunt's
	// new_input_task(h->tasks, "", 0, 0). A listener whose do_login_command
	// returns a player without consuming input thus logs in at connect time.
	return true
}

// callDoLoginCommand calls #0:do_login_command with the given line.
// Returns the player ObjID if login succeeded, or a negative value on failure.
func (s *Scheduler) callDoLoginCommand(conn *Connection, line string) (types.ObjID, error) {
	handler := conn.ListenerObject()
	if errCode := s.store.ObjectExists(handler); errCode != types.E_NONE {
		return types.ObjID(-1), fmt.Errorf("listener object not found")
	}

	if !s.store.HasLocalVerb(handler, "do_login_command") {
		conn.Send("Welcome! (No login handler defined)")
		return types.ObjID(2), nil
	}

	connID := types.ObjID(-conn.ID)

	words := commandWordList(line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}

	result := s.CallVerbWithArgstr(handler, "do_login_command", args, connID, line)

	if result.Flow == types.FlowException {
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		lines := task.FormatTraceback(stack, result.Error)
		for _, line := range lines {
			conn.Send(line)
		}
		return types.ObjID(-1), nil
	}

	if objVal, ok := result.Val.(types.ObjValue); ok {
		playerID := objVal.ID()
		if playerID > 0 {
			hasPlayerFlag, errCode := s.store.HasObjectFlag(playerID, dbstore.FlagUser)
			if errCode == types.E_NONE && hasPlayerFlag {
				return playerID, nil
			}
		}
	}

	// Check if switch_player was called during the verb execution
	currentPlayer := conn.GetPlayer()
	if currentPlayer > 0 {
		return currentPlayer, nil
	}

	return types.ObjID(-1), nil
}

// callDoBlankCommand calls #0:do_blank_command and returns whether login should proceed.
func (s *Scheduler) callDoBlankCommand(conn *Connection, line string) (bool, error) {
	words := commandWordList(line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}

	connID := types.ObjID(-conn.ID)
	result := s.CallVerbWithArgstr(conn.ListenerObject(), "do_blank_command", args, connID, line)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return false, nil
		}

		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		lines := task.FormatTraceback(stack, result.Error)
		for _, line := range lines {
			conn.Send(line)
		}
		return false, nil
	}

	if result.Val == nil {
		return false, nil
	}
	return result.Val.Truthy(), nil
}

// callDoCommand calls #0:do_command(command) and returns whether command was handled.
func (s *Scheduler) callDoCommand(handler types.ObjID, player types.ObjID, words []string, argstr string) (bool, error) {
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}
	result := s.CallVerbWithArgstr(handler, "do_command", args, player, argstr)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return false, nil
		}

		log.Printf("do_command error: %v", result.Error)
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		s.sendTracebackToPlayer(player, result.Error, stack)
		return true, nil
	}

	if result.Val == nil {
		return false, nil
	}
	return result.Val.Truthy(), nil
}

// callUserConnected calls #0:user_connected(player)
func (s *Scheduler) callUserConnected(handler types.ObjID, player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := s.CallVerb(handler, "user_connected", args, player)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return
		}
		log.Printf("user_connected error: %v", result.Error)
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		s.sendTracebackToPlayer(player, result.Error, stack)
	}
}

// callUserCreated calls handler:user_created(player)
func (s *Scheduler) callUserCreated(handler types.ObjID, player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := s.CallVerb(handler, "user_created", args, player)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return
		}
		log.Printf("user_created error: %v", result.Error)
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		s.sendTracebackToPlayer(player, result.Error, stack)
	}
}

// callUserReconnected calls #0:user_reconnected(player)
func (s *Scheduler) callUserReconnected(handler types.ObjID, player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := s.CallVerb(handler, "user_reconnected", args, player)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return
		}
		log.Printf("user_reconnected error: %v", result.Error)
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		s.sendTracebackToPlayer(player, result.Error, stack)
	}
}

// callUserDisconnected calls #0:user_disconnected(player)
func (s *Scheduler) callUserDisconnected(handler types.ObjID, player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := s.CallVerb(handler, "user_disconnected", args, player)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return
		}
		log.Printf("user_disconnected error: %v", result.Error)
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		s.sendTracebackToPlayer(player, result.Error, stack)
	}
}

// callUserClientDisconnected calls handler:user_client_disconnected(player)
func (s *Scheduler) callUserClientDisconnected(handler types.ObjID, player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := s.CallVerb(handler, "user_client_disconnected", args, player)
	if result.Flow == types.FlowException {
		if result.Error == types.E_VERBNF {
			return
		}
		log.Printf("user_client_disconnected error: %v", result.Error)
		var stack []task.ActivationFrame
		if result.CallStack != nil {
			if st, ok := result.CallStack.([]task.ActivationFrame); ok {
				stack = st
			}
		}
		s.sendTracebackToPlayer(player, result.Error, stack)
	}
}

// connectMessage returns the server_options.connect_msg value,
// falling back to "*** Connected ***" if not set.
func (s *Scheduler) connectMessage() string {
	if val, ok := s.getServerOption(0, "connect_msg"); ok {
		if strVal, ok := val.(types.StrValue); ok && strVal.Value() != "" {
			return strVal.Value()
		}
	}
	return "*** Connected ***"
}

// loginPlayer associates a connection with a player.
// Called on the scheduler goroutine after a successful do_login_command.
func (s *Scheduler) loginPlayer(conn *Connection, player types.ObjID, newlyCreated bool) {
	cm := s.connManager
	if cm == nil {
		return
	}

	cm.mu.Lock()

	// Remove negative ID mapping (used for pre-login notify())
	delete(cm.playerConns, types.ObjID(-conn.ID))

	// Check if player already connected
	alreadyLoggedIn := false
	reconnection := false
	var existingConn *Connection
	if ec, exists := cm.playerConns[player]; exists {
		if ec == conn {
			alreadyLoggedIn = true
		} else {
			existingConn = ec
			reconnection = true
		}
	}

	if !alreadyLoggedIn {
		conn.SetPlayer(player)
		conn.ConnectionTime = time.Now()
		cm.playerConns[player] = conn
	}

	cm.mu.Unlock()

	// Trace login event
	if reconnection {
		trace.Connection("RECONNECT", conn.ID, player, "")
	} else {
		trace.Connection("LOGIN", conn.ID, player, "")
	}

	// Call hooks on the scheduler goroutine
	if alreadyLoggedIn {
		// Ensure ConnectionTime is set even if switch_player handled login
		if conn.ConnectionTime.IsZero() {
			conn.ConnectionTime = time.Now()
		}
		log.Printf("Connection %d already logged in as player %d via switch_player", conn.ID, player)
		if conn.ListenerObject() == 0 || conn.PrintMessages() {
			_ = conn.Send(s.connectMessage())
		}
		if newlyCreated {
			s.callUserCreated(conn.ListenerObject(), player)
		}
		s.callUserConnected(conn.ListenerObject(), player)
		return
	}

	if reconnection {
		existingConn.Send("*** Redirecting connection to new port ***")
		s.callUserClientDisconnected(existingConn.ListenerObject(), player)
		if conn.ListenerObject() == 0 || conn.PrintMessages() {
			_ = conn.Send(s.connectMessage())
		}
		s.callUserConnected(conn.ListenerObject(), player)
	} else {
		if conn.ListenerObject() == 0 || conn.PrintMessages() {
			_ = conn.Send(s.connectMessage())
		}
		if newlyCreated {
			s.callUserCreated(conn.ListenerObject(), player)
		}
		s.callUserConnected(conn.ListenerObject(), player)
	}

	log.Printf("Connection %d logged in as player %d", conn.ID, player)
}

// isTrustedProxyConnection checks if a connection's IP is in the trusted proxies list.
func (s *Scheduler) isTrustedProxyConnection(conn *Connection) bool {
	trustedProxies, ok := s.getServerOption(0, "trusted_proxies")
	if !ok {
		return false
	}

	addr := conn.RemoteAddr()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := strings.Trim(host, "[]")
	if ip == "" {
		return false
	}

	return listContainsString(trustedProxies, ip)
}

// getServerOption looks up a server option from the server_options property.
func (s *Scheduler) getServerOption(listener types.ObjID, name string) (types.Value, bool) {
	serverOptions, err := s.store.FindProperty(listener, "server_options")
	if err != types.E_NONE && listener != 0 {
		serverOptions, err = s.store.FindProperty(0, "server_options")
	}
	if err != types.E_NONE {
		return nil, false
	}

	serverOptionsObj, ok := serverOptions.Value.(types.ObjValue)
	if !ok {
		return nil, false
	}

	prop, err := s.store.FindProperty(serverOptionsObj.ID(), name)
	if err != types.E_NONE {
		return nil, false
	}
	return prop.Value, true
}
