package builtins

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"

	"barn/kernel"
	"barn/task"
	"barn/trace"
	"barn/types"
)

const ListenerProtocolTCP = "tcp"

type ListenerSpec struct {
	Protocol           string
	Object             types.ObjID
	Port               int64
	Interface          string
	Path               string
	PrintMessages      bool
	TLSCertificatePath string
	TLSKeyPath         string
}

type ListenerDescriptor struct {
	Protocol string
	Port     int64
	Path     string
}

type ListenerInfo struct {
	Object        types.ObjID
	Port          int64
	Protocol      string
	Path          string
	PrintMessages bool
	IPv6          bool
	Interface     string
	TLS           bool
}

// ConnectionManager interface to avoid import cycle.
type ConnectionManager interface {
	GetConnection(player types.ObjID) Connection
	ConnectedPlayers(showAll bool) []types.ObjID
	BootPlayer(player types.ObjID) error
	RecyclePlayer(player types.ObjID) error
	SwitchPlayer(oldPlayer, newPlayer types.ObjID) error
	GetListenPort() int
	ListenerInfos() []ListenerInfo
	AddListener(spec ListenerSpec) (ListenerDescriptor, error)
	RemoveListener(desc ListenerDescriptor) error
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
	ListenerPort() int64
}

type inputWakeConnection interface {
	WakeInputReader()
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

var heldCommandState = struct {
	mu       sync.Mutex
	byPlayer map[types.ObjID][]string
}{
	byPlayer: make(map[types.ObjID][]string),
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

func resolveConnection(ctx *kernel.TaskContext, player types.ObjID) Connection {
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
		"binary", "flush-command", "keep-alive", "intrinsic-commands":
		return true
	default:
		return false
	}
}

func defaultIntrinsicCommands() types.Value {
	return types.NewList([]types.Value{
		types.NewStr(".program"),
		types.NewStr("PREFIX"),
		types.NewStr("SUFFIX"),
		types.NewStr("OUTPUTPREFIX"),
		types.NewStr("OUTPUTSUFFIX"),
	})
}

func defaultConnectionOptions() map[string]types.Value {
	return map[string]types.Value{
		"hold-input":         types.NewInt(0),
		"client-echo":        types.NewInt(1),
		"disable-oob":        types.NewInt(0),
		"binary":             types.NewInt(0),
		"flush-command":      types.NewStr(""),
		"keep-alive":         types.NewInt(0),
		"intrinsic-commands": defaultIntrinsicCommands(),
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

func drainHeldCommands(player types.ObjID) []string {
	heldCommandState.mu.Lock()
	defer heldCommandState.mu.Unlock()
	lines := append([]string(nil), heldCommandState.byPlayer[player]...)
	delete(heldCommandState.byPlayer, player)
	return lines
}

func clearHeldCommands(player types.ObjID) {
	heldCommandState.mu.Lock()
	defer heldCommandState.mu.Unlock()
	delete(heldCommandState.byPlayer, player)
}

func heldInputEnabled(player types.ObjID) bool {
	return getConnectionOptions(player)["hold-input"].Truthy()
}

func ConnectionOptionTruthy(player types.ObjID, name string) bool {
	options := getConnectionOptions(player)
	value, ok := options[name]
	return ok && value.Truthy()
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
	options := getConnectionOptions(player)
	if flush, ok := options["flush-command"].(types.StrValue); ok && flush.Value() != "" && line == flush.Value() {
		clearHeldCommands(player)
		return true
	}

	held := heldInputEnabled(player)

	httpHeldInputState.mu.Lock()
	state := httpHeldInputState.byPlayer[player]
	if state != nil {
		pruneHTTPWaitersLocked(state)
	}
	// An active read_http waiter owns incoming input: the line belongs to that
	// read and must NOT also be held as a command. Otherwise read_http consumes
	// it here while drainHeldCommands() later replays the same line as a command
	// when hold-input is turned off (the http-cluster cascade bug).
	hadWaiter := state != nil && len(state.waiters) > 0

	if !held && !hadWaiter {
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

	// Hold the line as a pending command only when hold-input is on and there was
	// no active read_http waiter to consume it.
	if held && !hadWaiter {
		heldCommandState.mu.Lock()
		if atFront {
			heldCommandState.byPlayer[player] = append([]string{line}, heldCommandState.byPlayer[player]...)
		} else {
			heldCommandState.byPlayer[player] = append(heldCommandState.byPlayer[player], line)
		}
		heldCommandState.mu.Unlock()
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

func normalizeListenerProtocol(protocol string) string {
	if protocol == "" {
		return ListenerProtocolTCP
	}
	return strings.ToLower(protocol)
}

func listenerProtocolSupported(protocol string) bool {
	switch normalizeListenerProtocol(protocol) {
	case ListenerProtocolTCP, "tls", "ws", "wss":
		return true
	default:
		return false
	}
}

func listenerDescriptorValue(desc ListenerDescriptor) types.Value {
	protocol := normalizeListenerProtocol(desc.Protocol)
	if protocol == ListenerProtocolTCP && desc.Path == "" {
		return types.NewInt(desc.Port)
	}

	pairs := [][2]types.Value{
		{types.NewStr("protocol"), types.NewStr(protocol)},
		{types.NewStr("port"), types.NewInt(desc.Port)},
	}
	if desc.Path != "" {
		pairs = append(pairs, [2]types.Value{types.NewStr("path"), types.NewStr(desc.Path)})
	}
	return types.NewMap(pairs)
}

func listenerInfoDescriptor(info ListenerInfo) ListenerDescriptor {
	return ListenerDescriptor{
		Protocol: normalizeListenerProtocol(info.Protocol),
		Port:     info.Port,
		Path:     info.Path,
	}
}

func parseListenerDescriptorValue(value types.Value) (ListenerDescriptor, types.ErrorCode) {
	switch v := value.(type) {
	case types.IntValue:
		if v.Val < 0 || v.Val > 65535 {
			return ListenerDescriptor{}, types.E_INVARG
		}
		return ListenerDescriptor{Protocol: ListenerProtocolTCP, Port: v.Val}, types.E_NONE
	case types.MapValue:
		desc := ListenerDescriptor{Protocol: ListenerProtocolTCP}
		if protocolValue, ok := v.Get(types.NewStr("protocol")); ok {
			protocol, ok := protocolValue.(types.StrValue)
			if !ok {
				return ListenerDescriptor{}, types.E_TYPE
			}
			desc.Protocol = normalizeListenerProtocol(protocol.Value())
			if !listenerProtocolSupported(desc.Protocol) {
				return ListenerDescriptor{}, types.E_INVARG
			}
		}
		portValue, ok := v.Get(types.NewStr("port"))
		if !ok {
			return ListenerDescriptor{}, types.E_INVARG
		}
		port, ok := portValue.(types.IntValue)
		if !ok {
			return ListenerDescriptor{}, types.E_TYPE
		}
		if port.Val < 0 || port.Val > 65535 {
			return ListenerDescriptor{}, types.E_INVARG
		}
		desc.Port = port.Val
		if pathValue, ok := v.Get(types.NewStr("path")); ok {
			path, ok := pathValue.(types.StrValue)
			if !ok {
				return ListenerDescriptor{}, types.E_TYPE
			}
			desc.Path = path.Value()
		}
		return desc, types.E_NONE
	default:
		return ListenerDescriptor{}, types.E_TYPE
	}
}

func listenerDescriptorEqual(left, right ListenerDescriptor) bool {
	return normalizeListenerProtocol(left.Protocol) == normalizeListenerProtocol(right.Protocol) &&
		left.Port == right.Port &&
		left.Path == right.Path
}

// notify(player, message [, no_flush [, no_newline]]) -> int
func builtinNotify(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	noFlush := false
	if len(args) >= 3 {
		noFlush = args[2].Truthy()
	}

	conn := resolveConnection(ctx, player)
	if conn == nil {
		// MOO behavior: missing/disconnected target is a successful no-op.
		return types.Ok(types.NewInt(1))
	}

	if ctx != nil && ctx.StoreTxn != nil {
		ctx.PendingNotifications = append(ctx.PendingNotifications, kernel.PendingNotification{
			Player:  player,
			Message: message,
			NoFlush: noFlush,
		})
		return types.Ok(types.NewInt(0))
	}

	trace.Notify(player, message)
	if noFlush {
		conn.Buffer(message)
		return types.Ok(types.NewInt(0))
	}
	if err := conn.Send(message); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

func FlushPendingNotifications(ctx *kernel.TaskContext) types.ErrorCode {
	if ctx == nil || len(ctx.PendingNotifications) == 0 {
		return types.E_NONE
	}
	pending := ctx.PendingNotifications
	ctx.PendingNotifications = nil
	for _, note := range pending {
		conn := resolveConnection(ctx, note.Player)
		if conn == nil {
			continue
		}
		trace.Notify(note.Player, note.Message)
		if note.NoFlush {
			conn.Buffer(note.Message)
			continue
		}
		if err := conn.Send(note.Message); err != nil {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func DiscardPendingNotifications(ctx *kernel.TaskContext) {
	if ctx != nil {
		ctx.PendingNotifications = nil
	}
}

// listeners([find]) -> list of listener maps.
func builtinListeners(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
		case types.MapValue:
			desc, errCode := parseListenerDescriptorValue(v)
			if errCode != types.E_NONE {
				return types.Err(errCode)
			}
			filtered := infos[:0]
			for _, info := range infos {
				if listenerDescriptorEqual(listenerInfoDescriptor(info), desc) {
					filtered = append(filtered, info)
				}
			}
			infos = filtered
		default:
			return types.Err(types.E_TYPE)
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		leftProtocol := normalizeListenerProtocol(infos[i].Protocol)
		rightProtocol := normalizeListenerProtocol(infos[j].Protocol)
		if leftProtocol != rightProtocol {
			return leftProtocol < rightProtocol
		}
		if infos[i].Port != infos[j].Port {
			return infos[i].Port < infos[j].Port
		}
		if infos[i].Path != infos[j].Path {
			return infos[i].Path < infos[j].Path
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
		tls := int64(0)
		if info.TLS {
			tls = 1
		}
		entry := types.NewMap([][2]types.Value{
			{types.NewStr("object"), types.NewObj(info.Object)},
			{types.NewStr("port"), types.NewInt(info.Port)},
			{types.NewStr("protocol"), types.NewStr(normalizeListenerProtocol(info.Protocol))},
			{types.NewStr("path"), types.NewStr(info.Path)},
			{types.NewStr("print-messages"), types.NewInt(printMessages)},
			{types.NewStr("ipv6"), types.NewInt(ipv6)},
			{types.NewStr("interface"), types.NewStr(info.Interface)},
			{types.NewStr("TLS"), types.NewInt(tls)},
		})
		entries = append(entries, entry)
	}

	return types.Ok(types.NewList(entries))
}

// connected_players([show_all]) -> list.
func builtinConnectedPlayers(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinConnectionName(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
		if conn.ListenerPort() > 0 {
			listenPort = int(conn.ListenerPort())
		} else if globalConnManager != nil {
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
func builtinBootPlayer(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if ctx != nil && ctx.StoreTxn != nil {
		ctx.PendingBootPlayers = append(ctx.PendingBootPlayers, player)
		return types.Ok(types.NewInt(0))
	}
	if err := globalConnManager.BootPlayer(player); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

func FlushPendingBootPlayers(ctx *kernel.TaskContext) types.ErrorCode {
	if ctx == nil || len(ctx.PendingBootPlayers) == 0 {
		return types.E_NONE
	}
	if globalConnManager == nil {
		return types.E_INVARG
	}
	pending := ctx.PendingBootPlayers
	ctx.PendingBootPlayers = nil
	for _, player := range pending {
		if resolveConnection(ctx, player) == nil {
			continue
		}
		if err := globalConnManager.BootPlayer(player); err != nil {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func DiscardPendingBootPlayers(ctx *kernel.TaskContext) {
	if ctx != nil {
		ctx.PendingBootPlayers = nil
	}
}

// switch_player(old_player, new_player [, silent]) -> int.
func builtinSwitchPlayer(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if resolveConnection(ctx, oldPlayerVal.ID()) == nil {
		return types.Err(types.E_INVARG)
	}
	if ctx != nil && ctx.StoreTxn != nil {
		ctx.PendingConnectionSwitches = append(ctx.PendingConnectionSwitches, kernel.PendingConnectionSwitch{
			OldPlayer: oldPlayerVal.ID(),
			NewPlayer: newPlayerVal.ID(),
		})
		return types.Ok(types.NewInt(0))
	}

	if err := globalConnManager.SwitchPlayer(oldPlayerVal.ID(), newPlayerVal.ID()); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

func FlushPendingConnectionSwitches(ctx *kernel.TaskContext) types.ErrorCode {
	if ctx == nil || len(ctx.PendingConnectionSwitches) == 0 {
		return types.E_NONE
	}
	if globalConnManager == nil {
		return types.E_INVARG
	}
	pending := ctx.PendingConnectionSwitches
	ctx.PendingConnectionSwitches = nil
	for _, sw := range pending {
		if err := globalConnManager.SwitchPlayer(sw.OldPlayer, sw.NewPlayer); err != nil {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func DiscardPendingConnectionSwitches(ctx *kernel.TaskContext) {
	if ctx != nil {
		ctx.PendingConnectionSwitches = nil
	}
}

// idle_seconds(player) -> int.
func builtinIdleSeconds(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinConnectedSeconds(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinConnectionInfo(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	sourcePort := conn.ListenerPort()
	if sourcePort <= 0 {
		sourcePort = int64(globalConnManager.GetListenPort())
	}

	protocol := "IPv4"
	if strings.Contains(host, ":") {
		protocol = "IPv6"
	}

	result := types.NewMap([][2]types.Value{
		{types.NewStr("source_address"), types.NewStr("localhost")},
		{types.NewStr("source_ip"), types.NewStr("127.0.0.1")},
		{types.NewStr("source_port"), types.NewInt(sourcePort)},
		{types.NewStr("destination_address"), types.NewStr(host)},
		{types.NewStr("destination_ip"), types.NewStr(host)},
		{types.NewStr("destination_port"), types.NewInt(destPort)},
		{types.NewStr("protocol"), types.NewStr(protocol)},
		{types.NewStr("outbound"), types.NewInt(0)},
	})
	return types.Ok(result)
}

// connection_name_lookup(player [, rewrite]) -> int.
func builtinConnectionNameLookup(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinSetConnectionOption(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}
	conn := resolveConnection(ctx, player)
	if conn == nil {
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
	if name == "intrinsic-commands" {
		if args[2].Truthy() {
			if list, ok := args[2].(types.ListValue); ok {
				allowed := map[string]bool{
					".program":     true,
					"PREFIX":       true,
					"SUFFIX":       true,
					"OUTPUTPREFIX": true,
					"OUTPUTSUFFIX": true,
				}
				for i := 1; i <= list.Len(); i++ {
					str, ok := list.Get(i).(types.StrValue)
					if !ok || !allowed[str.Value()] {
						return types.Err(types.E_INVARG)
					}
				}
			} else {
				args[2] = defaultIntrinsicCommands()
			}
		}
	}

	setConnectionOption(player, name, args[2])
	if name == "binary" && args[2].Truthy() {
		if wakeConn, ok := conn.(inputWakeConnection); ok {
			wakeConn.WakeInputReader()
		}
	}
	if name == "hold-input" && !args[2].Truthy() && globalInputForcer != nil {
		for _, line := range drainHeldCommands(player) {
			globalInputForcer.ForceInput(player, line, false)
		}
	}
	return types.Ok(types.NewInt(0))
}

// connection_option(conn, option) -> value.
func builtinConnectionOption(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
func builtinReadHTTP(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
