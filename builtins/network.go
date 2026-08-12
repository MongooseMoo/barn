package builtins

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
)

// ConnectionManager interface to avoid import cycle.
type ConnectionManager interface {
	GetConnection(player types.ObjID) Connection
	ConnectedPlayers(showAll bool) []types.ObjID
	BootPlayer(player types.ObjID) error
	RecyclePlayer(player types.ObjID) error
	SwitchPlayer(oldPlayer, newPlayer types.ObjID) error
	GetListenPort() int
	ListenerInfos() []listener.Info
	AddListener(spec listener.Spec) (listener.Descriptor, error)
	RemoveListener(desc listener.Descriptor) error
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
	ListenerPort() (int64, bool)
}

type outboundConnectionInfo interface {
	IsOutbound() bool
	OutboundSourceAddr() string
	OutboundDestinationAddr() string
}

type inputWakeConnection interface {
	WakeInputReader()
}

// InputForcer allows builtins to inject input lines into a player's stream.
// Implemented by the execution engine to avoid import cycles.
type InputForcer interface {
	ForceInput(player types.ObjID, line string, atFront bool)
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

func parseConnectionTarget(v types.Value) (types.ObjID, bool) {
	switch v.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		return v.ID(), true
	case types.TYPE_INT:
		return types.ObjID(v.Int()), true
	default:
		return types.ObjNothing, false
	}
}

func resolveConnection(ctx *Execution, player types.ObjID) Connection {
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return nil
	}
	return cm.GetConnection(player)
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

func (r *Registry) getConnectionOptions(player types.ObjID) map[string]types.Value {
	state := &r.runtime.connectionOptions
	state.mu.RLock()
	defer state.mu.RUnlock()
	existing, ok := state.byPlayer[player]
	if !ok {
		return defaultConnectionOptions()
	}
	// Copy under the read lock: setConnectionOption mutates the stored map in
	// place, so releasing before the copy races a concurrent writer.
	out := make(map[string]types.Value, len(existing))
	for k, v := range existing {
		out[k] = v
	}
	return out
}

func (r *Registry) setConnectionOption(player types.ObjID, name string, value types.Value) {
	state := &r.runtime.connectionOptions
	state.mu.Lock()
	defer state.mu.Unlock()

	existing, ok := state.byPlayer[player]
	if !ok {
		existing = defaultConnectionOptions()
		state.byPlayer[player] = existing
	}
	existing[name] = value
}

func (r *Registry) drainHeldCommands(player types.ObjID) []string {
	state := &r.runtime.heldCommands
	state.mu.Lock()
	defer state.mu.Unlock()
	lines := append([]string(nil), state.byPlayer[player]...)
	delete(state.byPlayer, player)
	return lines
}

func (r *Registry) clearHeldCommands(player types.ObjID) {
	state := &r.runtime.heldCommands
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.byPlayer, player)
}

func (r *Registry) heldInputEnabled(player types.ObjID) bool {
	return r.getConnectionOptions(player)["hold-input"].Truthy()
}

func (r *Registry) ConnectionOptionTruthy(player types.ObjID, name string) bool {
	options := r.getConnectionOptions(player)
	value, ok := options[name]
	return ok && value.Truthy()
}

func (r *Registry) getOrCreateHeldHTTPInput(player types.ObjID) *httpHeldInput {
	state, ok := r.runtime.heldHTTPInput.byPlayer[player]
	if !ok {
		state = &httpHeldInput{}
		r.runtime.heldHTTPInput.byPlayer[player] = state
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
			headers[lastHeader][1] = types.NewStr(headers[lastHeader][1].Str() + continued)
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

		// Check the attacker-controlled size before converting it to int or
		// adding the trailing CRLF length. Either operation can overflow for a
		// syntactically valid, very large chunk size.
		remaining := len(data) - pos
		if remaining < 2 || size > int64(remaining-2) {
			return "", 0, false
		}
		chunkSize := int(size)
		body = append(body, data[pos:pos+chunkSize]...)
		if data[pos+chunkSize] != '\r' || data[pos+chunkSize+1] != '\n' {
			return "", 0, false
		}
		pos += chunkSize + 2
	}
}

func parseHTTPRequest(data []byte) (types.Value, int, bool) {
	line, pos, ok := readHTTPCRLFLine(data, 0)
	if !ok {
		return types.None, 0, false
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
		return types.None, 0, false
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
			return types.None, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(body)})
		consumed = next
	} else if contentLength >= 0 {
		if len(data[bodyStart:]) < contentLength {
			return types.None, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(encodeBinaryStr(data[bodyStart : bodyStart+contentLength]))})
		consumed = bodyStart + contentLength
	}
	return types.NewMap(pairs), consumed, true
}

func parseHTTPResponse(data []byte) (types.Value, int, bool) {
	line, pos, ok := readHTTPCRLFLine(data, 0)
	if !ok {
		return types.None, 0, false
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
		return types.None, 0, false
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
			return types.None, 0, false
		}
		pairs = append(pairs, [2]types.Value{types.NewStr("body"), types.NewStr(body)})
		consumed = next
	} else if contentLength >= 0 {
		if len(data[bodyStart:]) < contentLength {
			return types.None, 0, false
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

func (r *Registry) collectHTTPWakeupsLocked(player types.ObjID, state *httpHeldInput) []httpWake {
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

	if !r.heldInputEnabled(player) && len(state.buffer) == 0 && state.invalidCount == 0 && len(state.waiters) == 0 {
		delete(r.runtime.heldHTTPInput.byPlayer, player)
	}
	return wakes
}

func (r *Registry) HandleHeldInput(player types.ObjID, line string, atFront bool) (bool, []string) {
	options := r.getConnectionOptions(player)
	if flush := options["flush-command"]; flush.Type() == types.TYPE_STR && flush.Str() != "" && strings.EqualFold(line, flush.Str()) {
		lines := r.drainHeldCommands(player)
		if lines == nil {
			lines = []string{}
		}
		return true, lines
	}

	held := r.heldInputEnabled(player)

	hadWaiter, wakes, handled := func() (bool, []httpWake, bool) {
		stateSet := &r.runtime.heldHTTPInput
		stateSet.mu.Lock()
		defer stateSet.mu.Unlock()

		state := stateSet.byPlayer[player]
		if state != nil {
			pruneHTTPWaitersLocked(state)
		}
		// An active read_http waiter owns incoming input: the line belongs to that
		// read and must NOT also be held as a command. Otherwise read_http consumes
		// it here while drainHeldCommands() later replays the same line as a command
		// when hold-input is turned off (the http-cluster cascade bug).
		hadWaiter := state != nil && len(state.waiters) > 0

		if !held && !hadWaiter {
			return false, nil, false
		}

		state = r.getOrCreateHeldHTTPInput(player)
		decoded, invalid := decodeBinaryString(line)
		if invalid {
			state.invalidCount++
		} else if atFront {
			state.buffer = append(append([]byte(nil), decoded...), state.buffer...)
		} else {
			state.buffer = append(state.buffer, decoded...)
		}

		return hadWaiter, r.collectHTTPWakeupsLocked(player, state), true
	}()
	if !handled {
		return false, nil
	}

	for _, wake := range wakes {
		wake.task.Resume(wake.value)
	}

	// Hold the line as a pending command only when hold-input is on and there was
	// no active read_http waiter to consume it.
	if held && !hadWaiter {
		state := &r.runtime.heldCommands
		state.mu.Lock()
		if atFront {
			state.byPlayer[player] = append([]string{line}, state.byPlayer[player]...)
		} else {
			state.byPlayer[player] = append(state.byPlayer[player], line)
		}
		state.mu.Unlock()
	}
	return true, nil
}

func (r *Registry) prepareHTTPRead(player types.ObjID, kind string, t *task.Task) (types.Value, bool) {
	stateSet := &r.runtime.heldHTTPInput
	stateSet.mu.Lock()
	defer stateSet.mu.Unlock()

	state := r.getOrCreateHeldHTTPInput(player)
	pruneHTTPWaitersLocked(state)
	if state.invalidCount > 0 {
		state.invalidCount--
		if !r.heldInputEnabled(player) && len(state.buffer) == 0 && len(state.waiters) == 0 && state.invalidCount == 0 {
			delete(stateSet.byPlayer, player)
		}
		return types.NewInt(0), true
	}

	value, consumed, complete := parseHTTPMessage(kind, state.buffer)
	if complete {
		if consumed > 0 {
			state.buffer = append([]byte(nil), state.buffer[consumed:]...)
		}
		if !r.heldInputEnabled(player) && len(state.buffer) == 0 && len(state.waiters) == 0 && state.invalidCount == 0 {
			delete(stateSet.byPlayer, player)
		}
		return value, true
	}

	state.waiters = append(state.waiters, httpReadWaiter{task: t, kind: kind})
	return types.None, false
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

func (r *Registry) HasPendingHTTPRead(player types.ObjID) bool {
	stateSet := &r.runtime.heldHTTPInput
	stateSet.mu.Lock()
	defer stateSet.mu.Unlock()

	state := stateSet.byPlayer[player]
	if state == nil {
		return false
	}
	pruneHTTPWaitersLocked(state)
	return len(state.waiters) > 0
}

func (r *Registry) CancelHTTPReadTask(taskID int64) {
	stateSet := &r.runtime.heldHTTPInput
	stateSet.mu.Lock()
	defer stateSet.mu.Unlock()

	for player, state := range stateSet.byPlayer {
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
		if !r.heldInputEnabled(player) && len(state.buffer) == 0 && state.invalidCount == 0 && len(state.waiters) == 0 {
			delete(stateSet.byPlayer, player)
		}
	}
}

func (r *Registry) ClearAllHeldHTTPInput() {
	state := &r.runtime.heldHTTPInput
	state.mu.Lock()
	defer state.mu.Unlock()
	state.byPlayer = make(map[types.ObjID]*httpHeldInput)
}

func (r *Registry) CloseHeldHTTPInput(player types.ObjID) {
	stateSet := &r.runtime.heldHTTPInput
	stateSet.mu.Lock()
	state := stateSet.byPlayer[player]
	if state == nil {
		stateSet.mu.Unlock()
		return
	}
	waiters := append([]httpReadWaiter(nil), state.waiters...)
	delete(stateSet.byPlayer, player)
	stateSet.mu.Unlock()

	for _, waiter := range waiters {
		if waiter.task != nil {
			waiter.task.Kill()
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

func listenerDescriptorValue(desc listener.Descriptor) types.Value {
	protocol := listener.NormalizeProtocol(desc.Protocol)
	if protocol == listener.ProtocolTCP && desc.Path == "" && !desc.IPv6 {
		return types.NewInt(desc.Port)
	}

	ipv6 := int64(0)
	if desc.IPv6 {
		ipv6 = 1
	}
	pairs := [][2]types.Value{
		{types.NewStr("protocol"), types.NewStr(string(protocol))},
		{types.NewStr("port"), types.NewInt(desc.Port)},
		{types.NewStr("ipv6"), types.NewInt(ipv6)},
	}
	if desc.Path != "" {
		pairs = append(pairs, [2]types.Value{types.NewStr("path"), types.NewStr(desc.Path)})
	}
	return types.NewMap(pairs)
}

func listenerInfoDescriptor(info listener.Info) listener.Descriptor {
	return listener.Descriptor{
		Protocol: listener.NormalizeProtocol(info.Protocol),
		Port:     info.Port,
		IPv6:     info.IPv6,
		Path:     info.Path,
	}
}

func parseListenerDescriptorValue(value types.Value) (listener.Descriptor, types.ErrorCode) {
	switch value.Type() {
	case types.TYPE_INT:
		if value.Int() < 0 || value.Int() > 65535 {
			return listener.Descriptor{}, types.E_INVARG
		}
		return listener.Descriptor{Protocol: listener.ProtocolTCP, Port: value.Int()}, types.E_NONE
	case types.TYPE_MAP:
		desc := listener.Descriptor{Protocol: listener.ProtocolTCP}
		if protocolValue, ok := value.MapGet(types.NewStr("protocol")); ok {
			if protocolValue.Type() != types.TYPE_STR {
				return listener.Descriptor{}, types.E_TYPE
			}
			desc.Protocol = listener.NormalizeProtocol(listener.Protocol(protocolValue.Str()))
			if !listener.IsSupportedProtocol(desc.Protocol) {
				return listener.Descriptor{}, types.E_INVARG
			}
		}
		portValue, ok := value.MapGet(types.NewStr("port"))
		if !ok {
			return listener.Descriptor{}, types.E_INVARG
		}
		if portValue.Type() != types.TYPE_INT {
			return listener.Descriptor{}, types.E_TYPE
		}
		if portValue.Int() < 0 || portValue.Int() > 65535 {
			return listener.Descriptor{}, types.E_INVARG
		}
		desc.Port = portValue.Int()
		if pathValue, ok := value.MapGet(types.NewStr("path")); ok {
			if pathValue.Type() != types.TYPE_STR {
				return listener.Descriptor{}, types.E_TYPE
			}
			desc.Path = pathValue.Str()
		}
		if ipv6Value, ok := value.MapGet(types.NewStr("ipv6")); ok {
			desc.IPv6 = ipv6Value.Truthy()
		}
		return desc, types.E_NONE
	default:
		return listener.Descriptor{}, types.E_TYPE
	}
}

func listenerDescriptorEqual(left, right listener.Descriptor) bool {
	return listener.NormalizeProtocol(left.Protocol) == listener.NormalizeProtocol(right.Protocol) &&
		left.Port == right.Port &&
		left.IPv6 == right.IPv6 &&
		left.Path == right.Path
}

// notify(player, message [, no_flush [, no_newline]]) -> int
func builtinNotify(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}
	if hostOf(ctx).ConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}

	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	message := args[1].Str()

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
		enqueuePendingEffect(ctx, kernel.PendingEffect{
			Kind: kernel.PendingEffectNotification,
			Notification: kernel.PendingNotification{
				Player:  player,
				Message: message,
				NoFlush: noFlush,
			},
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

// listeners([find]) -> list of listener maps.
func builtinListeners(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Ok(types.NewList([]types.Value{}))
	}

	infos := cm.ListenerInfos()
	if len(args) == 1 {
		switch args[0].Type() {
		case types.TYPE_OBJ, types.TYPE_ANON:
			filtered := infos[:0]
			for _, info := range infos {
				if info.Object == args[0].ID() {
					filtered = append(filtered, info)
				}
			}
			infos = filtered
		case types.TYPE_INT:
			filtered := infos[:0]
			for _, info := range infos {
				if info.Port == args[0].Int() {
					filtered = append(filtered, info)
				}
			}
			infos = filtered
		case types.TYPE_MAP:
			desc, errCode := parseListenerDescriptorValue(args[0])
			if errCode != types.E_NONE {
				infos = nil
				break
			}
			filtered := infos[:0]
			for _, info := range infos {
				if listenerDescriptorEqual(listenerInfoDescriptor(info), desc) {
					filtered = append(filtered, info)
				}
			}
			infos = filtered
		default:
			infos = nil
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		leftProtocol := listener.NormalizeProtocol(infos[i].Protocol)
		rightProtocol := listener.NormalizeProtocol(infos[j].Protocol)
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
			{types.NewStr("protocol"), types.NewStr(string(listener.NormalizeProtocol(info.Protocol)))},
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
func builtinConnectedPlayers(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}

	showAll := false
	if len(args) == 1 {
		showAll = args[0].Truthy()
	}

	players := make([]types.ObjID, 0, 8)
	seen := make(map[types.ObjID]struct{}, 8)
	for _, p := range cm.ConnectedPlayers(showAll) {
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
func builtinConnectionName(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if hostOf(ctx).ConnManager == nil {
		return types.Err(types.E_INVARG)
	}

	player, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}

	hasMethod := false
	method := int64(0)
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		method = args[1].Int()
		hasMethod = true
	}

	conn := resolveConnection(ctx, player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	host, port := parseRemoteAddress(conn.RemoteAddr())
	name := host
	if resolved := conn.GetResolvedName(); resolved != "" {
		name = resolved
	}

	// Toast bf_connection_name (server.cc): with no method argument, the bare
	// resolved connection name; with method 1, the numeric IP; with ANY other
	// method value, the full legacy string
	// "port <listen-port> from <hostname> [<ip>], port <remote-port>".
	switch {
	case !hasMethod:
		return types.Ok(types.NewStr(name))
	case method == 1:
		return types.Ok(types.NewStr(host))
	default:
		listenPort, ok := conn.ListenerPort()
		if !ok {
			listenPort = int64(hostOf(ctx).ConnManager.GetListenPort())
		}
		return types.Ok(types.NewStr(fmt.Sprintf("port %d from %s [%s], port %s", listenPort, name, host, port)))
	}
}

// boot_player(player) -> int.
func builtinBootPlayer(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
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
		enqueuePendingEffect(ctx, kernel.PendingEffect{
			Kind:       kernel.PendingEffectBootPlayer,
			BootPlayer: player,
		})
		return types.Ok(types.NewInt(0))
	}
	if err := cm.BootPlayer(player); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

// switch_player(old_player, new_player [, silent]) -> none.
func builtinSwitchPlayer(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	if !isObjectRef(args[1]) {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}
	if resolveConnection(ctx, args[0].ID()) == nil {
		return types.Err(types.E_INVARG)
	}
	if ctx != nil && ctx.StoreTxn != nil {
		enqueuePendingEffect(ctx, kernel.PendingEffect{
			Kind: kernel.PendingEffectConnectionSwitch,
			ConnectionSwitch: kernel.PendingConnectionSwitch{
				OldPlayer: args[0].ID(),
				NewPlayer: args[1].ID(),
			},
		})
		return types.Ok(types.NewInt(0))
	}

	if err := cm.SwitchPlayer(args[0].ID(), args[1].ID()); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

// idle_seconds(player) -> int.
func builtinIdleSeconds(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if hostOf(ctx).ConnManager == nil {
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
func builtinConnectedSeconds(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if hostOf(ctx).ConnManager == nil {
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
func builtinConnectionInfo(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
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

	outbound := false
	remoteAddr := conn.RemoteAddr()
	sourceAddr := ""
	if info, ok := conn.(outboundConnectionInfo); ok && info.IsOutbound() {
		outbound = true
		sourceAddr = info.OutboundSourceAddr()
		remoteAddr = info.OutboundDestinationAddr()
	}

	host, portText := parseRemoteAddress(remoteAddr)
	destPort := int64(0)
	_, _ = fmt.Sscanf(portText, "%d", &destPort)
	sourcePort, listenerPortSet := conn.ListenerPort()
	sourceHost := "127.0.0.1"
	if !listenerPortSet {
		sourcePort = int64(cm.GetListenPort())
	}
	if sourceAddr != "" {
		sourceHost, portText = parseRemoteAddress(sourceAddr)
		_, _ = fmt.Sscanf(portText, "%d", &sourcePort)
	}

	protocol := "IPv4"
	if strings.Contains(host, ":") {
		protocol = "IPv6"
	}
	outboundInt := int64(0)
	if outbound {
		outboundInt = 1
	}

	result := types.NewMap([][2]types.Value{
		{types.NewStr("source_address"), types.NewStr(sourceHost)},
		{types.NewStr("source_ip"), types.NewStr(sourceHost)},
		{types.NewStr("source_port"), types.NewInt(sourcePort)},
		{types.NewStr("destination_address"), types.NewStr(host)},
		{types.NewStr("destination_ip"), types.NewStr(host)},
		{types.NewStr("destination_port"), types.NewInt(destPort)},
		{types.NewStr("protocol"), types.NewStr(protocol)},
		{types.NewStr("outbound"), types.NewInt(outboundInt)},
	})
	return types.Ok(result)
}

// connection_name_lookup(player [, rewrite]) -> int.
func builtinConnectionNameLookup(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
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

	name, err := cm.ConnectionNameLookup(player, rewrite)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewStr(name))
}

// set_connection_option(conn, option, value) -> int.
func builtinSetConnectionOption(ctx *Execution, args []types.Value) types.Result {
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

	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[1].Str()
	if !validConnectionOption(name) {
		return types.Err(types.E_INVARG)
	}
	if name == "keep-alive" {
		switch args[2].Type() {
		case types.TYPE_INT, types.TYPE_MAP:
		default:
			return types.Err(types.E_INVARG)
		}
	}
	if name == "intrinsic-commands" {
		if args[2].Truthy() {
			if args[2].Type() == types.TYPE_LIST {
				allowed := map[string]bool{
					".program":     true,
					"PREFIX":       true,
					"SUFFIX":       true,
					"OUTPUTPREFIX": true,
					"OUTPUTSUFFIX": true,
				}
				for i := 1; i <= args[2].Len(); i++ {
					elem := args[2].Get(i)
					if elem.Type() != types.TYPE_STR || !allowed[elem.Str()] {
						return types.Err(types.E_INVARG)
					}
				}
			} else {
				args[2] = defaultIntrinsicCommands()
			}
		}
	}

	ctx.Registry.setConnectionOption(player, name, args[2])
	if name == "binary" && args[2].Truthy() {
		if wakeConn, ok := conn.(inputWakeConnection); ok {
			wakeConn.WakeInputReader()
		}
	}
	if forcer := hostOf(ctx).InputForcer; name == "hold-input" && !args[2].Truthy() && forcer != nil {
		for _, line := range ctx.Registry.drainHeldCommands(player) {
			forcer.ForceInput(player, line, false)
		}
	}
	return types.Ok(types.NewInt(0))
}

// connection_option(conn, option) -> value.
func builtinConnectionOption(ctx *Execution, args []types.Value) types.Result {
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

	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[1].Str()
	if !validConnectionOption(name) {
		return types.Err(types.E_INVARG)
	}

	options := ctx.Registry.getConnectionOptions(player)
	value, ok := options[name]
	if !ok {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(value)
}

// read_http([type [, connection]]) -> map | E_PERM | E_ARGS | E_TYPE | E_INVARG.
func builtinReadHTTP(ctx *Execution, args []types.Value) types.Result {
	if len(args) == 0 {
		return types.Err(types.E_ARGS)
	}
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	typeStr := args[0].Str()

	var connection types.ObjID = ctx.Player
	if len(args) > 1 {
		if !isObjectRef(args[1]) {
			return types.Err(types.E_TYPE)
		}
		connection = args[1].ID()
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
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	if mgr.FindReadingTask(connection) != nil || ctx.Registry.HasPendingHTTPRead(connection) {
		return types.Err(types.E_INVARG)
	}

	t := ctx.Task
	if t == nil {
		return types.Err(types.E_INVARG)
	}

	value, complete := ctx.Registry.prepareHTTPRead(connection, typeStr, t)
	if complete {
		return types.Ok(value)
	}

	t.IsHTTPReadSuspended = true
	mgr.SuspendTask(t, -1)
	return types.Suspend(-1)
}
