package builtins

import (
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
	fmt.Printf("[NOTIFY DEBUG] called with %d args\n", len(args))
	if globalConnManager == nil {
		fmt.Printf("[NOTIFY DEBUG] no connection manager\n")
		return types.Err(types.E_INVARG)
	}
	// Get player
	playerVal, ok := args[0].(types.ObjValue)
	if !ok {
		fmt.Printf("[NOTIFY DEBUG] player arg not ObjValue: %T\n", args[0])
		return types.Err(types.E_TYPE)
	}
	player := playerVal.ID()
	fmt.Printf("[NOTIFY DEBUG] player=%d\n", player)

	// Get message
	messageVal, ok := args[1].(types.StrValue)
	if !ok {
		fmt.Printf("[NOTIFY DEBUG] message arg not StrValue: %T\n", args[1])
		return types.Err(types.E_TYPE)
	}
	message := messageVal.Value()
	fmt.Printf("[NOTIFY DEBUG] message=%q (len=%d)\n", message, len(message))

	// Get no_flush (optional)
	noFlush := false
	if len(args) > 2 {
		noFlush = args[2].Truthy()
	}

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		fmt.Printf("[NOTIFY DEBUG] no connection for player %d\n", player)
		// Player not connected - fail silently (MOO behavior)
		return types.Ok(types.NewInt(0))
	}
	fmt.Printf("[NOTIFY DEBUG] got connection for player %d\n", player)

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
func builtinSwitchPlayer(ctx *types.TaskContext, args []types.Value) types.Result {
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
	fmt.Printf("[SWITCH] switch_player(%d, %d) called\n", oldPlayer, newPlayer)
	if err := globalConnManager.SwitchPlayer(oldPlayer, newPlayer); err != nil {
		fmt.Printf("[SWITCH] switch_player error: %v\n", err)
		return types.Err(types.E_INVARG)
	}
	fmt.Printf("[SWITCH] switch_player succeeded\n")

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
