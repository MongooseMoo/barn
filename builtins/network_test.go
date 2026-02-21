package builtins

import (
	"barn/types"
	"testing"
)

type stubConn struct {
	remote string
}

func (c *stubConn) Send(message string) error    { return nil }
func (c *stubConn) Buffer(message string)         {}
func (c *stubConn) Flush() error                  { return nil }
func (c *stubConn) RemoteAddr() string            { return c.remote }
func (c *stubConn) GetOutputPrefix() string       { return "" }
func (c *stubConn) GetOutputSuffix() string       { return "" }
func (c *stubConn) BufferedOutputLength() int     { return 0 }
func (c *stubConn) ConnectedSeconds() int64       { return 0 }
func (c *stubConn) IdleSeconds() int64            { return 0 }

type stubConnManager struct {
	conn   Connection
	listen int
}

func (m *stubConnManager) GetConnection(player types.ObjID) Connection { return m.conn }
func (m *stubConnManager) ConnectedPlayers(showAll bool) []types.ObjID  { return []types.ObjID{7} }
func (m *stubConnManager) BootPlayer(player types.ObjID) error          { return nil }
func (m *stubConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	return nil
}
func (m *stubConnManager) GetListenPort() int { return m.listen }

func TestConnectionNameFormats(t *testing.T) {
	prev := globalConnManager
	defer func() { globalConnManager = prev }()

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
