package server

import (
	"barn/builtins"
	"barn/types"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Connection represents a player connection
type Connection struct {
	ID            int64
	transport     Transport
	player        types.ObjID
	loggedIn      bool
	outputBuffer  []string
	connectedAt   time.Time
	lastInput     time.Time
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewConnection creates a new connection with a transport
func NewConnection(id int64, transport Transport) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	return &Connection{
		ID:           id,
		transport:    transport,
		player:       types.ObjID(-1), // Not logged in yet
		loggedIn:     false,
		outputBuffer: make([]string, 0),
		connectedAt:  time.Now(),
		lastInput:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Send sends a message to the connection immediately
func (c *Connection) Send(message string) error {
	return c.transport.WriteLine(message)
}

// Buffer adds a message to the output buffer (flushed later)
func (c *Connection) Buffer(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outputBuffer = append(c.outputBuffer, message)
}

// Flush flushes the output buffer
func (c *Connection) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, msg := range c.outputBuffer {
		if err := c.transport.WriteLine(msg); err != nil {
			return err
		}
	}
	c.outputBuffer = c.outputBuffer[:0]
	return nil
}

// ReadLine reads a line of input
func (c *Connection) ReadLine() (string, error) {
	line, err := c.transport.ReadLine()
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.lastInput = time.Now()
	c.mu.Unlock()

	return line, nil
}

// Close closes the connection
func (c *Connection) Close() error {
	c.cancel()
	return c.transport.Close()
}

// RemoteAddr returns the remote address of the connection
func (c *Connection) RemoteAddr() string {
	return c.transport.RemoteAddr()
}

// GetPlayer returns the player ObjID
func (c *Connection) GetPlayer() types.ObjID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.player
}

// SetPlayer sets the player ObjID and marks as logged in
func (c *Connection) SetPlayer(player types.ObjID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.player = player
	c.loggedIn = true
}

// IsLoggedIn returns whether the connection is logged in
func (c *Connection) IsLoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loggedIn
}

// ConnectionManager manages all active connections
type ConnectionManager struct {
	connections    map[int64]*Connection
	playerConns    map[types.ObjID]*Connection // Map player to connection
	nextConnID     int64
	mu             sync.Mutex
	server         *Server
	listeners      []net.Listener
	listenPort     int
	connectTimeout time.Duration
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(server *Server, port int) *ConnectionManager {
	return &ConnectionManager{
		connections:    make(map[int64]*Connection),
		playerConns:    make(map[types.ObjID]*Connection),
		nextConnID:     1,
		server:         server,
		listenPort:     port,
		connectTimeout: 5 * time.Minute,
	}
}

// Listen starts listening for connections
func (cm *ConnectionManager) Listen() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cm.listenPort))
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	cm.listeners = append(cm.listeners, listener)
	log.Printf("Listening on port %d", cm.listenPort)

	go cm.acceptConnections(listener)
	return nil
}

// acceptConnections accepts incoming connections
func (cm *ConnectionManager) acceptConnections(listener net.Listener) {
	for {
		socket, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		cm.handleNewConnection(socket)
	}
}

// handleNewConnection handles a new TCP connection
func (cm *ConnectionManager) handleNewConnection(socket net.Conn) {
	transport := NewTCPTransport(socket)
	conn := cm.NewConnectionFromTransport(transport)

	log.Printf("New connection from %s (ID: %d)", conn.RemoteAddr(), conn.ID)

	// Handle connection in goroutine
	go cm.HandleConnection(conn)
}

// NewConnectionFromTransport creates a connection from any transport (for testing)
func (cm *ConnectionManager) NewConnectionFromTransport(transport Transport) *Connection {
	cm.mu.Lock()
	connID := cm.nextConnID
	cm.nextConnID++
	conn := NewConnection(connID, transport)
	cm.connections[connID] = conn
	// Register with negative ID during unlogged phase (like toaststunt)
	// This allows notify() to reach pre-login connections
	cm.playerConns[types.ObjID(-connID)] = conn
	cm.mu.Unlock()

	return conn
}

// HandleConnection processes a connection (exported for testing)
func (cm *ConnectionManager) HandleConnection(conn *Connection) {
	defer func() {
		cm.removeConnection(conn)
		conn.Close()
	}()

	// Set up timeout for unlogged connections
	timeoutCtx, cancel := context.WithTimeout(conn.ctx, cm.connectTimeout)
	defer cancel()

	// Unlogged phase
	for !conn.IsLoggedIn() {
		select {
		case <-timeoutCtx.Done():
			conn.Send("Connection timeout")
			return
		default:
		}

		line, err := conn.ReadLine()
		if err != nil {
			log.Printf("Connection %d read error: %v", conn.ID, err)
			return
		}

		// Call #0:do_login_command(connection, line)
		player, err := cm.callDoLoginCommand(conn, line)
		if err != nil {
			log.Printf("Login command failed: %v", err)
			continue
		}

		if player > 0 {
			// Login successful
			cm.loginPlayer(conn, player)
			break
		}
	}

	// Command loop
	for {
		select {
		case <-conn.ctx.Done():
			return
		default:
		}

		line, err := conn.ReadLine()
		if err != nil {
			log.Printf("Connection %d read error: %v", conn.ID, err)
			return
		}

		// Dispatch command
		if err := cm.dispatchCommand(conn, line); err != nil {
			log.Printf("Command dispatch error: %v", err)
		}
	}
}

// callDoLoginCommand calls #0:do_login_command(connection, line)
func (cm *ConnectionManager) callDoLoginCommand(conn *Connection, line string) (types.ObjID, error) {
	systemObj := cm.server.store.Get(0)
	if systemObj == nil {
		return types.ObjID(-1), fmt.Errorf("system object not found")
	}

	verb := systemObj.Verbs["do_login_command"]
	if verb == nil {
		// Default login: accept any input and create/return player #2
		conn.Send("Welcome! (No login handler defined)")
		return types.ObjID(2), nil
	}

	connID := types.ObjID(-conn.ID) // Negative ID for unlogged connection

	// Parse line into words for args
	// toaststunt passes parsed words as args to do_login_command
	words := strings.Fields(line)
	args := make([]types.Value, len(words))
	for i, word := range words {
		args[i] = types.NewStr(word)
	}

	log.Printf("[LOGIN] Calling do_login_command with args=%v, connID=%d", args, connID)
	result := cm.server.scheduler.CallVerb(0, "do_login_command", args, connID)
	log.Printf("[LOGIN] do_login_command returned: Flow=%d, Val=%v, Error=%v", result.Flow, result.Val, result.Error)

	if result.Flow == types.FlowException {
		log.Printf("do_login_command error: %v", result.Error)
		return types.ObjID(-1), nil // Login failed, stay unlogged
	}

	// Check if result is a valid player object
	if objVal, ok := result.Val.(types.ObjValue); ok {
		playerID := objVal.ID()
		if playerID > 0 && cm.server.store.Get(playerID) != nil {
			return playerID, nil
		}
	}

	return types.ObjID(-1), nil // Login failed, stay unlogged
}

// loginPlayer associates a connection with a player
func (cm *ConnectionManager) loginPlayer(conn *Connection, player types.ObjID) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Remove negative ID mapping (used for pre-login notify())
	delete(cm.playerConns, types.ObjID(-conn.ID))

	// Check if player already connected
	if existingConn, exists := cm.playerConns[player]; exists {
		// Boot existing connection
		existingConn.Send("You have been disconnected (reconnected elsewhere)")
		existingConn.Close()
		cm.callUserReconnected(player)
	} else {
		cm.callUserConnected(player)
	}

	conn.SetPlayer(player)
	cm.playerConns[player] = conn

	log.Printf("Connection %d logged in as player %d", conn.ID, player)
	conn.Send("Login successful!")
}

// dispatchCommand parses and dispatches a command
func (cm *ConnectionManager) dispatchCommand(conn *Connection, line string) error {
	player := conn.GetPlayer()
	playerObj := cm.server.store.Get(player)
	if playerObj == nil {
		return fmt.Errorf("player object not found")
	}
	location := playerObj.Location

	// Parse the command
	cmd := ParseCommand(line)
	if cmd.Verb == "" {
		return nil // Empty command
	}

	// Resolve direct object
	if cmd.Dobjstr != "" {
		cmd.Dobj = MatchObject(cm.server.store, player, location, cmd.Dobjstr)
	}

	// Resolve indirect object
	if cmd.Iobjstr != "" {
		cmd.Iobj = MatchObject(cm.server.store, player, location, cmd.Iobjstr)
	}

	// Find the verb
	match := FindVerb(cm.server.store, player, location, cmd)
	if match == nil {
		// Try #0:do_command as fallback
		systemObj := cm.server.store.Get(0)
		if systemObj != nil {
			if verb := systemObj.Verbs["do_command"]; verb != nil {
				// TODO: Call #0:do_command(verb, argstr, dobj, iobj, ...)
				conn.Send(fmt.Sprintf("I don't understand that. (verb=%s)", cmd.Verb))
				return nil
			}
		}
		conn.Send(fmt.Sprintf("I don't understand that."))
		return nil
	}

	// Execute the verb
	if match.Verb.Program == nil || len(match.Verb.Program.Statements) == 0 {
		conn.Send(fmt.Sprintf("[%s has no code]", match.Verb.Name))
		return nil
	}

	// Create task to execute the verb
	cm.server.scheduler.CreateVerbTask(player, match, cmd)

	return nil
}

// removeConnection removes a connection
func (cm *ConnectionManager) removeConnection(conn *Connection) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.connections, conn.ID)
	if conn.IsLoggedIn() {
		delete(cm.playerConns, conn.GetPlayer())
		cm.callUserDisconnected(conn.GetPlayer())
	}

	log.Printf("Connection %d closed", conn.ID)
}

// callUserConnected calls #0:user_connected(player)
func (cm *ConnectionManager) callUserConnected(player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := cm.server.scheduler.CallVerb(0, "user_connected", args, player)
	if result.Flow == types.FlowException {
		log.Printf("user_connected error: %v", result.Error)
	}
}

// callUserReconnected calls #0:user_reconnected(player)
func (cm *ConnectionManager) callUserReconnected(player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := cm.server.scheduler.CallVerb(0, "user_reconnected", args, player)
	if result.Flow == types.FlowException {
		log.Printf("user_reconnected error: %v", result.Error)
	}
}

// callUserDisconnected calls #0:user_disconnected(player)
func (cm *ConnectionManager) callUserDisconnected(player types.ObjID) {
	args := []types.Value{types.NewObj(player)}
	result := cm.server.scheduler.CallVerb(0, "user_disconnected", args, player)
	if result.Flow == types.FlowException {
		log.Printf("user_disconnected error: %v", result.Error)
	}
}

// GetConnection returns a connection by player ID
// Supports negative IDs for unlogged connections
func (cm *ConnectionManager) GetConnection(player types.ObjID) builtins.Connection {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Try direct lookup first (works for both positive and negative IDs)
	conn := cm.playerConns[player]
	if conn != nil {
		return conn
	}

	// If negative ID not found in playerConns, try connections map
	if player < 0 {
		connID := int64(-player)
		if conn, ok := cm.connections[connID]; ok {
			return conn
		}
	}

	return nil
}

// ConnectedPlayers returns list of connected player ObjIDs
func (cm *ConnectionManager) ConnectedPlayers() []types.ObjID {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	players := make([]types.ObjID, 0, len(cm.playerConns))
	for player := range cm.playerConns {
		players = append(players, player)
	}
	return players
}

// BootPlayer disconnects a player
func (cm *ConnectionManager) BootPlayer(player types.ObjID) error {
	cm.mu.Lock()
	conn := cm.playerConns[player]
	cm.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("player not connected")
	}

	conn.Send("You have been disconnected")
	conn.Close()
	return nil
}

// SwitchPlayer switches a connection from one player to another
// This is used during login to switch from negative connection ID to actual player
func (cm *ConnectionManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Find connection for old player
	conn := cm.playerConns[oldPlayer]
	if conn == nil {
		// Try looking up by connection ID if oldPlayer is negative
		if oldPlayer < 0 {
			connID := int64(-oldPlayer)
			conn = cm.connections[connID]
		}
	}

	if conn == nil {
		return fmt.Errorf("old player not connected")
	}

	// Remove old player mapping
	delete(cm.playerConns, oldPlayer)

	// Check if new player is already connected (reconnection)
	if existingConn, exists := cm.playerConns[newPlayer]; exists && existingConn != conn {
		// Boot existing connection
		existingConn.Send("You have been disconnected (reconnected elsewhere)")
		existingConn.Close()
	}

	// Set up new player
	conn.SetPlayer(newPlayer)
	cm.playerConns[newPlayer] = conn

	log.Printf("Switched connection %d from player %d to %d", conn.ID, oldPlayer, newPlayer)
	return nil
}
