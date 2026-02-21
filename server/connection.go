package server

import (
	"barn/builtins"
	"barn/trace"
	"barn/types"
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Connection represents a player connection
type Connection struct {
	ID             int64
	transport      Transport
	player         types.ObjID
	loggedIn       bool
	outputBuffer   []string
	outputPrefix   string // PREFIX/OUTPUTPREFIX command sets this
	outputSuffix   string // SUFFIX/OUTPUTSUFFIX command sets this
	connectedAt    time.Time
	ConnectionTime time.Time // Set when login completes (zero means not yet logged in)
	lastInput      time.Time
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
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

// GetOutputPrefix returns the connection's output prefix
func (c *Connection) GetOutputPrefix() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputPrefix
}

// GetOutputSuffix returns the connection's output suffix
func (c *Connection) GetOutputSuffix() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputSuffix
}

// BufferedOutputLength returns the number of queued output lines.
func (c *Connection) BufferedOutputLength() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.outputBuffer)
}

// ConnectedSeconds returns how long the connection has been active.
func (c *Connection) ConnectedSeconds() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seconds := time.Since(c.connectedAt).Seconds()
	if seconds < 0 {
		return 0
	}
	return int64(seconds)
}

// IdleSeconds returns how long since the last input was received.
func (c *Connection) IdleSeconds() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seconds := time.Since(c.lastInput).Seconds()
	if seconds < 0 {
		return 0
	}
	return int64(seconds)
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
		nextConnID:     2, // Start at 2 so first connection is -2 (not -1 which is NOTHING)
		server:         server,
		listenPort:     port,
		connectTimeout: 5 * time.Minute,
	}
}

// GetListenPort returns the port the server is listening on
func (cm *ConnectionManager) GetListenPort() int {
	return cm.listenPort
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

// HandleConnection processes a connection (exported for testing).
// This is now an I/O-only loop: it reads lines and enqueues InputEvents
// for the scheduler to process. All MOO verb execution happens on the
// scheduler goroutine.
func (cm *ConnectionManager) HandleConnection(conn *Connection) {
	// Trace new connection
	trace.Connection("NEW", conn.ID, types.ObjID(-conn.ID), conn.RemoteAddr())

	defer func() {
		// Enqueue disconnect event and wait for it to be processed
		done := make(chan struct{})
		cm.server.scheduler.EnqueueInput(InputEvent{
			ConnID:       conn.ID,
			IsDisconnect: true,
			Done:         done,
		})
		<-done
		conn.Close()
	}()

	// Set up timeout for unlogged connections
	timeoutCtx, cancel := context.WithTimeout(conn.ctx, cm.connectTimeout)
	defer cancel()

	// Send initial welcome banner by enqueuing empty string to scheduler
	// This matches ToastStunt behavior: new_input_task(h->tasks, "", 0, 0)
	{
		done := make(chan struct{})
		cm.server.scheduler.EnqueueInput(InputEvent{
			ConnID: conn.ID,
			Player: types.ObjID(-conn.ID),
			Line:   "",
			Done:   done,
		})
		<-done
	}

	// I/O loop: read lines, enqueue events, wait for processing
	for {
		select {
		case <-conn.ctx.Done():
			return
		case <-timeoutCtx.Done():
			if !conn.IsLoggedIn() {
				conn.Send("Connection timeout")
				return
			}
		default:
		}

		line, err := conn.ReadLine()
		if err != nil {
			log.Printf("Connection %d read error: %v", conn.ID, err)
			return
		}

		// Cancel the login timeout once logged in
		if conn.IsLoggedIn() {
			cancel()
		}

		done := make(chan struct{})
		cm.server.scheduler.EnqueueInput(InputEvent{
			ConnID: conn.ID,
			Player: conn.GetPlayer(),
			Line:   line,
			Done:   done,
		})
		<-done
	}
}

// listContainsString checks if a MOO list contains a string value.
func listContainsString(value types.Value, target string) bool {
	list, ok := value.(types.ListValue)
	if !ok {
		return false
	}

	for i := 1; i <= list.Len(); i++ {
		s, ok := list.Get(i).(types.StrValue)
		if ok && s.Value() == target {
			return true
		}
	}
	return false
}

// getConnectionByConnID returns a Connection by its connection ID (not player ID).
func (cm *ConnectionManager) getConnectionByConnID(connID int64) *Connection {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.connections[connID]
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

// ConnectedPlayers returns list of connected player ObjIDs.
// When showAll is false (default), only connections that have completed login
// (non-zero ConnectionTime) are included, matching Toast's semantics.
// When showAll is true, all connections including unlogged ones are returned.
func (cm *ConnectionManager) ConnectedPlayers(showAll bool) []types.ObjID {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	players := make([]types.ObjID, 0, len(cm.playerConns))
	for player, conn := range cm.playerConns {
		if !showAll && conn.ConnectionTime.IsZero() {
			continue
		}
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
