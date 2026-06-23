package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

type stubConn struct {
	remote       string
	listenerPort int64
	sent         []string
	buffered     []string
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

type stubConnManager struct {
	conn     Connection
	listen   int
	infos    []ListenerInfo
	added    ListenerSpec
	removed  ListenerDescriptor
	boots    []types.ObjID
	switches []stubSwitch
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
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	conn := &stubConn{}
	globalConnManager = &stubConnManager{conn: conn}

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.Player = 7
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("hello")})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent before flush = %#v, want none", conn.sent)
	}
	if len(ctx.PendingNotifications) != 1 {
		t.Fatalf("pending notifications = %d, want 1", len(ctx.PendingNotifications))
	}

	if errCode := FlushPendingNotifications(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingNotifications failed: %v", errCode)
	}
	if len(conn.sent) != 1 || conn.sent[0] != "hello" {
		t.Fatalf("sent after flush = %#v, want hello", conn.sent)
	}
	if len(ctx.PendingNotifications) != 0 {
		t.Fatalf("pending notifications after flush = %d, want 0", len(ctx.PendingNotifications))
	}
}

func TestBufferedOutputLengthIncludesPendingNotifications(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	conn := &stubConn{}
	globalConnManager = &stubConnManager{conn: conn}

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.Player = -8
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	before := builtinBufferedOutputLength(ctx, []types.Value{types.NewObj(-8)})
	if before.IsError() {
		t.Fatalf("buffered_output_length before notify failed: %v", before.Error)
	}
	beforeValue := before.Val.(types.IntValue).Val

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
	afterValue := after.Val.(types.IntValue).Val
	if afterValue <= beforeValue {
		t.Fatalf("buffered_output_length after pending notifications = %d, want > %d", afterValue, beforeValue)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent before transaction flush = %#v, want none", conn.sent)
	}
}

func TestNotifyDefersNoFlushBufferUntilTransactionFlush(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	conn := &stubConn{}
	globalConnManager = &stubConnManager{conn: conn}

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.Player = 7
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("held"), types.NewInt(1)})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}
	if len(conn.buffered) != 0 {
		t.Fatalf("buffered before flush = %#v, want none", conn.buffered)
	}

	if errCode := FlushPendingNotifications(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingNotifications failed: %v", errCode)
	}
	if len(conn.sent) != 0 {
		t.Fatalf("sent after no-flush notify = %#v, want none", conn.sent)
	}
	if len(conn.buffered) != 1 || conn.buffered[0] != "held" {
		t.Fatalf("buffered after flush = %#v, want held", conn.buffered)
	}
}

func TestDiscardPendingNotificationsDropsDeferredNotify(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	conn := &stubConn{}
	globalConnManager = &stubConnManager{conn: conn}

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.Player = 7
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinNotify(ctx, []types.Value{types.NewObj(7), types.NewStr("discard")})
	if res.IsError() {
		t.Fatalf("notify failed: %v", res.Error)
	}

	DiscardPendingNotifications(ctx)
	if len(ctx.PendingNotifications) != 0 {
		t.Fatalf("pending notifications after discard = %d, want 0", len(ctx.PendingNotifications))
	}
	if len(conn.sent) != 0 || len(conn.buffered) != 0 {
		t.Fatalf("connection output after discard sent=%#v buffered=%#v, want none", conn.sent, conn.buffered)
	}
}

func TestBootPlayerDefersUntilAfterNotifications(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	conn := &stubConn{}
	manager := &stubConnManager{conn: conn}
	globalConnManager = manager

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
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

	if errCode := FlushPendingNotifications(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingNotifications failed: %v", errCode)
	}
	if errCode := FlushPendingBootPlayers(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingBootPlayers failed: %v", errCode)
	}
	if len(manager.boots) != 1 || manager.boots[0] != 7 {
		t.Fatalf("boots after flush = %#v, want [7]", manager.boots)
	}
	if len(conn.sent) != 2 || conn.sent[0] != "before" || conn.sent[1] != "*** Disconnected ***" {
		t.Fatalf("sent after flush = %#v, want notify then disconnect", conn.sent)
	}
}

func TestSwitchPlayerDefersUntilTransactionFlush(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{conn: &stubConn{}}
	globalConnManager = manager

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinSwitchPlayer(ctx, []types.Value{types.NewObj(7), types.NewObj(8)})
	if res.IsError() {
		t.Fatalf("switch_player failed: %v", res.Error)
	}
	if len(manager.switches) != 0 {
		t.Fatalf("switches before flush = %#v, want none", manager.switches)
	}
	if len(ctx.PendingConnectionSwitches) != 1 {
		t.Fatalf("pending switches = %d, want 1", len(ctx.PendingConnectionSwitches))
	}

	if errCode := FlushPendingConnectionSwitches(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingConnectionSwitches failed: %v", errCode)
	}
	if len(manager.switches) != 1 {
		t.Fatalf("switches after flush = %#v, want one", manager.switches)
	}
	if manager.switches[0].oldPlayer != 7 || manager.switches[0].newPlayer != 8 {
		t.Fatalf("switch = %#v, want 7->8", manager.switches[0])
	}
	if len(ctx.PendingConnectionSwitches) != 0 {
		t.Fatalf("pending switches after flush = %d, want 0", len(ctx.PendingConnectionSwitches))
	}
}

func TestDiscardPendingConnectionSwitchesDropsDeferredSwitch(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{conn: &stubConn{}}
	globalConnManager = manager

	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.StoreTxn = store.BeginReadOnly(0)

	res := builtinSwitchPlayer(ctx, []types.Value{types.NewObj(7), types.NewObj(8)})
	if res.IsError() {
		t.Fatalf("switch_player failed: %v", res.Error)
	}

	DiscardPendingConnectionSwitches(ctx)
	if len(ctx.PendingConnectionSwitches) != 0 {
		t.Fatalf("pending switches after discard = %d, want 0", len(ctx.PendingConnectionSwitches))
	}
	if len(manager.switches) != 0 {
		t.Fatalf("switches after discard = %#v, want none", manager.switches)
	}
}

func TestConnectionNameFormats(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	globalConnManager = &stubConnManager{
		conn:   &stubConn{remote: "[::1]:4567", listenerPort: 7777},
		listen: 7777,
	}

	ctx := kernel.NewTaskContext()
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

func TestListenBuildsListenerSpecFromOptions(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8888),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("tcp")},
			{types.NewStr("interface"), types.NewStr("127.0.0.1")},
			{types.NewStr("print-messages"), types.NewInt(1)},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	port, ok := res.Val.(types.IntValue)
	if !ok || port.Val != 8888 {
		t.Fatalf("got %v (%T), want TCP port 8888", res.Val, res.Val)
	}
	if manager.added.Object != 42 ||
		manager.added.Port != 8888 ||
		manager.added.Protocol != ListenerProtocolTCP ||
		manager.added.Interface != "127.0.0.1" ||
		!manager.added.PrintMessages {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestListenBuildsTLSListenerSpec(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8889),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("tls")},
			{types.NewStr("certificate"), types.NewStr("server.crt")},
			{types.NewStr("key"), types.NewStr("server.key")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	desc, ok := res.Val.(types.MapValue)
	if !ok {
		t.Fatalf("got %T, want descriptor map", res.Val)
	}
	protocol, _ := desc.Get(types.NewStr("protocol"))
	port, _ := desc.Get(types.NewStr("port"))
	if protocol.(types.StrValue).Value() != "tls" || port.(types.IntValue).Val != 8889 {
		t.Fatalf("unexpected descriptor: %s", desc.String())
	}
	if manager.added.Protocol != "tls" ||
		manager.added.TLSCertificatePath != "server.crt" ||
		manager.added.TLSKeyPath != "server.key" {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestListenBuildsWebSocketListenerSpec(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8890),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("ws")},
			{types.NewStr("path"), types.NewStr("/moo")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	desc, ok := res.Val.(types.MapValue)
	if !ok {
		t.Fatalf("got %T, want descriptor map", res.Val)
	}
	protocol, _ := desc.Get(types.NewStr("protocol"))
	port, _ := desc.Get(types.NewStr("port"))
	path, _ := desc.Get(types.NewStr("path"))
	if protocol.(types.StrValue).Value() != "ws" ||
		port.(types.IntValue).Val != 8890 ||
		path.(types.StrValue).Value() != "/moo" {
		t.Fatalf("unexpected descriptor: %s", desc.String())
	}
	if manager.added.Protocol != "ws" || manager.added.Path != "/moo" {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestUnlistenAcceptsListenerDescriptorMap(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	res := builtinUnlisten(ctx, []types.Value{
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("ws")},
			{types.NewStr("port"), types.NewInt(8888)},
			{types.NewStr("path"), types.NewStr("/moo")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	want := ListenerDescriptor{Protocol: "ws", Port: 8888, Path: "/moo"}
	if !listenerDescriptorEqual(manager.removed, want) {
		t.Fatalf("removed %+v, want %+v", manager.removed, want)
	}
}

func TestListenersIncludesProtocolMetadataAndFiltersByDescriptor(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	globalConnManager = &stubConnManager{
		infos: []ListenerInfo{
			{
				Object:        5,
				Port:          8888,
				Protocol:      "ws",
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

	ctx := kernel.NewTaskContext()
	res := builtinListeners(ctx, []types.Value{
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("ws")},
			{types.NewStr("port"), types.NewInt(8888)},
			{types.NewStr("path"), types.NewStr("/moo")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	list, ok := res.Val.(types.ListValue)
	if !ok {
		t.Fatalf("got %T, want list", res.Val)
	}
	if list.Len() != 1 {
		t.Fatalf("got %d entries, want 1", list.Len())
	}
	entry, ok := list.Get(1).(types.MapValue)
	if !ok {
		t.Fatalf("got %T, want map", list.Get(1))
	}
	protocol, _ := entry.Get(types.NewStr("protocol"))
	path, _ := entry.Get(types.NewStr("path"))
	tlsValue, _ := entry.Get(types.NewStr("TLS"))
	if protocol.(types.StrValue).Value() != "ws" ||
		path.(types.StrValue).Value() != "/moo" ||
		tlsValue.(types.IntValue).Val != 0 {
		t.Fatalf("unexpected listener entry: %s", entry.String())
	}
}
