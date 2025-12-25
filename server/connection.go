package server

import (
	"barn/parser"
	"barn/types"
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Connection represents a player connection
type Connection struct {
	ID            int64
	socket        net.Conn
	player        types.ObjID
	loggedIn      bool
	reader        *bufio.Reader
	writer        *bufio.Writer
	outputBuffer  []string
	connectedAt   time.Time
	lastInput     time.Time
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewConnection creates a new connection
func NewConnection(id int64, socket net.Conn) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	return &Connection{
		ID:           id,
		socket:       socket,
		player:       types.ObjID(-1), // Not logged in yet
		loggedIn:     false,
		reader:       bufio.NewReader(socket),
		writer:       bufio.NewWriter(socket),
		outputBuffer: make([]string, 0),
		connectedAt:  time.Now(),
		lastInput:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Send sends a message to the connection
func (c *Connection) Send(message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.writer.WriteString(message + "\n")
	if err != nil {
		return err
	}
	return c.writer.Flush()
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
		if _, err := c.writer.WriteString(msg + "\n"); err != nil {
			return err
		}
	}
	c.outputBuffer = c.outputBuffer[:0]
	return c.writer.Flush()
}

// ReadLine reads a line of input
func (c *Connection) ReadLine() (string, error) {
	line, err := c.reader.ReadString('\n')
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
	return c.socket.Close()
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

// handleNewConnection handles a new connection
func (cm *ConnectionManager) handleNewConnection(socket net.Conn) {
	cm.mu.Lock()
	connID := cm.nextConnID
	cm.nextConnID++
	conn := NewConnection(connID, socket)
	cm.connections[connID] = conn
	cm.mu.Unlock()

	log.Printf("New connection from %s (ID: %d)", socket.RemoteAddr(), connID)

	// Handle connection in goroutine
	go cm.handleConnection(conn)
}

// handleConnection processes a connection
func (cm *ConnectionManager) handleConnection(conn *Connection) {
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

	// TODO: Actually call the verb and get the return value
	// For now, just return success
	return types.ObjID(2), nil
}

// loginPlayer associates a connection with a player
func (cm *ConnectionManager) loginPlayer(conn *Connection, player types.ObjID) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

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
	// Parse command
	// For now, just echo back
	conn.Send(fmt.Sprintf("You said: %s", line))

	// TODO: Parse command and dispatch to verb
	// Create foreground task
	code := []parser.Stmt{} // Empty for now
	cm.server.scheduler.CreateForegroundTask(conn.GetPlayer(), code)

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
	systemObj := cm.server.store.Get(0)
	if systemObj == nil {
		return
	}

	verb := systemObj.Verbs["user_connected"]
	if verb == nil {
		return
	}

	// TODO: Actually call the verb
	log.Printf("Would call #0:user_connected(%d)", player)
}

// callUserReconnected calls #0:user_reconnected(player)
func (cm *ConnectionManager) callUserReconnected(player types.ObjID) {
	systemObj := cm.server.store.Get(0)
	if systemObj == nil {
		return
	}

	verb := systemObj.Verbs["user_reconnected"]
	if verb == nil {
		return
	}

	// TODO: Actually call the verb
	log.Printf("Would call #0:user_reconnected(%d)", player)
}

// callUserDisconnected calls #0:user_disconnected(player)
func (cm *ConnectionManager) callUserDisconnected(player types.ObjID) {
	systemObj := cm.server.store.Get(0)
	if systemObj == nil {
		return
	}

	verb := systemObj.Verbs["user_disconnected"]
	if verb == nil {
		return
	}

	// TODO: Actually call the verb
	log.Printf("Would call #0:user_disconnected(%d)", player)
}

// GetConnection returns a connection by player ID
func (cm *ConnectionManager) GetConnection(player types.ObjID) *Connection {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.playerConns[player]
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
