package builtins

import (
	"barn/config"
	"barn/types"
	"testing"
)

type stubConn struct {
	remote       string
	listenerPort int64
}

func (c *stubConn) Send(message string) error { return nil }
func (c *stubConn) Buffer(message string)     {}
func (c *stubConn) Flush() error              { return nil }
func (c *stubConn) RemoteAddr() string        { return c.remote }
func (c *stubConn) GetOutputPrefix() string   { return "" }
func (c *stubConn) GetOutputSuffix() string   { return "" }
func (c *stubConn) BufferedOutputLength() int { return 0 }
func (c *stubConn) ConnectedSeconds() int64   { return 0 }
func (c *stubConn) IdleSeconds() int64        { return 0 }
func (c *stubConn) GetResolvedName() string   { return "" }
func (c *stubConn) ListenerPort() int64       { return c.listenerPort }

type stubConnManager struct {
	conn    Connection
	listen  int
	infos   []ListenerInfo
	added   ListenerSpec
	removed ListenerDescriptor
	opened  int
}

func (m *stubConnManager) GetConnection(player types.ObjID) Connection { return m.conn }
func (m *stubConnManager) ConnectedPlayers(showAll bool) []types.ObjID { return []types.ObjID{7} }
func (m *stubConnManager) BootPlayer(player types.ObjID) error         { return nil }
func (m *stubConnManager) RecyclePlayer(player types.ObjID) error      { return nil }
func (m *stubConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
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
	m.opened++
	return types.ObjID(-8), nil
}
func (m *stubConnManager) ConnectionNameLookup(player types.ObjID, rewrite bool) (string, error) {
	return "lookup", nil
}

func TestConnectionNameFormats(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	globalConnManager = &stubConnManager{
		conn:   &stubConn{remote: "[::1]:4567", listenerPort: 7777},
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

func TestConnectedPlayersAcceptsFalseSecondFlag(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	globalConnManager = &stubConnManager{}
	ctx := types.NewTaskContext()
	registry := NewRegistry()
	fn, ok := registry.Get("connected_players")
	if !ok {
		t.Fatal("connected_players builtin not registered")
	}

	res := fn(ctx, []types.Value{types.NewInt(1), types.NewInt(0)})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	got, ok := res.Val.(types.ListValue)
	if !ok {
		t.Fatalf("got %T, want list", res.Val)
	}
	if len(got.Elements()) != 1 {
		t.Fatalf("got %d players, want 1", len(got.Elements()))
	}

	res = fn(ctx, []types.Value{types.NewInt(1), types.NewInt(1)})
	if !res.IsError() || res.Error != types.E_ARGS {
		t.Fatalf("got %v, want E_ARGS", res)
	}
}

func TestOpenNetworkConnectionDisabledReturnsPermission(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
	ctx.IsWizard = true
	ctx.RuntimeOptions = config.Options{OutboundNetwork: false}

	res := builtinOpenNetworkConnection(ctx, []types.Value{types.NewStr("127.0.0.1"), types.NewInt(80)})
	if !res.IsError() || res.Error != types.E_PERM {
		t.Fatalf("got %v, want E_PERM", res)
	}
	if manager.opened != 0 {
		t.Fatalf("opened %d connections, want 0", manager.opened)
	}
}

func TestOpenNetworkConnectionEnabledKeepsCurrentBehavior(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
	ctx.IsWizard = true
	ctx.RuntimeOptions = config.Options{OutboundNetwork: true}

	res := builtinOpenNetworkConnection(ctx, []types.Value{types.NewStr("127.0.0.1"), types.NewInt(80)})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	got, ok := res.Val.(types.ObjValue)
	if !ok || got.ID() != -8 {
		t.Fatalf("got %v (%T), want #-8", res.Val, res.Val)
	}
	if manager.opened != 1 {
		t.Fatalf("opened %d connections, want 1", manager.opened)
	}
}

func TestListenBuildsListenerSpecFromOptions(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
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

	ctx := types.NewTaskContext()
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

	ctx := types.NewTaskContext()
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

func TestListenBuildsSecureWebSocketListenerSpec(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewInt(8891),
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("wss")},
			{types.NewStr("path"), types.NewStr("/moo")},
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
	path, _ := desc.Get(types.NewStr("path"))
	if protocol.(types.StrValue).Value() != "wss" ||
		port.(types.IntValue).Val != 8891 ||
		path.(types.StrValue).Value() != "/moo" {
		t.Fatalf("unexpected descriptor: %s", desc.String())
	}
	if manager.added.Protocol != "wss" ||
		manager.added.Path != "/moo" ||
		manager.added.TLSCertificatePath != "server.crt" ||
		manager.added.TLSKeyPath != "server.key" {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestListenBuildsUnixListenerSpec(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
	ctx.IsWizard = true

	res := builtinListen(ctx, []types.Value{
		types.NewObj(42),
		types.NewStr("/tmp/barn.sock"),
		types.NewMap([][2]types.Value{
			{types.NewStr("print-messages"), types.NewInt(1)},
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
	path, _ := desc.Get(types.NewStr("path"))
	if protocol.(types.StrValue).Value() != ListenerProtocolUnix ||
		path.(types.StrValue).Value() != "/tmp/barn.sock" {
		t.Fatalf("unexpected descriptor: %s", desc.String())
	}
	if _, ok := desc.Get(types.NewStr("port")); ok {
		t.Fatalf("unix descriptor included port: %s", desc.String())
	}
	if manager.added.Protocol != ListenerProtocolUnix ||
		manager.added.Path != "/tmp/barn.sock" ||
		!manager.added.PrintMessages {
		t.Fatalf("unexpected spec: %+v", manager.added)
	}
}

func TestUnlistenAcceptsListenerDescriptorMap(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
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

func TestUnlistenAcceptsUnixListenerDescriptorMap(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

	manager := &stubConnManager{}
	globalConnManager = manager

	ctx := types.NewTaskContext()
	ctx.IsWizard = true

	res := builtinUnlisten(ctx, []types.Value{
		types.NewMap([][2]types.Value{
			{types.NewStr("protocol"), types.NewStr("unix")},
			{types.NewStr("path"), types.NewStr("/tmp/barn.sock")},
		}),
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	want := ListenerDescriptor{Protocol: "unix", Path: "/tmp/barn.sock"}
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

	ctx := types.NewTaskContext()
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
