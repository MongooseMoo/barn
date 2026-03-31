package builtins

import (
	"barn/types"
	"bytes"
	"errors"
	"testing"
)

type stubConn struct {
	remote         string
	lastLine       string
	lastBytes      []byte
	keepAliveCalls []bool
	keepAliveErr   error
}

func (c *stubConn) Send(message string) error { c.lastLine = message; return nil }
func (c *stubConn) SendBytes(data []byte) error {
	c.lastBytes = append([]byte(nil), data...)
	return nil
}
func (c *stubConn) SetKeepAlive(enabled bool) error {
	c.keepAliveCalls = append(c.keepAliveCalls, enabled)
	return c.keepAliveErr
}
func (c *stubConn) Buffer(message string)     {}
func (c *stubConn) Flush() error              { return nil }
func (c *stubConn) RemoteAddr() string        { return c.remote }
func (c *stubConn) GetOutputPrefix() string   { return "" }
func (c *stubConn) GetOutputSuffix() string   { return "" }
func (c *stubConn) BufferedOutputLength() int { return 0 }
func (c *stubConn) ConnectedSeconds() int64   { return 0 }
func (c *stubConn) IdleSeconds() int64        { return 0 }

type stubConnManager struct {
	conn         Connection
	listen       int
	connected    []types.ObjID
	listeners    []ListenerInfo
	listenCalls  []listenCall
	unlistenCall []int
	openCalls    []openCall
	listenErr    error
	unlistenErr  error
	openConnID   types.ObjID
	openErr      error
}

type listenCall struct {
	listenerObj types.ObjID
	port        int
}

type openCall struct {
	host string
	port int
}

func (m *stubConnManager) GetConnection(player types.ObjID) Connection { return m.conn }
func (m *stubConnManager) ConnectedPlayers(showAll bool) []types.ObjID {
	if len(m.connected) == 0 {
		return []types.ObjID{7}
	}
	return append([]types.ObjID(nil), m.connected...)
}
func (m *stubConnManager) BootPlayer(player types.ObjID) error { return nil }
func (m *stubConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	return nil
}
func (m *stubConnManager) GetListenPort() int { return m.listen }
func (m *stubConnManager) ListListeners() []ListenerInfo {
	if len(m.listeners) > 0 {
		out := make([]ListenerInfo, len(m.listeners))
		copy(out, m.listeners)
		return out
	}
	return []ListenerInfo{{Object: 0, Port: m.listen}}
}
func (m *stubConnManager) ListenOnPort(listenerObj types.ObjID, port int) error {
	m.listenCalls = append(m.listenCalls, listenCall{
		listenerObj: listenerObj,
		port:        port,
	})
	if m.listenErr != nil {
		return m.listenErr
	}
	m.listeners = append(m.listeners, ListenerInfo{Object: listenerObj, Port: port})
	return nil
}
func (m *stubConnManager) UnlistenPort(port int) error {
	m.unlistenCall = append(m.unlistenCall, port)
	return m.unlistenErr
}
func (m *stubConnManager) OpenNetworkConnection(host string, port int) (types.ObjID, error) {
	m.openCalls = append(m.openCalls, openCall{host: host, port: port})
	if m.openErr != nil {
		return types.ObjNothing, m.openErr
	}
	if m.openConnID == 0 {
		return types.ObjID(-2), nil
	}
	return m.openConnID, nil
}

type forcedInput struct {
	player  types.ObjID
	line    string
	atFront bool
}

type stubInputForcer struct {
	forced []forcedInput
}

func (f *stubInputForcer) ForceInput(player types.ObjID, line string, atFront bool) {
	f.forced = append(f.forced, forcedInput{
		player:  player,
		line:    line,
		atFront: atFront,
	})
}

func withNetworkTestState(t *testing.T) {
	t.Helper()

	prevConn := globalConnManager
	prevForcer := globalInputForcer

	connectionOptionState.mu.Lock()
	prevOptions := connectionOptionState.byPlayer
	connectionOptionState.byPlayer = make(map[types.ObjID]map[string]types.Value)
	connectionOptionState.mu.Unlock()

	heldInputState.mu.Lock()
	prevHeld := heldInputState.byPlayer
	heldInputState.byPlayer = make(map[types.ObjID][]string)
	heldInputState.mu.Unlock()

	t.Cleanup(func() {
		globalConnManager = prevConn
		globalInputForcer = prevForcer

		connectionOptionState.mu.Lock()
		connectionOptionState.byPlayer = prevOptions
		connectionOptionState.mu.Unlock()

		heldInputState.mu.Lock()
		heldInputState.byPlayer = prevHeld
		heldInputState.mu.Unlock()
	})
}

func TestConnectionNameFormats(t *testing.T) {
	withNetworkTestState(t)

	globalConnManager = &stubConnManager{
		conn:   &stubConn{remote: "[::1]:4567"},
		listen: 7777,
	}

	ctx := types.NewTaskContext()
	ctx.Player = 7

	cases := []struct {
		name string
		args []types.Value
		want string
	}{
		{
			name: "method_0_legacy",
			args: []types.Value{types.NewObj(7)},
			want: "port 7777 from ::1, port 4567",
		},
		{
			name: "method_1_host_only",
			args: []types.Value{types.NewObj(7), types.NewInt(1)},
			want: "::1",
		},
		{
			name: "method_2_host_port",
			args: []types.Value{types.NewObj(7), types.NewInt(2)},
			want: "::1, port 4567",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := builtinConnectionName(ctx, tc.args)
			if res.IsError() {
				t.Fatalf("unexpected error: %v", res.Error)
			}
			got, ok := res.Val.(types.StrValue)
			if !ok {
				t.Fatalf("expected string result, got %T", res.Val)
			}
			if got.Value() != tc.want {
				t.Fatalf("got %q, want %q", got.Value(), tc.want)
			}
		})
	}
}

func TestNotifyBinaryModeDecodesBinaryEscapes(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{remote: "127.0.0.1:1234"}
	globalConnManager = &stubConnManager{conn: conn, listen: 7777}
	setConnectionOption(7, "binary", types.NewInt(1))

	ctx := types.NewTaskContext()
	ctx.Player = 7

	res := builtinNotify(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("~FF~FBF"),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}

	want := []byte{0xFF, 0xFB, 'F', '\r', '\n'}
	if !bytes.Equal(conn.lastBytes, want) {
		t.Fatalf("got bytes %v, want %v", conn.lastBytes, want)
	}
}

func TestNotifyBinaryModeInvalidBinaryStringReturnsEInvarg(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{remote: "127.0.0.1:1234"}
	globalConnManager = &stubConnManager{conn: conn, listen: 7777}
	setConnectionOption(7, "binary", types.NewInt(1))

	ctx := types.NewTaskContext()
	ctx.Player = 7

	res := builtinNotify(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("~F"),
	})
	if !res.IsError() || res.Error != types.E_INVARG {
		t.Fatalf("got result %+v, want E_INVARG", res)
	}
	if len(conn.lastBytes) != 0 {
		t.Fatalf("expected no bytes sent on malformed binary input, got %v", conn.lastBytes)
	}
}

func TestSetConnectionOptionClientEchoNegotiation(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{remote: "127.0.0.1:1234"}
	globalConnManager = &stubConnManager{conn: conn, listen: 7777}

	ctx := types.NewTaskContext()
	ctx.Player = 7

	disableEcho := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("client-echo"),
		types.NewInt(0),
	})
	if disableEcho.IsError() {
		t.Fatalf("unexpected error setting client-echo=0: %v", disableEcho.Error)
	}
	if !bytes.Equal(conn.lastBytes, []byte{255, 251, 1}) {
		t.Fatalf("expected IAC WILL ECHO sequence, got %v", conn.lastBytes)
	}

	enableClientEcho := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("client-echo"),
		types.NewInt(1),
	})
	if enableClientEcho.IsError() {
		t.Fatalf("unexpected error setting client-echo=1: %v", enableClientEcho.Error)
	}
	if !bytes.Equal(conn.lastBytes, []byte{255, 252, 1}) {
		t.Fatalf("expected IAC WONT ECHO sequence, got %v", conn.lastBytes)
	}
}

func TestSetConnectionOptionKeepAliveApplied(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{remote: "127.0.0.1:1234"}
	globalConnManager = &stubConnManager{conn: conn, listen: 7777}

	ctx := types.NewTaskContext()
	ctx.Player = 7

	enable := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("keep-alive"),
		types.NewInt(1),
	})
	if enable.IsError() {
		t.Fatalf("unexpected error setting keep-alive=1: %v", enable.Error)
	}

	disable := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("keep-alive"),
		types.NewInt(0),
	})
	if disable.IsError() {
		t.Fatalf("unexpected error setting keep-alive=0: %v", disable.Error)
	}

	if len(conn.keepAliveCalls) != 2 {
		t.Fatalf("expected 2 keep-alive calls, got %d", len(conn.keepAliveCalls))
	}
	if !conn.keepAliveCalls[0] || conn.keepAliveCalls[1] {
		t.Fatalf("expected keep-alive toggles [true,false], got %v", conn.keepAliveCalls)
	}
}

func TestSetConnectionOptionKeepAliveErrorReturnsEInvarg(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{
		remote:       "127.0.0.1:1234",
		keepAliveErr: errors.New("keepalive failed"),
	}
	globalConnManager = &stubConnManager{conn: conn, listen: 7777}

	ctx := types.NewTaskContext()
	ctx.Player = 7

	res := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("keep-alive"),
		types.NewInt(1),
	})
	if !res.IsError() || res.Error != types.E_INVARG {
		t.Fatalf("expected E_INVARG from keep-alive failure, got %+v", res)
	}
}

func TestHoldInputReleaseReplaysQueuedLines(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{remote: "127.0.0.1:1234"}
	forcer := &stubInputForcer{}
	globalConnManager = &stubConnManager{conn: conn, listen: 7777}
	globalInputForcer = forcer

	ctx := types.NewTaskContext()
	ctx.Player = 7

	enableHold := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("hold-input"),
		types.NewInt(1),
	})
	if enableHold.IsError() {
		t.Fatalf("unexpected error enabling hold-input: %v", enableHold.Error)
	}
	if !ShouldHoldInput(7) {
		t.Fatal("expected hold-input option to be enabled")
	}

	QueueHeldInput(7, "look")
	QueueHeldInput(7, "say hi")

	disableHold := builtinSetConnectionOption(ctx, []types.Value{
		types.NewObj(7),
		types.NewStr("hold-input"),
		types.NewInt(0),
	})
	if disableHold.IsError() {
		t.Fatalf("unexpected error disabling hold-input: %v", disableHold.Error)
	}
	if ShouldHoldInput(7) {
		t.Fatal("expected hold-input option to be disabled")
	}

	if len(forcer.forced) != 2 {
		t.Fatalf("expected 2 replayed lines, got %d", len(forcer.forced))
	}
	if forcer.forced[0].line != "look" || forcer.forced[1].line != "say hi" {
		t.Fatalf("held-input replay order mismatch: %+v", forcer.forced)
	}
	if len(DrainHeldInput(7)) != 0 {
		t.Fatal("expected held-input queue to be empty after replay")
	}
}

func TestFlushInputClearsHeldInputQueue(t *testing.T) {
	withNetworkTestState(t)

	QueueHeldInput(7, "alpha")
	QueueHeldInput(7, "beta")

	ctx := types.NewTaskContext()
	ctx.Player = 7

	res := builtinFlushInput(ctx, []types.Value{types.NewObj(7)})
	if res.IsError() {
		t.Fatalf("unexpected error from flush_input: %v", res.Error)
	}
	count, ok := res.Val.(types.IntValue)
	if !ok {
		t.Fatalf("expected integer count, got %T", res.Val)
	}
	if count.Val != 2 {
		t.Fatalf("expected flush_input count 2, got %d", count.Val)
	}

	res2 := builtinFlushInput(ctx, []types.Value{types.NewObj(7)})
	if res2.IsError() {
		t.Fatalf("unexpected error from second flush_input: %v", res2.Error)
	}
	count2 := res2.Val.(types.IntValue)
	if count2.Val != 0 {
		t.Fatalf("expected second flush_input count 0, got %d", count2.Val)
	}
}

func TestListenUnlistenAndOpenNetworkConnectionUseConnectionManager(t *testing.T) {
	withNetworkTestState(t)

	conn := &stubConn{remote: "127.0.0.1:1234"}
	manager := &stubConnManager{
		conn:       conn,
		listen:     7777,
		openConnID: types.ObjID(-42),
		listeners: []ListenerInfo{
			{Object: 5, Port: 9002},
		},
	}
	globalConnManager = manager

	ctx := types.NewTaskContext()
	ctx.Player = 1
	ctx.IsWizard = true

	listenRes := builtinListen(ctx, []types.Value{
		types.NewObj(5),
		types.NewInt(9001),
	})
	if listenRes.IsError() {
		t.Fatalf("unexpected listen() error: %v", listenRes.Error)
	}
	if len(manager.listenCalls) != 1 || manager.listenCalls[0].listenerObj != 5 || manager.listenCalls[0].port != 9001 {
		t.Fatalf("listen call not forwarded correctly: %+v", manager.listenCalls)
	}

	unlistenByPort := builtinUnlisten(ctx, []types.Value{
		types.NewInt(9001),
	})
	if unlistenByPort.IsError() {
		t.Fatalf("unexpected unlisten(port) error: %v", unlistenByPort.Error)
	}
	if len(manager.unlistenCall) != 1 || manager.unlistenCall[0] != 9001 {
		t.Fatalf("unlisten(port) call not forwarded correctly: %+v", manager.unlistenCall)
	}

	unlistenByObj := builtinUnlisten(ctx, []types.Value{
		types.NewObj(5),
	})
	if unlistenByObj.IsError() {
		t.Fatalf("unexpected unlisten(object) error: %v", unlistenByObj.Error)
	}
	if len(manager.unlistenCall) != 2 || manager.unlistenCall[1] != 9002 {
		t.Fatalf("unlisten(object) did not use listener metadata: %+v", manager.unlistenCall)
	}

	openRes := builtinOpenNetworkConnection(ctx, []types.Value{
		types.NewStr("example.com"),
		types.NewInt(7778),
	})
	if openRes.IsError() {
		t.Fatalf("unexpected open_network_connection() error: %v", openRes.Error)
	}
	openObj, ok := openRes.Val.(types.ObjValue)
	if !ok {
		t.Fatalf("expected object result from open_network_connection(), got %T", openRes.Val)
	}
	if openObj.ID() != -42 {
		t.Fatalf("expected returned connection object #-42, got #%d", openObj.ID())
	}
	if len(manager.openCalls) != 1 || manager.openCalls[0].host != "example.com" || manager.openCalls[0].port != 7778 {
		t.Fatalf("open_network_connection call not forwarded correctly: %+v", manager.openCalls)
	}
}

func TestListenersUsesConnectionManagerMetadataAndFilters(t *testing.T) {
	withNetworkTestState(t)

	globalConnManager = &stubConnManager{
		conn:   &stubConn{remote: "127.0.0.1:1234"},
		listen: 7777,
		listeners: []ListenerInfo{
			{Object: 5, Port: 7777},
			{Object: 9, Port: 8888},
		},
	}

	ctx := types.NewTaskContext()

	all := builtinListeners(ctx, nil)
	if all.IsError() {
		t.Fatalf("unexpected listeners() error: %v", all.Error)
	}
	allList := all.Val.(types.ListValue)
	if allList.Len() != 2 {
		t.Fatalf("expected 2 listeners, got %d", allList.Len())
	}

	byObj := builtinListeners(ctx, []types.Value{types.NewObj(5)})
	if byObj.IsError() {
		t.Fatalf("unexpected listeners(#5) error: %v", byObj.Error)
	}
	byObjList := byObj.Val.(types.ListValue)
	if byObjList.Len() != 1 {
		t.Fatalf("expected 1 listener for #5, got %d", byObjList.Len())
	}
	entry := byObjList.Get(1).(types.MapValue)
	objVal, _ := entry.Get(types.NewStr("object"))
	portVal, _ := entry.Get(types.NewStr("port"))
	if objVal.(types.ObjValue).ID() != 5 || portVal.(types.IntValue).Val != 7777 {
		t.Fatalf("unexpected listener map for #5: %v", entry.String())
	}

	byPort := builtinListeners(ctx, []types.Value{types.NewInt(8888)})
	if byPort.IsError() {
		t.Fatalf("unexpected listeners(8888) error: %v", byPort.Error)
	}
	byPortList := byPort.Val.(types.ListValue)
	if byPortList.Len() != 1 {
		t.Fatalf("expected 1 listener for port 8888, got %d", byPortList.Len())
	}
	portEntry := byPortList.Get(1).(types.MapValue)
	objVal, _ = portEntry.Get(types.NewStr("object"))
	if objVal.(types.ObjValue).ID() != 9 {
		t.Fatalf("unexpected listener object for port 8888: %v", portEntry.String())
	}
}
