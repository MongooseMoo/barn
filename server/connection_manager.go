package server

import (
	"barn/builtins"
	"barn/trace"
	"barn/types"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

type listenerRecord struct {
	listener      net.Listener
	object        types.ObjID
	port          int64
	printMessages bool
	ipv6          bool
	iface         string
	primary       bool
}

// ConnectionManager manages all active connections
type ConnectionManager struct {
	connections       map[int64]*Connection
	playerConns       map[types.ObjID]*Connection // Map player to active connection
	playerConnHistory map[types.ObjID][]*Connection
	nextConnID        int64
	mu                sync.Mutex
	server            *Server
	listeners         map[int64]*listenerRecord
	outboundClients   map[int64]net.Conn
	listenPort        int
	connectTimeout    time.Duration
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(server *Server, port int) *ConnectionManager {
	return &ConnectionManager{
		connections:       make(map[int64]*Connection),
		playerConns:       make(map[types.ObjID]*Connection),
		playerConnHistory: make(map[types.ObjID][]*Connection),
		listeners:         make(map[int64]*listenerRecord),
		outboundClients:   make(map[int64]net.Conn),
		nextConnID:        2, // Start at 2 so first connection is -2 (not -1 which is NOTHING)
		server:            server,
		listenPort:        port,
		connectTimeout:    5 * time.Minute,
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

	return cm.registerListener(listener, 0, false, true)
}

func (cm *ConnectionManager) registerListener(listener net.Listener, object types.ObjID, printMessages bool, primary bool) error {
	port, ipv6, err := parseListenerPort(listener.Addr())
	if err != nil {
		_ = listener.Close()
		return err
	}

	record := &listenerRecord{
		listener:      listener,
		object:        object,
		port:          port,
		printMessages: printMessages,
		ipv6:          ipv6,
		primary:       primary,
	}

	cm.mu.Lock()
	if _, exists := cm.listeners[port]; exists {
		cm.mu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("listener already exists on port %d", port)
	}
	cm.listeners[port] = record
	cm.mu.Unlock()

	if primary {
		log.Printf("Listening on port %d", port)
	} else {
		log.Printf("Added listener on port %d for #%d", port, object)
	}

	go cm.acceptConnections(record)
	return nil
}

// acceptConnections accepts incoming connections
func (cm *ConnectionManager) acceptConnections(record *listenerRecord) {
	for {
		socket, err := record.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("Accept error: %v", err)
			continue
		}

		cm.handleNewConnection(record, socket)
	}
}

// handleNewConnection handles a new TCP connection
func (cm *ConnectionManager) handleNewConnection(record *listenerRecord, socket net.Conn) {
	transport := NewTCPTransport(socket)
	conn := cm.NewConnectionFromTransport(transport)
	conn.SetListener(record.object, record.printMessages)

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
		player := conn.GetPlayer()
		if !conn.IsLoggedIn() {
			player = types.ObjID(-conn.ID)
		}
		cm.server.scheduler.EnqueueInput(InputEvent{
			ConnID: conn.ID,
			Player: player,
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

func (cm *ConnectionManager) pushPlayerHistoryLocked(player types.ObjID, conn *Connection) {
	if conn == nil {
		return
	}
	history := cm.playerConnHistory[player]
	if len(history) > 0 && history[len(history)-1] == conn {
		return
	}
	cm.playerConnHistory[player] = append(history, conn)
}

func (cm *ConnectionManager) removePlayerHistoryConnLocked(player types.ObjID, target *Connection) {
	history := cm.playerConnHistory[player]
	if len(history) == 0 {
		return
	}

	kept := history[:0]
	for _, conn := range history {
		if conn != nil && conn != target {
			kept = append(kept, conn)
		}
	}

	if len(kept) == 0 {
		delete(cm.playerConnHistory, player)
		return
	}
	cm.playerConnHistory[player] = kept
}

func (cm *ConnectionManager) restorePreviousPlayerConnLocked(player types.ObjID, closing *Connection) *Connection {
	history := cm.playerConnHistory[player]
	for len(history) > 0 {
		candidate := history[len(history)-1]
		history = history[:len(history)-1]
		if candidate == nil || candidate == closing {
			continue
		}
		if cm.connections[candidate.ID] != candidate {
			continue
		}
		if !candidate.IsLoggedIn() || candidate.GetPlayer() != player {
			continue
		}
		cm.playerConns[player] = candidate
		if len(history) == 0 {
			delete(cm.playerConnHistory, player)
		} else {
			cm.playerConnHistory[player] = history
		}
		return candidate
	}

	delete(cm.playerConnHistory, player)
	return nil
}

// ConnectedPlayers returns list of connected player ObjIDs.
// When showAll is false (default), only connections that have completed login
// (non-zero ConnectionTime) are included, matching Toast's semantics.
// When showAll is true, all connections including unlogged ones are returned.
func (cm *ConnectionManager) ConnectedPlayers(showAll bool) []types.ObjID {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	type connectedPlayer struct {
		player types.ObjID
		conn   *Connection
	}
	connected := make([]connectedPlayer, 0, len(cm.playerConns))
	for player, conn := range cm.playerConns {
		if !showAll && conn.ConnectionTime.IsZero() {
			continue
		}
		connected = append(connected, connectedPlayer{player: player, conn: conn})
	}

	sort.SliceStable(connected, func(i, j int) bool {
		left := connected[i].conn.ConnectionTime
		right := connected[j].conn.ConnectionTime
		if left.Equal(right) {
			return connected[i].player < connected[j].player
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.After(right)
	})

	players := make([]types.ObjID, 0, len(connected))
	for _, item := range connected {
		players = append(players, item.player)
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

func (cm *ConnectionManager) ListenerInfos() []builtins.ListenerInfo {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	out := make([]builtins.ListenerInfo, 0, len(cm.listeners))
	for _, record := range cm.listeners {
		out = append(out, builtins.ListenerInfo{
			Object:        record.object,
			Port:          record.port,
			PrintMessages: record.printMessages,
			IPv6:          record.ipv6,
			Interface:     record.iface,
		})
	}
	return out
}

func (cm *ConnectionManager) AddListener(object types.ObjID, port int64, printMessages bool) (int64, error) {
	addr := fmt.Sprintf(":%d", port)
	if port == 0 {
		addr = ":0"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, err
	}
	if err := cm.registerListener(listener, object, printMessages, false); err != nil {
		return 0, err
	}

	actualPort, _, err := parseListenerPort(listener.Addr())
	return actualPort, err
}

func (cm *ConnectionManager) RemoveListener(port int64) error {
	cm.mu.Lock()
	record := cm.listeners[port]
	if record == nil {
		cm.mu.Unlock()
		return fmt.Errorf("listener not found")
	}
	if record.primary {
		cm.mu.Unlock()
		return fmt.Errorf("cannot remove primary listener")
	}
	delete(cm.listeners, port)
	cm.mu.Unlock()

	return record.listener.Close()
}

func (cm *ConnectionManager) OpenNetworkConnection(host string, port int64) (types.ObjID, error) {
	existing := make(map[int64]struct{})
	cm.mu.Lock()
	for id := range cm.connections {
		existing[id] = struct{}{}
	}
	cm.mu.Unlock()

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return types.ObjNothing, err
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cm.mu.Lock()
		ids := make([]int64, 0, len(cm.connections))
		for id := range cm.connections {
			if _, seen := existing[id]; !seen {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		if len(ids) > 0 {
			connID := ids[0]
			cm.outboundClients[connID] = client
			cm.mu.Unlock()
			return types.ObjID(-connID), nil
		}
		cm.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	_ = client.Close()
	return types.ObjNothing, fmt.Errorf("timed out waiting for outbound connection to register")
}

func (cm *ConnectionManager) ConnectionNameLookup(player types.ObjID, rewrite bool) (string, error) {
	connIface := cm.GetConnection(player)
	conn, ok := connIface.(*Connection)
	if !ok || conn == nil {
		return "", fmt.Errorf("connection not found")
	}

	host, _, err := net.SplitHostPort(conn.RemoteAddr())
	host = strings.Trim(host, "[]")
	if host == "" {
		host = conn.RemoteAddr()
	}

	names, err := net.LookupAddr(host)
	resolved := host
	if err == nil && len(names) > 0 {
		resolved = strings.TrimSuffix(names[0], ".")
	}

	if rewrite {
		conn.SetResolvedName(resolved)
	}
	return resolved, nil
}

func (cm *ConnectionManager) detachOutboundClient(connID int64) {
	cm.mu.Lock()
	client := cm.outboundClients[connID]
	delete(cm.outboundClients, connID)
	cm.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}

func parseListenerPort(addr net.Addr) (int64, bool, error) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if ok {
		return int64(tcpAddr.Port), tcpAddr.IP.To4() == nil, nil
	}
	host, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, false, err
	}
	var port int64
	_, err = fmt.Sscanf(portText, "%d", &port)
	if err != nil {
		return 0, false, err
	}
	return port, strings.Contains(host, ":"), nil
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

	if existing := cm.playerConns[newPlayer]; existing != nil && existing != conn {
		cm.pushPlayerHistoryLocked(newPlayer, existing)
	}

	// Set up new player
	conn.SetPlayer(newPlayer)
	cm.playerConns[newPlayer] = conn

	log.Printf("Switched connection %d from player %d to %d", conn.ID, oldPlayer, newPlayer)
	return nil
}
