package builtins

import (
	"sync"
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

func TestConnectionOptionsConcurrentReadWrite(t *testing.T) {
	player := types.ObjID(424242)
	connectionOptionState.mu.Lock()
	delete(connectionOptionState.byPlayer, player)
	connectionOptionState.mu.Unlock()
	t.Cleanup(func() {
		connectionOptionState.mu.Lock()
		delete(connectionOptionState.byPlayer, player)
		connectionOptionState.mu.Unlock()
	})

	setConnectionOption(player, "binary", types.NewInt(0))
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 10000; i++ {
			_ = getConnectionOptions(player)
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 10000; i++ {
			setConnectionOption(player, "binary", types.NewInt(int64(i&1)))
		}
	}()
	close(start)
	workers.Wait()
}

// ctxWithConnManager returns a task context whose registry has the given
// connection manager wired, mirroring how the server wires its registry.
func ctxWithConnManager(cm ConnectionManager) *kernel.TaskContext {
	r := NewRegistry()
	r.SetConnectionManager(cm)
	ctx := kernel.NewTaskContext()
	ctx.Registry = r
	return ctx
}

type stubConn struct {
	remote       string
	listenerPort int64
	sent         []string
	buffered     []string
	outbound     bool
	source       string
	destination  string
}

func (c *stubConn) Send(message string) error {
	c.sent = append(c.sent, message)
	return nil
}
func (c *stubConn) Buffer(message string) {
	c.buffered = append(c.buffered, message)
}
func (c *stubConn) Flush() error              { return nil }
func (c *stubConn) RemoteAddr() string        { return c.remote }
func (c *stubConn) GetOutputPrefix() string   { return "" }
func (c *stubConn) GetOutputSuffix() string   { return "" }
func (c *stubConn) BufferedOutputLength() int { return len(c.buffered) }
func (c *stubConn) ConnectedSeconds() int64   { return 0 }
func (c *stubConn) IdleSeconds() int64        { return 0 }
func (c *stubConn) GetResolvedName() string   { return "" }
func (c *stubConn) ListenerPort() int64       { return c.listenerPort }
func (c *stubConn) IsOutbound() bool          { return c.outbound }
func (c *stubConn) OutboundSourceAddr() string {
	return c.source
}
func (c *stubConn) OutboundDestinationAddr() string {
	return c.destination
}

type stubConnManager struct {
	conn        Connection
	listen      int
	infos       []ListenerInfo
	added       ListenerSpec
	removed     ListenerDescriptor
	boots       []types.ObjID
	switches    []stubSwitch
	switchedOld types.ObjID
	switchedNew types.ObjID
}

type stubSwitch struct {
	oldPlayer types.ObjID
	newPlayer types.ObjID
}

func (m *stubConnManager) GetConnection(player types.ObjID) Connection { return m.conn }
func (m *stubConnManager) ConnectedPlayers(showAll bool) []types.ObjID { return []types.ObjID{7} }
func (m *stubConnManager) BootPlayer(player types.ObjID) error {
	m.boots = append(m.boots, player)
	if m.conn != nil {
		_ = m.conn.Send("*** Disconnected ***")
	}
	return nil
}
func (m *stubConnManager) RecyclePlayer(player types.ObjID) error { return nil }
func (m *stubConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	m.switches = append(m.switches, stubSwitch{oldPlayer: oldPlayer, newPlayer: newPlayer})
	m.switchedOld = oldPlayer
	m.switchedNew = newPlayer
	return nil
}
func (m *stubConnManager) GetListenPort() int { return m.listen }
func (m *stubConnManager) ListenerInfos() []ListenerInfo {
	if m.infos != nil {
		return m.infos
	}
	return []ListenerInfo{{
		Object:   0,
		Port:     int64(m.listen),
		Protocol: ListenerProtocolTCP,
	}}
}
func (m *stubConnManager) AddListener(spec ListenerSpec) (ListenerDescriptor, error) {
	m.added = spec
	return ListenerDescriptor{
		Protocol: normalizeListenerProtocol(spec.Protocol),
		Port:     spec.Port,
		IPv6:     spec.IPv6,
		Path:     spec.Path,
	}, nil
}
func (m *stubConnManager) RemoveListener(desc ListenerDescriptor) error {
	m.removed = desc
	return nil
}
func (m *stubConnManager) OpenNetworkConnection(host string, port int64) (types.ObjID, error) {
	return types.ObjID(-8), nil
}
func (m *stubConnManager) ConnectionNameLookup(player types.ObjID, rewrite bool) (string, error) {
	return "lookup", nil
}

func TestNotifyDefersOutputUntilTransactionFlush(t *testing.T) {
	conn := &stubConn{}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(&stubConnManager{conn: conn})
	ctx.Player = 7
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("hello")})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent before flush = %#v, want none", conn.sent)
	}
	if len(ctx.PendingEffects) != 1 || ctx.PendingEffects[0].Kind != kernel.PendingEffectNotification {
		t.Fatalf("pending effects = %#v, want one notification", ctx.PendingEffects)
	}

	if errCode := FlushPendingEffects(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingEffects failed: %v", errCode)
	}
	if len(conn.sent) != 1 || conn.sent[0] != "hello" {
		t.Fatalf("sent after flush = %#v, want hello", conn.sent)
	}
	if len(ctx.PendingEffects) != 0 {
		t.Fatalf("pending effects after flush = %d, want 0", len(ctx.PendingEffects))
	}
}

func TestBufferedOutputLengthIncludesPendingNotifications(t *testing.T) {
	conn := &stubConn{}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(&stubConnManager{conn: conn})
	ctx.Player = -8
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	before := builtinBufferedOutputLength(ctx, []types.Value{types.NewObj(-8)})
	if before.IsError() {
		t.Fatalf("buffered_output_length before notify failed: %v", before.Error)
	}
	beforeValue := before.Val.Int()

	for i := 0; i < 5; i++ {
		res := builtinNotify(ctx, []types.Value{types.NewObj(-8), types.NewStr("buffered_output_length probe")})
		if res.IsError() {
			t.Fatalf("notify %d failed: %v", i, res.Error)
		}
	}

	after := builtinBufferedOutputLength(ctx, []types.Value{types.NewObj(-8)})
	if after.IsError() {
		t.Fatalf("buffered_output_length after notify failed: %v", after.Error)
	}
	afterValue := after.Val.Int()
	if afterValue <= beforeValue {
		t.Fatalf("buffered_output_length after pending notifications = %d, want > %d", afterValue, beforeValue)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent before transaction flush = %#v, want none", conn.sent)
	}
}

func TestNotifyDefersNoFlushBufferUntilTransactionFlush(t *testing.T) {
	conn := &stubConn{}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(&stubConnManager{conn: conn})
	ctx.Player = 7
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("held"), types.NewInt(1)})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}
	if len(conn.buffered) != 0 {
		t.Fatalf("buffered before flush = %#v, want none", conn.buffered)
	}

	if errCode := FlushPendingEffects(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingEffects failed: %v", errCode)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent after no-flush notify = %#v, want none", conn.sent)
	}
	if len(conn.buffered) != 1 || conn.buffered[0] != "held" {
		t.Fatalf("buffered after flush = %#v, want held", conn.buffered)
	}
}

func TestDiscardPendingNotificationsDropsDeferredNotify(t *testing.T) {
	conn := &stubConn{}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(&stubConnManager{conn: conn})
	ctx.Player = 7
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("discard")})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}

	DiscardPendingEffects(ctx)
	if len(ctx.PendingEffects) != 0 {
		t.Fatalf("pending effects after discard = %d, want 0", len(ctx.PendingEffects))
	}
	if len(conn.sent) != 0 || len(conn.buffered) != 0 {
		t.Fatalf("connection output after discard sent=%#v buffered=%#v, want none", conn.sent, conn.buffered)
	}
}

func TestBootPlayerDefersUntilAfterNotifications(t *testing.T) {
	conn := &stubConn{}
	manager := &stubConnManager{conn: conn}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(manager)
	ctx.Player = 7
	ctx.Programmer = 7
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("before")})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}
	res = builtinBootPlayer(ctx, []types.Value{types.NewObj(7)})
	if res.IsError() {
		t.Fatalf("boot_player failed: %v", res.Error)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent before flush = %#v, want none", conn.sent)
	}
	if len(manager.boots) != 0 {
		t.Fatalf("boots before flush = %#v, want none", manager.boots)
	}

	if errCode := FlushPendingEffects(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingEffects failed: %v", errCode)
	}
	if len(manager.boots) != 1 || manager.boots[0] != 7 {
		t.Fatalf("boots after flush = %#v, want [7]", manager.boots)
	}
	if len(conn.sent) != 2 || conn.sent[0] != "before" || conn.sent[1] != "*** Disconnected ***" {
		t.Fatalf("sent after flush = %#v, want notify then disconnect", conn.sent)
	}
}

func TestSwitchPlayerDefersUntilTransactionFlush(t *testing.T) {
	manager := &stubConnManager{conn: &stubConn{}}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinSwitchPlayer(ctx, []types.Value{types.NewObj(7), types.NewObj(8)})
	if res.IsError() {
		t.Fatalf("switch_player failed: %v", res.Error)
	}
	if len(manager.switches) != 0 {
		t.Fatalf("switches before flush = %#v, want none", manager.switches)
	}
	if len(ctx.PendingEffects) != 1 || ctx.PendingEffects[0].Kind != kernel.PendingEffectConnectionSwitch {
		t.Fatalf("pending effects = %#v, want one switch", ctx.PendingEffects)
	}

	if errCode := FlushPendingEffects(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingEffects failed: %v", errCode)
	}
	if len(manager.switches) != 1 {
		t.Fatalf("switches after flush = %#v, want one", manager.switches)
	}
	if manager.switches[0].oldPlayer != 7 || manager.switches[0].newPlayer != 8 {
		t.Fatalf("switch = %#v, want 7->8", manager.switches[0])
	}
	if len(ctx.PendingEffects) != 0 {
		t.Fatalf("pending effects after flush = %d, want 0", len(ctx.PendingEffects))
	}
}

func TestDiscardPendingConnectionSwitchesDropsDeferredSwitch(t *testing.T) {
	manager := &stubConnManager{conn: &stubConn{}}
	store := dbstore.NewStore()
	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinSwitchPlayer(ctx, []types.Value{types.NewObj(7), types.NewObj(8)})
	if res.IsError() {
		t.Fatalf("switch_player failed: %v", res.Error)
	}

	DiscardPendingEffects(ctx)
	if len(ctx.PendingEffects) != 0 {
		t.Fatalf("pending effects after discard = %d, want 0", len(ctx.PendingEffects))
	}
	if len(manager.switches) != 0 {
		t.Fatalf("switches after discard = %#v, want none", manager.switches)
	}
}

func TestSwitchPlayerReturnsNoValueOnSuccess(t *testing.T) {
	// switch_player requires the old player to have a live connection (it swaps
	// the player bound to that connection), so the stub must resolve one.
	manager := &stubConnManager{conn: &stubConn{}}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinSwitchPlayer(ctx, []types.Value{
		types.NewObj(2),
		types.NewObj(3),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_INT {
		t.Fatalf("got %T, want int no-value representation", res.Val)
	}
	if res.Val.Int() != 0 {
		t.Fatalf("got %d, want 0", res.Val.Int())
	}
	if manager.switchedOld != 2 || manager.switchedNew != 3 {
		t.Fatalf("switch called with (%d, %d), want (2, 3)", manager.switchedOld, manager.switchedNew)
	}
}

func TestConnectionNameFormats(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{
		conn:   &stubConn{remote: "[::1]:4567", listenerPort: 7777},
		listen: 7777,
	})
	ctx.Player = 7

	// Contract per Toast bf_connection_name (server.cc), verified live
	// 2026-07-01 and captured in conformance connection_name_semantics.yaml:
	// no method → bare resolved name; method 1 → numeric IP; any other
	// method → "port <listen-port> from <host> [<ip>], port <remote-port>".
	cases := []struct {
		name string
		args []types.Value
		want string
	}{
		{
			name: "no_method_bare_name",
			args: []types.Value{types.NewObj(7)},
			want: "::1",
		},
		{
			name: "method_1_numeric_ip",
			args: []types.Value{types.NewObj(7), types.NewInt(1)},
			want: "::1",
		},
		{
			name: "method_0_full_legacy",
			args: []types.Value{types.NewObj(7), types.NewInt(0)},
			want: "port 7777 from ::1 [::1], port 4567",
		},
		{
			name: "method_2_full_legacy",
			args: []types.Value{types.NewObj(7), types.NewInt(2)},
			want: "port 7777 from ::1 [::1], port 4567",
		},
		{
			name: "method_negative_full_legacy",
			args: []types.Value{types.NewObj(7), types.NewInt(-1)},
			want: "port 7777 from ::1 [::1], port 4567",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := builtinConnectionName(ctx, tc.args)
			if res.IsError() {
				t.Fatalf("unexpected error: %v", res.Error)
			}
			if res.Val.Type() != types.TYPE_STR {
				t.Fatalf("expected string result, got %T", res.Val)
			}
			if res.Val.Str() != tc.want {
				t.Fatalf("got %q, want %q", res.Val.Str(), tc.want)
			}
		})
	}
}

func TestListenBuildsListenerSpecFromOptions(t *testing.T) {
	manager := &stubConnManager{}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8888),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("tcp")},
			{types.NewStr("interface"), types.NewStr("127.0.0.1")},
			{types.NewStr("ipv6"), types.NewInt(0)},
			{types.NewStr("print-messages"), types.NewInt(1)},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_INT || res.Val.Int() != 8888 {
		t.Fatalf("got %v (%T), want TCP port 8888", res.Val, res.Val)
	}
	if manager.added.Object != 42 ||
		manager.added.Port != 8888 ||
		manager.added.Protocol != ListenerProtocolTCP ||
		manager.added.Interface != "127.0.0.1" ||
		manager.added.IPv6 ||
		!manager.added.PrintMessages {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestListenBuildsIPv6ListenerSpec(t *testing.T) {
	manager := &stubConnManager{}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8888),
		types.NewMap([][2]types.Value{
			{types.NewStr("ipv6"), types.NewInt(1)},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_MAP {
		t.Fatalf("got %T, want descriptor map", res.Val)
	}
	ipv6, _ := res.Val.MapGet(types.NewStr("ipv6"))
	if ipv6.Int() != 1 || !manager.added.IPv6 {
		t.Fatalf("unexpected ipv6 descriptor/spec: %s %+v", res.Val.String(), manager.added)
	}
}

func TestListenBuildsTLSListenerSpec(t *testing.T) {
	manager := &stubConnManager{}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8889),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr(ListenerProtocolTLS)},
			{types.NewStr("certificate"), types.NewStr("server.crt")},
			{types.NewStr("key"), types.NewStr("server.key")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_MAP {
		t.Fatalf("got %T, want descriptor map", res.Val)
	}
	desc := res.Val
	protocol, _ := desc.MapGet(types.NewStr("protocol"))
	port, _ := desc.MapGet(types.NewStr("port"))
	if protocol.Str() != ListenerProtocolTLS || port.Int() != 8889 {
		t.Fatalf("unexpected descriptor: %s", desc.String())
	}
	if manager.added.Protocol != ListenerProtocolTLS ||
		manager.added.TLSCertificatePath != "server.crt" ||
		manager.added.TLSKeyPath != "server.key" {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestListenBuildsWebSocketListenerSpec(t *testing.T) {
	manager := &stubConnManager{}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8890),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr(ListenerProtocolWebSocket)},
			{types.NewStr("path"), types.NewStr("/moo")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_MAP {
		t.Fatalf("got %T, want descriptor map", res.Val)
	}
	desc := res.Val
	protocol, _ := desc.MapGet(types.NewStr("protocol"))
	port, _ := desc.MapGet(types.NewStr("port"))
	path, _ := desc.MapGet(types.NewStr("path"))
	if protocol.Str() != ListenerProtocolWebSocket ||
		port.Int() != 8890 ||
		path.Str() != "/moo" {
		t.Fatalf("unexpected descriptor: %s", desc.String())
	}
	if manager.added.Protocol != ListenerProtocolWebSocket || manager.added.Path != "/moo" {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestUnlistenAcceptsListenerDescriptorMap(t *testing.T) {
	manager := &stubConnManager{}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinUnlisten(ctx, []types.Value{
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr(ListenerProtocolWebSocket)},
			{types.NewStr("port"), types.NewInt(8888)},
			{types.NewStr("path"), types.NewStr("/moo")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	want := ListenerDescriptor{Protocol: ListenerProtocolWebSocket, Port: 8888, Path: "/moo"}
	if !listenerDescriptorEqual(manager.removed, want) {
		t.Fatalf("removed %+v, want %+v", manager.removed, want)
	}
}

func TestUnlistenSecondArgumentSelectsIPv6Descriptor(t *testing.T) {
	manager := &stubConnManager{}

	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true

	res := builtinUnlisten(ctx, []types.Value{types.NewInt(8888), types.NewInt(1)})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	want := ListenerDescriptor{Protocol: ListenerProtocolTCP, Port: 8888, IPv6: true}
	if !listenerDescriptorEqual(manager.removed, want) {
		t.Fatalf("removed %+v, want %+v", manager.removed, want)
	}
}

func TestConnectionInfoUsesOutboundEndpointMetadata(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{
		conn: &stubConn{
			remote:      "127.0.0.1:60000",
			outbound:    true,
			source:      "127.0.0.1:60000",
			destination: "127.0.0.1:8888",
		},
		listen: 7777,
	})

	res := builtinConnectionInfo(ctx, []types.Value{types.NewObj(-1)})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	info := res.Val
	outbound, _ := info.MapGet(types.NewStr("outbound"))
	destinationPort, _ := info.MapGet(types.NewStr("destination_port"))
	sourcePort, _ := info.MapGet(types.NewStr("source_port"))
	if outbound.Int() != 1 || destinationPort.Int() != 8888 || sourcePort.Int() != 60000 {
		t.Fatalf("unexpected outbound info: %s", info.String())
	}
}

func TestListenersIncludesProtocolMetadataAndFiltersByDescriptor(t *testing.T) {
	manager := &stubConnManager{
		infos: []ListenerInfo{
			{
				Object:        5,
				Port:          8888,
				Protocol:      ListenerProtocolWebSocket,
				Path:          "/moo",
				PrintMessages: true,
			},
			{
				Object:   6,
				Port:     8888,
				Protocol: ListenerProtocolTCP,
			},
		},
	}

	ctx := ctxWithConnManager(manager)
	res := builtinListeners(ctx, []types.Value{
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr(ListenerProtocolWebSocket)},
			{types.NewStr("port"), types.NewInt(8888)},
			{types.NewStr("path"), types.NewStr("/moo")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_LIST {
		t.Fatalf("got %T, want list", res.Val)
	}
	list := res.Val
	if list.Len() != 1 {
		t.Fatalf("got %d entries, want 1", list.Len())
	}
	if list.Get(1).Type() != types.TYPE_MAP {
		t.Fatalf("got %T, want map", list.Get(1))
	}
	entry := list.Get(1)
	protocol, _ := entry.MapGet(types.NewStr("protocol"))
	path, _ := entry.MapGet(types.NewStr("path"))
	tlsValue, _ := entry.MapGet(types.NewStr("TLS"))
	if protocol.Str() != ListenerProtocolWebSocket ||
		path.Str() != "/moo" ||
		tlsValue.Int() != 0 {
		t.Fatalf("unexpected listener entry: %s", entry.String())
	}
}
