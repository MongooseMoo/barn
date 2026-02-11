package builtins

import (
	"barn/trace"
	"barn/types"
	"fmt"
)

// ConnectionManager interface to avoid import cycle
type ConnectionManager interface {
	GetConnection(player types.ObjID) Connection
	ConnectedPlayers() []types.ObjID
	BootPlayer(player types.ObjID) error
	SwitchPlayer(oldPlayer, newPlayer types.ObjID) error
}

// Connection interface to avoid import cycle
type Connection interface {
	Send(message string) error
	Buffer(message string)
	Flush() error
	RemoteAddr() string
}

// Global connection manager (set by server)
var globalConnManager ConnectionManager

// SetConnectionManager sets the global connection manager
func SetConnectionManager(cm ConnectionManager) {
	globalConnManager = cm
}

// notify(player, message [, no_flush]) -> none
func builtinNotify(ctx *types.TaskContext, args []types.Value) types.Result {
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()

	// Get message
	messageVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	message := messageVal.Value()

	// Trace notify call
	trace.Notify(player, message)

	// Get no_flush (optional)
	noFlush := false
	if len(args) > 2 {
		noFlush = args[2].Truthy()
	}

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		// Player not connected - fail silently (MOO behavior)
		return types.Ok(types.NewInt(0))
	}

	// Send message
	if noFlush {
		conn.Buffer(message)
	} else {
		if err := conn.Send(message); err != nil {
			return types.Err(types.E_INVARG)
		}
	}

	return types.Ok(types.NewInt(0))
}

// listeners([find]) -> list of maps
// Returns list of listening ports as maps with keys:
//   "object" (OBJ), "port" (ANY), "print-messages" (INT),
//   "ipv6" (INT), "interface" (STR)
// Optional argument filters by object ID (if OBJ) or port descriptor (if other type)
//
// ToastStunt implementation: iterates over all_slisteners linked list,
// returns map for each listener. When no listeners exist, returns empty list.
//
// Since Barn doesn't implement listen() builtin yet, we always return empty list.
// When listen() is implemented, this should query the server's listener registry.
func builtinListeners(ctx *types.TaskContext, args []types.Value) types.Result {
	// Optional filter argument (object ID or port descriptor)
	var findObj types.ObjID = -1
	var findPort types.Value = nil

	if len(args) > 0 {
		if objVal, ok := args[0].(types.ObjValue); ok {
			findObj = objVal.ID()
		} else {
			findPort = args[0]
		}
	}

	// TODO: When listen() builtin is implemented, query actual listeners here
	// For now, return empty list since no listeners can be registered
	// This prevents MCP code from running during login (it checks for listeners on #0)

	_ = findObj  // Suppress unused warnings
	_ = findPort

	return types.Ok(types.NewList([]types.Value{}))
}

// connected_players([include_queued]) -> list
func builtinConnectedPlayers(ctx *types.TaskContext, args []types.Value) types.Result {
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	// Get list of connected players
	players := globalConnManager.ConnectedPlayers()

	// Convert to list of ObjValues
	elements := make([]types.Value, len(players))
	for i, player := range players {
		elements[i] = types.NewObj(player)
	}

	return types.Ok(types.NewList(elements))
}

// connection_name(player [, method]) -> str
// ToastStunt behavior:
// - No method or method=0: return hostname (or IP if no DNS lookup)
// - method=1: return numeric IP address
// - method=2: return legacy "IP, port XXXX" format
func builtinConnectionName(ctx *types.TaskContext, args []types.Value) types.Result {
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()

	// Get method (optional, default 0)
	method := int64(0)
	if len(args) > 1 {
		if methodVal, ok := args[1].(types.IntValue); ok {
			method = methodVal.Val
		}
	}

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	// Parse remote address to extract IP and port
	remoteAddr := conn.RemoteAddr()
	ip := remoteAddr
	port := "0"
	// remoteAddr format is typically "IP:port"
	if idx := len(remoteAddr) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if remoteAddr[i] == ':' {
				ip = remoteAddr[:i]
				port = remoteAddr[i+1:]
				break
			}
		}
	}

	// Get connection info based on method
	var name string
	switch method {
	case 0:
		// Default: return just the IP (or hostname if DNS lookup was done)
		name = ip
	case 1:
		// Return numeric IP address
		name = ip
	case 2:
		// Return legacy "IP, port XXXX" format
		name = fmt.Sprintf("%s, port %s", ip, port)
	default:
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewStr(name))
}

// boot_player(player) -> none
func builtinBootPlayer(ctx *types.TaskContext, args []types.Value) types.Result {
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()

	// Boot player
	if err := globalConnManager.BootPlayer(player); err != nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewInt(0))
}

// switch_player(old_player, new_player) -> none
// Associates the connection for old_player with new_player
// Used during login to switch from negative connection ID to player object
// Requires wizard permissions
func builtinSwitchPlayer(ctx *types.TaskContext, args []types.Value) types.Result {
	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	if len(args) < 2 {
		return types.Err(types.E_ARGS)
	}

	// Get old player
	oldPlayerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	oldPlayer := oldPlayerVal.ID()

	// Get new player
	newPlayerVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	newPlayer := newPlayerVal.ID()

	// Switch player
	if err := globalConnManager.SwitchPlayer(oldPlayer, newPlayer); err != nil {
		fmt.Printf("[SWITCH_PLAYER DEBUG] SwitchPlayer failed: old=#%d new=#%d error=%v\n", oldPlayer, newPlayer, err)
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewInt(0))
}

// idle_seconds(player) -> int
func builtinIdleSeconds(ctx *types.TaskContext, args []types.Value) types.Result {
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	// Calculate idle time (placeholder - need to track last input time)
	idleSeconds := 0

	return types.Ok(types.NewInt(int64(idleSeconds)))
}

// connected_seconds(player) -> int
func builtinConnectedSeconds(ctx *types.TaskContext, args []types.Value) types.Result {
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	// Calculate connected time (placeholder - need to track connection time)
	connectedSeconds := 0

	return types.Ok(types.NewInt(int64(connectedSeconds)))
}

// connection_info(player) -> MAP
// Returns detailed connection information
func builtinConnectionInfo(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		fmt.Printf("[CONNECTION_INFO DEBUG] No connection for player #%d\n", player)
		return types.Err(types.E_INVARG)
	}

	// Parse remote address to extract IP and port
	remoteAddr := conn.RemoteAddr()
	destIP := remoteAddr
	destPort := int64(0)
	// remoteAddr format is typically "IP:port" or "[IPv6]:port"
	if idx := len(remoteAddr) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if remoteAddr[i] == ':' {
				destIP = remoteAddr[:i]
				if p, err := fmt.Sscanf(remoteAddr[i+1:], "%d", &destPort); err != nil || p != 1 {
					destPort = 0
				}
				break
			}
		}
	}
	// Strip brackets from IPv6 addresses
	if len(destIP) > 2 && destIP[0] == '[' && destIP[len(destIP)-1] == ']' {
		destIP = destIP[1 : len(destIP)-1]
	}

	// Build result map with connection info matching Toast's format:
	// source_* = server side, destination_* = client side
	// For incoming connections, source is the server's listening port
	result := types.NewMap([][2]types.Value{
		{types.NewStr("source_address"), types.NewStr("localhost")}, // Server hostname
		{types.NewStr("source_ip"), types.NewStr("127.0.0.1")},      // Server IP
		{types.NewStr("source_port"), types.NewInt(9450)},           // TODO: Get actual listening port
		{types.NewStr("destination_address"), types.NewStr(destIP)}, // Client hostname
		{types.NewStr("destination_ip"), types.NewStr(destIP)},      // Client IP
		{types.NewStr("destination_port"), types.NewInt(destPort)},  // Client port
		{types.NewStr("protocol"), types.NewStr("IPv4")},            // TODO: Detect actual protocol
		{types.NewStr("outbound"), types.NewInt(0)},                 // 0 = incoming connection
	})

	return types.Ok(result)
}

// connection_name_lookup(player) -> 0
// ToastStunt builtin to start async DNS lookup for a connection
// We stub this to return 0 (success) immediately since we don't need async DNS
func builtinConnectionNameLookup(ctx *types.TaskContext, args []types.Value) types.Result {
	// Stub: just return 0 (success) - no async DNS lookup needed
	return types.Ok(types.NewInt(0))
}

// set_connection_option(conn, option, value) -> none
// Sets I/O options for a connection (hold-input, disable-oob, etc.)
func builtinSetConnectionOption(ctx *types.TaskContext, args []types.Value) types.Result {
	// Stub: accept and ignore connection options for now
	// TODO: implement hold-input, disable-oob, client-echo, etc.
	return types.Ok(types.NewInt(0))
}

// connection_option(conn, option) -> value
// Gets I/O options for a connection
func builtinConnectionOption(ctx *types.TaskContext, args []types.Value) types.Result {
	// Stub: return default values
	// TODO: implement actual option retrieval
	return types.Ok(types.NewInt(0))
}

// read_http([type [, connection]]) -> map | E_PERM | E_ARGS | E_TYPE | E_INVARG
// Reads HTTP request or response data from a connection
// - type: "request" or "response" (required)
// - connection: object ID of connection (optional, defaults to caller's connection)
//
// ToastStunt behavior:
// - No args: requires wizard (for backward compat - should pass type)
// - type arg: must be string "request" or "response", else E_INVARG
// - connection arg: must be object, else E_TYPE
// - With connection arg: requires wizard or owner of connection object
// - Without connection arg: requires wizard and active task from that connection
// - Suspends task to parse HTTP data from connection buffer
func builtinReadHTTP(ctx *types.TaskContext, args []types.Value) types.Result {
	// Validate we have at least one argument (type)
	if len(args) == 0 {
		return types.Err(types.E_ARGS)
	}

	// First argument must be a string (type)
	typeVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	typeStr := typeVal.Value()

	// Validate type is "request" or "response"
	if typeStr != "request" && typeStr != "response" {
		return types.Err(types.E_INVARG)
	}

	// Second argument (if provided) must be an object (connection)
	var connection types.ObjID = ctx.Player
	if len(args) > 1 {
		connVal, ok := args[1].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		connection = connVal.ID()
	}

	// Permission checks (from ToastStunt bf_read_http)
	if len(args) > 1 {
		// With explicit connection: require wizard or owner of connection
		if !ctx.IsWizard {
			// Check if programmer owns the connection object
			// TODO: implement db_object_owner check when we have DB access
			// For now, require wizard for explicit connection
			return types.Err(types.E_PERM)
		}
	} else {
		// Without explicit connection: require wizard
		if !ctx.IsWizard {
			return types.Err(types.E_PERM)
		}
		// TODO: Also check that last_input_task_id(connection) == current_task_id
		// This prevents reading from connections that aren't actively inputting
	}

	// At this point in ToastStunt, the function would:
	// 1. Create a suspended task that waits for HTTP data
	// 2. Return a suspend package with make_parsing_http_request_task or
	//    make_parsing_http_response_task
	// 3. When data arrives, parse it and resume with the parsed map
	//
	// Since Barn doesn't have HTTP connection infrastructure yet:
	// - We validate all arguments correctly (tests verify this)
	// - We would need to implement connection buffering, force_input(),
	//   and HTTP parsing to make this fully functional
	// - For now, return E_INVARG to indicate "no HTTP data available"
	//   (this matches the behavior when a connection has no buffered data)

	_ = connection // Use the connection variable

	// TODO: Implement HTTP parsing and task suspension
	// For now, return E_INVARG (no data available)
	return types.Err(types.E_INVARG)
}
