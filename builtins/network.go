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

	// Get method (optional)
	method := "legacy"
	if len(args) > 1 {
		if methodVal, ok := args[1].(types.StrValue); ok {
			method = methodVal.Value()
		}
	}

	// Get connection
	conn := globalConnManager.GetConnection(player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	// Get connection info
	var name string
	switch method {
	case "legacy":
		name = fmt.Sprintf("%s, port %d", "127.0.0.1", 7777) // Placeholder
	case "ip-address":
		name = "127.0.0.1" // Placeholder
	case "hostname":
		name = "localhost" // Placeholder
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
