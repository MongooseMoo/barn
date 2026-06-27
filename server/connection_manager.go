package server

import (
	"barn/builtins"
	"barn/types"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type listenerRecord struct {
	listener      net.Listener
	object        types.ObjID
	port          int64
	protocol      string
	path          string
	printMessages bool
	ipv6          bool
	iface         string
	tls           bool
	tlsConfig     *tls.Config
	httpServer    *http.Server
	primary       bool
	accepting     bool
	done          chan struct{}
}

type listenerKey struct {
	protocol string
	port     int64
	path     string
}

// ConnectionManager manages all active connections
type ConnectionManager struct {
	connections       map[int64]*Connection
	playerConns       map[types.ObjID]*Connection // Map player to active connection
	playerConnHistory map[types.ObjID][]*Connection
	nextConnID        int64
	mu                sync.Mutex
	connectionWG      sync.WaitGroup
	connectionSetupWG sync.WaitGroup
	connectionHandler func(*Connection)
	listeners         map[listenerKey]*listenerRecord
	setupConns        map[net.Conn]struct{}
	outboundClients   map[int64]net.Conn
	closing           bool
	listenPort        int
	connectTimeout    time.Duration
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(port int) *ConnectionManager {
	return &ConnectionManager{
		connections:       make(map[int64]*Connection),
		playerConns:       make(map[types.ObjID]*Connection),
		playerConnHistory: make(map[types.ObjID][]*Connection),
		listeners:         make(map[listenerKey]*listenerRecord),
		setupConns:        make(map[net.Conn]struct{}),
		outboundClients:   make(map[int64]net.Conn),
		nextConnID:        2, // Start at 2 so first connection is -2 (not -1 which is NOTHING)
		listenPort:        port,
		connectTimeout:    5 * time.Minute,
	}
}

func (cm *ConnectionManager) setConnectionHandler(handler func(*Connection)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connectionHandler = handler
}

// GetListenPort returns the port the server is listening on
func (cm *ConnectionManager) GetListenPort() int {
	return cm.listenPort
}

// BindListeners creates startup-owned listener sockets without accepting
// connections yet.
func (cm *ConnectionManager) BindListeners(specs []builtins.ListenerSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("no listeners configured")
	}

	originalPort := cm.listenPort
	bound := make([]builtins.ListenerDescriptor, 0, len(specs))
	for i, spec := range specs {
		desc, err := cm.addListener(spec, true)
		if err != nil {
			for _, boundDesc := range bound {
				key := listenerKeyFromDescriptor(boundDesc)
				cm.mu.Lock()
				record := cm.listeners[key]
				if record != nil {
					delete(cm.listeners, key)
				}
				cm.mu.Unlock()
				if record != nil {
					if record.httpServer != nil {
						_ = record.httpServer.Close()
					} else {
						_ = record.listener.Close()
					}
				}
			}
			cm.listenPort = originalPort
			return err
		}
		bound = append(bound, desc)
		if i == 0 {
			cm.listenPort = int(desc.Port)
		}
	}
	return nil
}

// StartAccepting begins accept loops for all bound listeners that are not
// already accepting.
func (cm *ConnectionManager) StartAccepting() {
	cm.mu.Lock()
	records := make([]*listenerRecord, 0, len(cm.listeners))
	for _, record := range cm.listeners {
		if record.accepting {
			continue
		}
		record.accepting = true
		records = append(records, record)
	}
	cm.mu.Unlock()

	for _, record := range records {
		go func(record *listenerRecord) {
			defer close(record.done)
			if record.httpServer != nil {
				cm.serveWebSocketListener(record)
				return
			}
			cm.acceptConnections(record)
		}(record)
	}
}

// CloseListeners closes every bound listener, including startup-owned primary
// listeners. It is the shutdown counterpart to RemoveListener, which preserves
// the MOO-level rule that primary listeners cannot be removed at runtime.
func (cm *ConnectionManager) CloseListeners() {
	cm.mu.Lock()
	type listenerToClose struct {
		record *listenerRecord
		wait   bool
	}
	records := make([]listenerToClose, 0, len(cm.listeners))
	for key, record := range cm.listeners {
		records = append(records, listenerToClose{
			record: record,
			wait:   record.accepting,
		})
		delete(cm.listeners, key)
	}
	cm.mu.Unlock()

	for _, item := range records {
		if item.record.httpServer != nil {
			_ = item.record.httpServer.Close()
			continue
		}
		_ = item.record.listener.Close()
	}
	for _, item := range records {
		if item.wait {
			<-item.record.done
		}
	}
}

// CloseConnections sends the shutdown banner to every active connection and
// closes its transport. Normal disconnect processing remains responsible for
// removing connections from the manager maps.
func (cm *ConnectionManager) CloseConnections(message string) {
	cm.mu.Lock()
	cm.closing = true
	setupConns := make([]net.Conn, 0, len(cm.setupConns))
	for conn := range cm.setupConns {
		setupConns = append(setupConns, conn)
	}
	cm.mu.Unlock()

	for _, conn := range setupConns {
		_ = conn.Close()
	}
	cm.connectionSetupWG.Wait()

	cm.mu.Lock()
	connections := make([]*Connection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		connections = append(connections, conn)
	}
	cm.mu.Unlock()

	line := fmt.Sprintf("*** Shutting down: %s ***", message)
	for _, conn := range connections {
		_ = conn.Send(line)
		_ = conn.Close()
	}
	cm.connectionWG.Wait()
}

func (cm *ConnectionManager) registerListener(listener net.Listener, spec builtins.ListenerSpec, primary bool, tlsConfig *tls.Config) (builtins.ListenerDescriptor, error) {
	port, ipv6, err := parseListenerPort(listener.Addr())
	if err != nil {
		_ = listener.Close()
		return builtins.ListenerDescriptor{}, err
	}

	spec.Protocol = normalizeListenerProtocol(spec.Protocol)
	desc := builtins.ListenerDescriptor{
		Protocol: spec.Protocol,
		Port:     port,
		Path:     canonicalListenerPath(spec.Protocol, spec.Path),
	}
	key := listenerKeyFromDescriptor(desc)
	record := &listenerRecord{
		listener:      listener,
		object:        spec.Object,
		port:          port,
		protocol:      desc.Protocol,
		path:          desc.Path,
		printMessages: spec.PrintMessages,
		ipv6:          ipv6,
		iface:         spec.Interface,
		tls:           spec.Protocol == builtins.ListenerProtocolTLS || spec.Protocol == builtins.ListenerProtocolSecureWebSocket,
		tlsConfig:     tlsConfig,
		primary:       primary,
		done:          make(chan struct{}),
	}
	if desc.Protocol == builtins.ListenerProtocolWebSocket || desc.Protocol == builtins.ListenerProtocolSecureWebSocket {
		record.httpServer = &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cm.handleWebSocketRequest(record, w, r)
			}),
		}
	}

	cm.mu.Lock()
	if _, exists := cm.listeners[key]; exists {
		cm.mu.Unlock()
		_ = listener.Close()
		return builtins.ListenerDescriptor{}, fmt.Errorf("listener already exists for %s", formatListenerDescriptor(desc))
	}
	cm.listeners[key] = record
	cm.mu.Unlock()

	if primary {
		log.Printf("Listening on port %d", port)
	} else {
		log.Printf("Added %s listener on port %d for #%d", spec.Protocol, port, spec.Object)
	}

	return desc, nil
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

		cm.mu.Lock()
		if cm.closing {
			cm.mu.Unlock()
			_ = socket.Close()
			continue
		}
		cm.connectionSetupWG.Add(1)
		cm.setupConns[socket] = struct{}{}
		cm.mu.Unlock()

		go func() {
			defer cm.connectionSetupWG.Done()
			defer func() {
				cm.mu.Lock()
				delete(cm.setupConns, socket)
				cm.mu.Unlock()
			}()
			cm.handleNewConnection(record, socket)
		}()
	}
}

// handleNewConnection handles a new TCP connection
func (cm *ConnectionManager) handleNewConnection(record *listenerRecord, socket net.Conn) {
	if record.tlsConfig != nil {
		_ = socket.SetDeadline(time.Now().Add(cm.connectTimeout))
		tlsSocket := tls.Server(socket, record.tlsConfig)
		if err := tlsSocket.Handshake(); err != nil {
			log.Printf("TLS handshake error from %s: %v", socket.RemoteAddr(), err)
			_ = socket.Close()
			return
		}
		_ = tlsSocket.SetDeadline(time.Time{})
		socket = tlsSocket
	}

	transport := NewTCPTransport(socket)
	conn := cm.NewConnectionFromTransport(transport)
	conn.SetListener(record.object, record.port, record.printMessages)

	log.Printf("New connection from %s (ID: %d)", conn.RemoteAddr(), conn.ID)

	cm.connectionWG.Add(1)
	go cm.handleConnection(conn)
}

func (cm *ConnectionManager) serveWebSocketListener(record *listenerRecord) {
	err := record.httpServer.Serve(record.listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		log.Printf("WebSocket listener error: %v", err)
	}
}

func (cm *ConnectionManager) handleWebSocketRequest(record *listenerRecord, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != record.path {
		http.NotFound(w, r)
		return
	}
	if !isWebSocketUpgrade(r) {
		w.Header().Set("Upgrade", "websocket")
		http.Error(w, "WebSocket upgrade required", http.StatusUpgradeRequired)
		return
	}

	cm.mu.Lock()
	if cm.closing {
		cm.mu.Unlock()
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	}
	cm.connectionSetupWG.Add(1)
	cm.mu.Unlock()
	defer cm.connectionSetupWG.Done()

	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("WebSocket upgrade error from %s: %v", r.RemoteAddr, err)
		return
	}
	wsConn.SetReadLimit(1 << 20)

	transport := NewWebSocketTransport(wsConn, r.RemoteAddr)
	conn := cm.NewConnectionFromTransport(transport)
	conn.SetListener(record.object, record.port, record.printMessages)

	log.Printf("New %s connection from %s (ID: %d)", record.protocol, conn.RemoteAddr(), conn.ID)

	cm.connectionWG.Add(1)
	go cm.handleConnection(conn)
}

func (cm *ConnectionManager) handleConnection(conn *Connection) {
	defer cm.connectionWG.Done()

	cm.mu.Lock()
	handler := cm.connectionHandler
	cm.mu.Unlock()
	if handler == nil {
		_ = conn.Close()
		return
	}
	handler(conn)
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

	conn.Send("*** Disconnected ***")
	conn.Close()
	return nil
}

func (cm *ConnectionManager) RecyclePlayer(player types.ObjID) error {
	cm.mu.Lock()
	conn := cm.playerConns[player]
	cm.mu.Unlock()

	if conn == nil {
		return nil
	}

	conn.Send("*** Recycled ***")
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
			Protocol:      record.protocol,
			Path:          record.path,
			PrintMessages: record.printMessages,
			IPv6:          record.ipv6,
			Interface:     record.iface,
			TLS:           record.tls,
		})
	}
	return out
}

func (cm *ConnectionManager) AddListener(spec builtins.ListenerSpec) (builtins.ListenerDescriptor, error) {
	desc, err := cm.addListener(spec, false)
	if err != nil {
		return builtins.ListenerDescriptor{}, err
	}
	cm.StartAccepting()
	return desc, nil
}

func (cm *ConnectionManager) addListener(spec builtins.ListenerSpec, primary bool) (builtins.ListenerDescriptor, error) {
	spec.Protocol = normalizeListenerProtocol(spec.Protocol)
	if spec.Protocol != builtins.ListenerProtocolTCP && spec.Protocol != builtins.ListenerProtocolTLS && spec.Protocol != builtins.ListenerProtocolWebSocket {
		return builtins.ListenerDescriptor{}, fmt.Errorf("unsupported listener protocol %q", spec.Protocol)
	}
	if spec.Protocol == builtins.ListenerProtocolWebSocket {
		if spec.TLSCertificatePath != "" || spec.TLSKeyPath != "" {
			return builtins.ListenerDescriptor{}, fmt.Errorf("ws listener does not accept TLS options")
		}
		if spec.Path == "" {
			spec.Path = "/"
		}
		if !strings.HasPrefix(spec.Path, "/") {
			return builtins.ListenerDescriptor{}, fmt.Errorf("websocket listener path must start with /")
		}
		return cm.listenAndRegister(spec, primary, nil)
	}
	if spec.Path != "" {
		return builtins.ListenerDescriptor{}, fmt.Errorf("%s listener does not accept path", spec.Protocol)
	}
	if spec.Protocol == builtins.ListenerProtocolTLS {
		if spec.TLSCertificatePath == "" || spec.TLSKeyPath == "" {
			return builtins.ListenerDescriptor{}, fmt.Errorf("tls listener requires certificate and key")
		}
		cert, err := tls.LoadX509KeyPair(spec.TLSCertificatePath, spec.TLSKeyPath)
		if err != nil {
			return builtins.ListenerDescriptor{}, err
		}
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
		return cm.listenAndRegister(spec, primary, tlsConfig)
	}

	return cm.listenAndRegister(spec, primary, nil)
}

func (cm *ConnectionManager) listenAndRegister(spec builtins.ListenerSpec, primary bool, tlsConfig *tls.Config) (builtins.ListenerDescriptor, error) {
	addr := net.JoinHostPort(spec.Interface, fmt.Sprintf("%d", spec.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return builtins.ListenerDescriptor{}, err
	}
	return cm.registerListener(listener, spec, primary, tlsConfig)
}

func (cm *ConnectionManager) RemoveListener(desc builtins.ListenerDescriptor) error {
	key := listenerKeyFromDescriptor(desc)
	cm.mu.Lock()
	record := cm.listeners[key]
	if record == nil {
		cm.mu.Unlock()
		return fmt.Errorf("listener not found")
	}
	if record.primary {
		cm.mu.Unlock()
		return fmt.Errorf("cannot remove primary listener")
	}
	delete(cm.listeners, key)
	wait := record.accepting
	cm.mu.Unlock()

	var err error
	if record.httpServer != nil {
		err = record.httpServer.Close()
	} else {
		err = record.listener.Close()
	}
	if wait {
		<-record.done
	}
	return err
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

	// SplitHostPort fails when RemoteAddr() carries no parseable host:port; that
	// failure is signalled by the empty host, in which case we fall back to the
	// raw address (the deterministic numeric value, matching Toast's behavior).
	host, _, _ := net.SplitHostPort(conn.RemoteAddr())
	host = strings.Trim(host, "[]")
	if host == "" {
		host = conn.RemoteAddr()
	}

	// Reverse-DNS lookup. On failure, deterministically fall back to the numeric
	// host. Toast does the same (network.cc:985 get_nameinfo -> get_ntop numeric
	// fallback; network.cc:1593 keeps the existing name on getaddrinfo failure)
	// and never surfaces a lookup error to MOO — so we keep the (string, error)
	// error channel reserved for "connection not found" above.
	resolved := host
	names, lookupErr := net.LookupAddr(host)
	lookupOK := lookupErr == nil && len(names) > 0
	if lookupOK {
		resolved = strings.TrimSuffix(names[0], ".")
	}

	// Only overwrite the cached connection name when the lookup actually
	// succeeded. Toast gates the rewrite on the lookup status (server.cc:2980,
	// `rewrite_connect_name && status == 0`); previously Barn ignored lookupErr
	// and rewrote the name to the numeric fallback on every failure, corrupting
	// the value returned by connection_name(). The lookup error now drives that
	// decision instead of being silently discarded.
	if rewrite && lookupOK {
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

func normalizeListenerProtocol(protocol string) string {
	if protocol == "" {
		return builtins.ListenerProtocolTCP
	}
	return strings.ToLower(protocol)
}

func canonicalListenerPath(protocol, path string) string {
	switch protocol {
	case builtins.ListenerProtocolWebSocket, builtins.ListenerProtocolSecureWebSocket:
		if path == "" {
			return "/"
		}
		return path
	default:
		return ""
	}
}

func listenerKeyFromDescriptor(desc builtins.ListenerDescriptor) listenerKey {
	protocol := normalizeListenerProtocol(desc.Protocol)
	return listenerKey{
		protocol: protocol,
		port:     desc.Port,
		path:     canonicalListenerPath(protocol, desc.Path),
	}
}

func formatListenerDescriptor(desc builtins.ListenerDescriptor) string {
	protocol := normalizeListenerProtocol(desc.Protocol)
	path := canonicalListenerPath(protocol, desc.Path)
	if path == "" {
		return fmt.Sprintf("%s://:%d", protocol, desc.Port)
	}
	return fmt.Sprintf("%s://:%d%s", protocol, desc.Port, path)
}

func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, part := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
			return true
		}
	}
	return false
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
