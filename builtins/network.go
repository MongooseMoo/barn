package builtins

import (
	"barn/trace"
	"barn/types"
	"fmt"
	"net"
	"strings"
	"sync"
)

// ConnectionManager interface to avoid import cycle.
type ConnectionManager interface {
	GetConnection(player types.ObjID) Connection
	ConnectedPlayers(showAll bool) []types.ObjID
	BootPlayer(player types.ObjID) error
	SwitchPlayer(oldPlayer, newPlayer types.ObjID) error
	GetListenPort() int
}

// Connection interface to avoid import cycle.
type Connection interface {
	Send(message string) error
	Buffer(message string)
	Flush() error
	RemoteAddr() string
	GetOutputPrefix() string
	GetOutputSuffix() string
	BufferedOutputLength() int
	ConnectedSeconds() int64
	IdleSeconds() int64
}

// Global connection manager (set by server).
var globalConnManager ConnectionManager

// SetConnectionManager sets the global connection manager.
func SetConnectionManager(cm ConnectionManager) {
	globalConnManager = cm
}

// InputForcer allows builtins to inject input lines into a player's stream.
// Implemented by the scheduler to avoid import cycles.
type InputForcer interface {
	ForceInput(player types.ObjID, line string, atFront bool)
}

// Global input forcer (set by server).
var globalInputForcer InputForcer

// SetInputForcer sets the global input forcer.
func SetInputForcer(f InputForcer) {
	globalInputForcer = f
}

var connectionOptionState = struct {
	mu       sync.RWMutex
	byPlayer map[types.ObjID]map[string]types.Value
}{
	byPlayer: make(map[types.ObjID]map[string]types.Value),
}

func parseConnectionTarget(v types.Value) (types.ObjID, bool) {
	switch t := v.(type) {
	case types.ObjValue:
		return t.ID(), true
	case types.IntValue:
		return types.ObjID(t.Val), true
	default:
		return types.ObjNothing, false
	}
}

func resolveConnection(ctx *types.TaskContext, player types.ObjID) Connection {
	if globalConnManager == nil {
		return nil
	}
	if conn := globalConnManager.GetConnection(player); conn != nil {
		return conn
	}
	// Compatibility fallback: when running top-level eval with mismatched locals,
	// resolving self should still find the active connection.
	if ctx != nil && player == ctx.Player {
		for _, p := range globalConnManager.ConnectedPlayers(true) {
			if conn := globalConnManager.GetConnection(p); conn != nil {
				return conn
			}
		}
	}
	return nil
}

func validConnectionOption(name string) bool {
	switch name {
	case "hold-input", "client-echo", "disable-oob",
		"binary", "flush-command", "keep-alive":
		return true
	default:
		return false
	}
}

func defaultConnectionOptions() map[string]types.Value {
	return map[string]types.Value{
		"hold-input":    types.NewInt(0),
		"client-echo":   types.NewInt(1),
		"disable-oob":   types.NewInt(0),
		"binary":        types.NewInt(0),
		"flush-command": types.NewStr(""),
		"keep-alive":    types.NewInt(0),
	}
}

func getConnectionOptions(player types.ObjID) map[string]types.Value {
	connectionOptionState.mu.RLock()
	existing, ok := connectionOptionState.byPlayer[player]
	connectionOptionState.mu.RUnlock()
	if ok {
		out := make(map[string]types.Value, len(existing))
		for k, v := range existing {
			out[k] = v
		}
		return out
	}
	return defaultConnectionOptions()
}

func setConnectionOption(player types.ObjID, name string, value types.Value) {
	connectionOptionState.mu.Lock()
	defer connectionOptionState.mu.Unlock()

	existing, ok := connectionOptionState.byPlayer[player]
	if !ok {
		existing = defaultConnectionOptions()
		connectionOptionState.byPlayer[player] = existing
	}
	existing[name] = value
}

func parseRemoteAddress(remoteAddr string) (string, string) {
	host, port, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return strings.Trim(host, "[]"), port
	}

	// Fallback for malformed/non-standard addresses.
	if idx := strings.LastIndex(remoteAddr, ":"); idx > 0 {
		return strings.Trim(remoteAddr[:idx], "[]"), remoteAddr[idx+1:]
	}
	return strings.Trim(remoteAddr, "[]"), "0"
}

// notify(player, message [, no_flush [, no_newline]]) -> int
func builtinNotify(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}

	messageVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	message := messageVal.Value()
	trace.Notify(player, message)

	noFlush := false
	if len(args) >= 3 {
		noFlush = args[2].Truthy()
	}

	conn := resolveConnection(ctx, player)
	if conn == nil {
		// MOO behavior: missing/disconnected target is a successful no-op.
		return types.Ok(types.NewInt(1))
	}

	if noFlush {
		conn.Buffer(message)
		return types.Ok(types.NewInt(0))
	}
	if err := conn.Send(message); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

// listeners([find]) -> list of listener maps.
func builtinListeners(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Ok(types.NewList([]types.Value{}))
	}

	port := int64(globalConnManager.GetListenPort())
	entry := types.NewMap([][2]types.Value{
		{types.NewStr("object"), types.NewObj(0)},
		{types.NewStr("port"), types.NewInt(port)},
		{types.NewStr("print-messages"), types.NewInt(0)},
		{types.NewStr("ipv6"), types.NewInt(0)},
		{types.NewStr("interface"), types.NewStr("")},
	})

	if len(args) == 1 {
		if obj, ok := args[0].(types.ObjValue); ok {
			if obj.ID() != 0 {
				return types.Ok(types.NewList([]types.Value{}))
			}
		} else if p, ok := args[0].(types.IntValue); ok {
			if p.Val != port {
				return types.Ok(types.NewList([]types.Value{}))
			}
		}
	}

	return types.Ok(types.NewList([]types.Value{entry}))
}

// connected_players([show_all]) -> list.
func builtinConnectedPlayers(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	showAll := false
	if len(args) == 1 {
		showAll = args[0].Truthy()
	}

	players := make([]types.ObjID, 0, 8)
	seen := make(map[types.ObjID]struct{}, 8)
	if ctx != nil && ctx.Player > 0 {
		seen[ctx.Player] = struct{}{}
		players = append(players, ctx.Player)
	}
	for _, p := range globalConnManager.ConnectedPlayers(showAll) {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		players = append(players, p)
	}

	elements := make([]types.Value, 0, len(players))
	for _, player := range players {
		elements = append(elements, types.NewObj(player))
	}
	return types.Ok(types.NewList(elements))
}

// connection_name(player [, method]) -> str.
func builtinConnectionName(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}

	method := int64(0)
	if len(args) == 2 {
		m, ok := args[1].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		method = m.Val
	}

	conn := resolveConnection(ctx, player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	host, port := parseRemoteAddress(conn.RemoteAddr())
	switch method {
	case 0:
		// Legacy LambdaMOO/Mongoose format consumed by
		// $string_utils:connection_hostname_bsd():
		//   "port <listen-port> from <host>, port <remote-port>"
		listenPort := 0
		if globalConnManager != nil {
			listenPort = globalConnManager.GetListenPort()
		}
		return types.Ok(types.NewStr(fmt.Sprintf("port %d from %s, port %s", listenPort, host, port)))
	case 1:
		return types.Ok(types.NewStr(host))
	case 2:
		return types.Ok(types.NewStr(fmt.Sprintf("%s, port %s", host, port)))
	default:
		return types.Err(types.E_INVARG)
	}
}

// boot_player(player) -> int.
func builtinBootPlayer(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !ctx.IsWizard && player != ctx.Player {
		return types.Err(types.E_PERM)
	}

	if err := globalConnManager.BootPlayer(player); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

// switch_player(old_player, new_player [, silent]) -> int.
func builtinSwitchPlayer(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	oldPlayerVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	newPlayerVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if _, ok := args[2].(types.IntValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}

	if err := globalConnManager.SwitchPlayer(oldPlayerVal.ID(), newPlayerVal.ID()); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

// idle_seconds(player) -> int.
func builtinIdleSeconds(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	conn := resolveConnection(ctx, player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	idle := conn.IdleSeconds()
	if idle < 0 {
		idle = 0
	}
	return types.Ok(types.NewInt(idle))
}

// connected_seconds(player) -> int.
func builtinConnectedSeconds(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	conn := resolveConnection(ctx, player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	seconds := conn.ConnectedSeconds()
	if seconds < 0 {
		seconds = 0
	}
	return types.Ok(types.NewInt(seconds))
}

// connection_info(player) -> map.
func builtinConnectionInfo(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	conn := resolveConnection(ctx, player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	host, portText := parseRemoteAddress(conn.RemoteAddr())
	destPort := int64(0)
	_, _ = fmt.Sscanf(portText, "%d", &destPort)

	protocol := "IPv4"
	if strings.Contains(host, ":") {
		protocol = "IPv6"
	}

	result := types.NewMap([][2]types.Value{
		{types.NewStr("source_address"), types.NewStr("localhost")},
		{types.NewStr("source_ip"), types.NewStr("127.0.0.1")},
		{types.NewStr("source_port"), types.NewInt(int64(globalConnManager.GetListenPort()))},
		{types.NewStr("destination_address"), types.NewStr(host)},
		{types.NewStr("destination_ip"), types.NewStr(host)},
		{types.NewStr("destination_port"), types.NewInt(destPort)},
		{types.NewStr("protocol"), types.NewStr(protocol)},
		{types.NewStr("outbound"), types.NewInt(0)},
	})
	return types.Ok(result)
}

// connection_name_lookup(player [, rewrite]) -> int.
func builtinConnectionNameLookup(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if resolveConnection(ctx, player) == nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

// set_connection_option(conn, option, value) -> int.
func builtinSetConnectionOption(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if resolveConnection(ctx, player) == nil {
		return types.Err(types.E_INVARG)
	}
	if !ctx.IsWizard && player != ctx.Player {
		return types.Err(types.E_PERM)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	name := nameVal.Value()
	if !validConnectionOption(name) {
		return types.Err(types.E_INVARG)
	}

	setConnectionOption(player, name, args[2])
	return types.Ok(types.NewInt(0))
}

// connection_option(conn, option) -> value.
func builtinConnectionOption(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if resolveConnection(ctx, player) == nil {
		return types.Err(types.E_INVARG)
	}
	if !ctx.IsWizard && player != ctx.Player {
		return types.Err(types.E_PERM)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	name := nameVal.Value()
	if !validConnectionOption(name) {
		return types.Err(types.E_INVARG)
	}

	options := getConnectionOptions(player)
	value, ok := options[name]
	if !ok {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(value)
}

// read_http([type [, connection]]) -> map | E_PERM | E_ARGS | E_TYPE | E_INVARG.
func builtinReadHTTP(ctx *types.TaskContext, args []types.Value) types.Result {
	// Validate we have at least one argument (type).
	if len(args) == 0 {
		return types.Err(types.E_ARGS)
	}

	// First argument must be a string (type).
	typeVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	typeStr := typeVal.Value()

	// Validate type is "request" or "response".
	if typeStr != "request" && typeStr != "response" {
		return types.Err(types.E_INVARG)
	}

	// Second argument (if provided) must be an object (connection).
	var connection types.ObjID = ctx.Player
	if len(args) > 1 {
		connVal, ok := args[1].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		connection = connVal.ID()
	}

	// Permission checks (from ToastStunt bf_read_http).
	if len(args) > 1 {
		// With explicit connection: require wizard or owner of connection.
		if !ctx.IsWizard {
			// TODO: implement db_object_owner check when we have DB access.
			return types.Err(types.E_PERM)
		}
	} else {
		// Without explicit connection: require wizard.
		if !ctx.IsWizard {
			return types.Err(types.E_PERM)
		}
		// TODO: check last_input_task_id(connection) == current_task_id.
	}

	_ = connection

	// TODO: Implement HTTP parsing and task suspension.
	return types.Err(types.E_INVARG)
}
