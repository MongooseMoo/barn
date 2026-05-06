package builtins

import (
	"barn/task"
	"barn/trace"
	"barn/types"
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type ListenerInfo struct {
	Object        types.ObjID
	Port          int64
	PrintMessages bool
	IPv6          bool
	Interface     string
}

// ConnectionManager interface to avoid import cycle.
type ConnectionManager interface {
	GetConnection(player types.ObjID) Connection
	ConnectedPlayers(showAll bool) []types.ObjID
	BootPlayer(player types.ObjID) error
	SwitchPlayer(oldPlayer, newPlayer types.ObjID) error
	GetListenPort() int
	ListenerInfos() []ListenerInfo
	AddListener(object types.ObjID, port int64, printMessages bool) (int64, error)
	RemoveListener(port int64) error
	OpenNetworkConnection(host string, port int64) (types.ObjID, error)
	ConnectionNameLookup(player types.ObjID, rewrite bool) (string, error)
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
	GetResolvedName() string
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

type httpReadWaiter struct {
	task *task.Task
	kind string
}

type httpHeldInput struct {
	buffer       []byte
	invalidCount int
	waiters      []httpReadWaiter
}

type httpWake struct {
	task  *task.Task
	value types.Value
}

var httpHeldInputState = struct {
	mu       sync.Mutex
	byPlayer map[types.ObjID]*httpHeldInput
}{
	byPlayer: make(map[types.ObjID]*httpHeldInput),
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

func heldInputEnabled(player types.ObjID) bool {
	return getConnectionOptions(player)["hold-input"].Truthy()
}

func getOrCreateHeldHTTPInput(player types.ObjID) *httpHeldInput {
	state, ok := httpHeldInputState.byPlayer[player]
	if !ok {
		state = &httpHeldInput{}
		httpHeldInputState.byPlayer[player] = state
	}
	return state
}

func trimHTTPLeadingWhitespace(data []byte) []byte {
	start := 0
	for start < len(data) && (data[start] == ' ' || data[start] == '\t') {
		start++
	}
	return data[start:]
}

func isHTTPTokenByte(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	}

	switch b {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

func isValidHTTPToken(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for _, b := range data {
		if !isHTTPTokenByte(b) {
			return false
		}
	}
	return true
}

func isValidHTTPRequestURI(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for _, b := range data {
		if b <= 0x20 || b >= 0x7f {
			return false
		}
	}
	return true
}

func readHTTPCRLFLine(data []byte, start int) ([]byte, int, bool) {
	for i := start; i+1 < len(data); i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			return data[start:i], i + 2, true
		}
	}
	return nil, 0, false
}

func newHTTPErrorValue(code string) types.Value {
	return types.NewMap([][2]types.Value{
		{types.NewStr("error"), types.NewList([]types.Value{types.NewStr(code)})},
	})
}

func parseHTTPHeaders(data []byte, start int) ([][2]types.Value, int, int, bool, bool, bool) {
	headers := make([][2]types.Value, 0)
	lastHeader := -1
	contentLength := -1
	chunked := false

	for pos := start; ; {
		line, next, ok := readHTTPCRLFLine(data, pos)
		if !ok {
			return nil, 0, 0, false, false, true
		}
		pos = next
		if len(line) == 0 {
			return headers, pos, contentLength, chunked, false, false
		}

		if line[0] == ' ' || line[0] == '\t' {
			if lastHeader < 0 {
				return nil, pos, 0, false, true, false
			}
			continued := encodeBinaryStr(trimHTTPLeadingWhitespace(line))
			headers[lastHeader][1] = types.NewStr(headers[lastHeader][1].(types.StrValue).Value() + continued)
			continue
		}

		colon := bytes.IndexByte(line, ':')
		if colon <= 0 || !isValidHTTPToken(line[:colon]) {
			return nil, pos, 0, false, true, false
		}

		name := string(line[:colon])
		value := encodeBinaryStr(trimHTTPLeadingWhitespace(line[colon+1:]))
		headers = append(headers, [2]types.Value{types.NewStr(name), types.NewStr(value)})
		lastHeader = len(headers) - 1

		switch strings.ToLower(name) {
		case "content-length":
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && n >= 0 {
				contentLength = n
			}
		case "transfer-encoding":
			chunked = strings.Contains(strings.ToLower(value), "chunked")
		}
	}
}

func parseHTTPChunkedBody(data []byte, start int) (string, int, bool) {
	var body []byte
	pos := start

	for {
		line, next, ok := readHTTPCRLFLine(data, pos)
		if !ok {
			return "", 0, false
		}
		sizeText := string(line)
		if semi := strings.IndexByte(sizeText, ';'); semi >= 0 {
			sizeText = sizeText[:semi]
		}
		sizeText = strings.TrimSpace(sizeText)
		if sizeText == "" {
			return "", 0, false
		}

		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil || size < 0 {
			return "", 0, false
		}

		pos = next
		if size == 0 {
			for {
				trailer, nextTrailer, ok := readHTTPCRLFLine(data, pos)
				if !ok {
					return "", 0, false
				}
				pos = nextTrailer
				if len(trailer) == 0 {
					return encodeBinaryStr(body), pos, true
				}
			}
		}

		if len(data[pos:]) < int(size)+2 {
			return "", 0, false
		}
		body = append(body, data[pos:pos+int(size)]...)
		if data[pos+int(size)] != '\r' || data[pos+int(size)+1] != '\n' {
			return "", 0, false
		}
		pos += int(size) + 2
	}
}

func parseHTTPRequest(data []byte) (types.Value, int, bool) {
	line, pos, ok := readHTTPCRLFLine(data, 0)
	if !ok {
		return nil, 0, false
	}
	parts := bytes.Fields(line)
	if (len(parts) != 2 && len(parts) != 3) || !isValidHTTPToken(parts[0]) {
		return newHTTPErrorValue("INVALID_METHOD"), pos, true
	}
	if !isValidHTTPRequestURI(parts[1]) {
		return newHTTPErrorValue("INVALID_PATH"), pos, true
	}

	headers, bodyStart, contentLength, chunked, badHeader, incomplete := parseHTTPHeaders(data, pos)
	if incomplete {
		return nil, 0, false
	}
	if badHeader {
		return newHTTPErrorValue("INVALID_HEADER_TOKEN"), bodyStart, true
	}

	pairs := [][2]types.Value{
		{types.NewStr("method"), types.NewStr(string(parts[0]))},
		{types.NewStr("uri"), types.NewStr(string(parts[1]))},
		{types.NewStr("headers"), types.NewMap(headers)},
	}
	if len(parts) == 3 {
		pairs = append(pairs, [2]types.Value{types.NewStr("version"), types.NewStr(string(parts[2]))})
	}
	consumed := bodyStart
	if chunked {
		body, next, complete := parseHTTPChunkedBody(data, bodyStart)
		if !complete {
			return nil, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(body)})
		consumed = next
	} else if contentLength >= 0 {
		if len(data[bodyStart:]) < contentLength {
			return nil, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(encodeBinaryStr(data[bodyStart : bodyStart+contentLength]))})
		consumed = bodyStart + contentLength
	}
	return types.NewMap(pairs), consumed, true
}

func parseHTTPResponse(data []byte) (types.Value, int, bool) {
	line, pos, ok := readHTTPCRLFLine(data, 0)
	if !ok {
		return nil, 0, false
	}
	if !bytes.HasPrefix(line, []byte("HTTP/")) {
		return newHTTPErrorValue("INVALID_CONSTANT"), pos, true
	}
	parts := bytes.Fields(line)
	if len(parts) < 2 {
		return newHTTPErrorValue("INVALID_STATUS"), pos, true
	}
	statusToken := parts[1]
	if len(statusToken) != 3 {
		return newHTTPErrorValue("INVALID_STATUS"), pos, true
	}
	for _, b := range statusToken {
		if b < '0' || b > '9' {
			return newHTTPErrorValue("INVALID_STATUS"), pos, true
		}
	}
	status, _ := strconv.Atoi(string(statusToken))

	headers, bodyStart, contentLength, chunked, badHeader, incomplete := parseHTTPHeaders(data, pos)
	if incomplete {
		return nil, 0, false
	}
	if badHeader {
		return newHTTPErrorValue("INVALID_HEADER_TOKEN"), bodyStart, true
	}

	pairs := [][2]types.Value{
		{types.NewStr("version"), types.NewStr(string(parts[0]))},
		{types.NewStr("status"), types.NewInt(int64(status))},
		{types.NewStr("headers"), types.NewMap(headers)},
	}
	if len(parts) >= 3 {
		pairs = append(pairs, [2]types.Value{types.NewStr("reason"), types.NewStr(string(parts[2]))})
	}

	consumed := bodyStart
	if chunked {
		body, next, complete := parseHTTPChunkedBody(data, bodyStart)
		if !complete {
			return nil, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(body)})
		consumed = next
	} else if contentLength >= 0 {
		if len(data[bodyStart:]) < contentLength {
			return nil, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(encodeBinaryStr(data[bodyStart : bodyStart+contentLength]))})
		consumed = bodyStart + contentLength
	}
	return types.NewMap(pairs), consumed, true
}

func parseHTTPMessage(kind string, data []byte) (types.Value, int, bool) {
	if kind == "request" {
		return parseHTTPRequest(data)
	}
	return parseHTTPResponse(data)
}

func collectHTTPWakeupsLocked(player types.ObjID, state *httpHeldInput) []httpWake {
	pruneHTTPWaitersLocked(state)
	wakes := make([]httpWake, 0)
	for len(state.waiters) > 0 {
		waiter := state.waiters[0]
		if state.invalidCount > 0 {
			state.invalidCount--
			state.waiters = state.waiters[1:]
			wakes = append(wakes, httpWake{task: waiter.task, value: types.NewInt(0)})
			continue
		}

		value, consumed, complete := parseHTTPMessage(waiter.kind, state.buffer)
		if !complete {
			break
		}
		if consumed > 0 {
			state.buffer = append([]byte(nil), state.buffer[consumed:]...)
		}
		state.waiters = state.waiters[1:]
		wakes = append(wakes, httpWake{task: waiter.task, value: value})
	}

	if !heldInputEnabled(player) && len(state.buffer) == 0 && state.invalidCount == 0 && len(state.waiters) == 0 {
		delete(httpHeldInputState.byPlayer, player)
	}
	return wakes
}

func HandleHeldInput(player types.ObjID, line string, atFront bool) bool {
	httpHeldInputState.mu.Lock()
	state := httpHeldInputState.byPlayer[player]
	if !heldInputEnabled(player) && (state == nil || len(state.waiters) == 0) {
		httpHeldInputState.mu.Unlock()
		return false
	}

	state = getOrCreateHeldHTTPInput(player)
	decoded, invalid := decodeBinaryString(line)
	if invalid {
		state.invalidCount++
	} else if atFront {
		state.buffer = append(append([]byte(nil), decoded...), state.buffer...)
	} else {
		state.buffer = append(state.buffer, decoded...)
	}

	wakes := collectHTTPWakeupsLocked(player, state)
	httpHeldInputState.mu.Unlock()

	for _, wake := range wakes {
		wake.task.Resume(wake.value)
	}
	return true
}

func prepareHTTPRead(player types.ObjID, kind string, t *task.Task) (types.Value, bool) {
	httpHeldInputState.mu.Lock()
	defer httpHeldInputState.mu.Unlock()

	state := getOrCreateHeldHTTPInput(player)
	pruneHTTPWaitersLocked(state)
	if state.invalidCount > 0 {
		state.invalidCount--
		if !heldInputEnabled(player) && len(state.buffer) == 0 && len(state.waiters) == 0 && state.invalidCount == 0 {
			delete(httpHeldInputState.byPlayer, player)
		}
		return types.NewInt(0), true
	}

	value, consumed, complete := parseHTTPMessage(kind, state.buffer)
	if complete {
		if consumed > 0 {
			state.buffer = append([]byte(nil), state.buffer[consumed:]...)
		}
		if !heldInputEnabled(player) && len(state.buffer) == 0 && len(state.waiters) == 0 && state.invalidCount == 0 {
			delete(httpHeldInputState.byPlayer, player)
		}
		return value, true
	}

	state.waiters = append(state.waiters, httpReadWaiter{task: t, kind: kind})
	return nil, false
}

func pruneHTTPWaitersLocked(state *httpHeldInput) {
	if len(state.waiters) == 0 {
		return
	}
	kept := state.waiters[:0]
	for _, waiter := range state.waiters {
		if waiter.task != nil && waiter.task.GetState() == task.TaskSuspended {
			kept = append(kept, waiter)
		}
	}
	state.waiters = kept
}

func HasPendingHTTPRead(player types.ObjID) bool {
	httpHeldInputState.mu.Lock()
	defer httpHeldInputState.mu.Unlock()

	state := httpHeldInputState.byPlayer[player]
	if state == nil {
		return false
	}
	pruneHTTPWaitersLocked(state)
	return len(state.waiters) > 0
}

func CancelHTTPReadTask(taskID int64) {
	httpHeldInputState.mu.Lock()
	defer httpHeldInputState.mu.Unlock()

	for player, state := range httpHeldInputState.byPlayer {
		if len(state.waiters) == 0 {
			continue
		}
		removed := false
		kept := state.waiters[:0]
		for _, waiter := range state.waiters {
			if waiter.task != nil && waiter.task.ID == taskID {
				removed = true
				continue
			}
			kept = append(kept, waiter)
		}
		state.waiters = kept
		if removed {
			state.buffer = nil
			state.invalidCount = 0
		}
		if !heldInputEnabled(player) && len(state.buffer) == 0 && state.invalidCount == 0 && len(state.waiters) == 0 {
			delete(httpHeldInputState.byPlayer, player)
		}
	}
}

func ClearAllHeldHTTPInput() {
	httpHeldInputState.mu.Lock()
	defer httpHeldInputState.mu.Unlock()
	httpHeldInputState.byPlayer = make(map[types.ObjID]*httpHeldInput)
}

func CloseHeldHTTPInput(player types.ObjID) {
	httpHeldInputState.mu.Lock()
	state := httpHeldInputState.byPlayer[player]
	if state == nil {
		httpHeldInputState.mu.Unlock()
		return
	}
	waiters := append([]httpReadWaiter(nil), state.waiters...)
	delete(httpHeldInputState.byPlayer, player)
	httpHeldInputState.mu.Unlock()

	for _, waiter := range waiters {
		if waiter.task != nil {
			waiter.task.Resume(types.NewInt(0))
		}
	}
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

	infos := globalConnManager.ListenerInfos()
	if len(args) == 1 {
		switch v := args[0].(type) {
		case types.ObjValue:
			filtered := infos[:0]
			for _, info := range infos {
				if info.Object == v.ID() {
					filtered = append(filtered, info)
				}
			}
			infos = filtered
		case types.IntValue:
			filtered := infos[:0]
			for _, info := range infos {
				if info.Port == v.Val {
					filtered = append(filtered, info)
				}
			}
			infos = filtered
		default:
			return types.Err(types.E_TYPE)
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Port != infos[j].Port {
			return infos[i].Port < infos[j].Port
		}
		return infos[i].Object < infos[j].Object
	})

	entries := make([]types.Value, 0, len(infos))
	for _, info := range infos {
		printMessages := int64(0)
		if info.PrintMessages {
			printMessages = 1
		}
		ipv6 := int64(0)
		if info.IPv6 {
			ipv6 = 1
		}
		entry := types.NewMap([][2]types.Value{
			{types.NewStr("object"), types.NewObj(info.Object)},
			{types.NewStr("port"), types.NewInt(info.Port)},
			{types.NewStr("print-messages"), types.NewInt(printMessages)},
			{types.NewStr("ipv6"), types.NewInt(ipv6)},
			{types.NewStr("interface"), types.NewStr(info.Interface)},
		})
		entries = append(entries, entry)
	}

	return types.Ok(types.NewList(entries))
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

	if method == 0 {
		if resolved := conn.GetResolvedName(); resolved != "" {
			return types.Ok(types.NewStr(resolved))
		}
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

	if resolveConnection(ctx, player) == nil {
		return types.Ok(types.NewInt(0))
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
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
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
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}

	rewrite := false
	if len(args) == 2 {
		rewrite = args[1].Truthy()
	}

	name, err := globalConnManager.ConnectionNameLookup(player, rewrite)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewStr(name))
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
	if name == "keep-alive" {
		switch args[2].(type) {
		case types.IntValue, types.MapValue:
		default:
			return types.Err(types.E_INVARG)
		}
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
	if len(args) == 0 {
		return types.Err(types.E_ARGS)
	}
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	typeVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	typeStr := typeVal.Value()

	var connection types.ObjID = ctx.Player
	if len(args) > 1 {
		connVal, ok := args[1].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		connection = connVal.ID()
	}

	if typeStr != "request" && typeStr != "response" {
		return types.Err(types.E_INVARG)
	}
	if resolveConnection(ctx, connection) == nil {
		return types.Err(types.E_INVARG)
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
	if task.GetManager().FindReadingTask(connection) != nil || HasPendingHTTPRead(connection) {
		return types.Err(types.E_INVARG)
	}

	t, ok := ctx.Task.(*task.Task)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	value, complete := prepareHTTPRead(connection, typeStr, t)
	if complete {
		return types.Ok(value)
	}

	task.GetManager().SuspendTask(t, -1)
	return types.Suspend(-1)
}
