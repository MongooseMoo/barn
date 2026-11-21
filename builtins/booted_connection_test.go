package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A booted connection is closed at once (its lifetime channel is closed) but
// stays in the connection manager until its disconnect event is processed.
// Toast's boot_player sets disconnect_me, and connection_info,
// connected_seconds, idle_seconds, connection_name, connection_name_lookup,
// set_connection_option and connection_options all treat
// `!h || h->disconnect_me` as no connection (server.cc bf_connection_info and
// friends), so each raises E_INVARG at once.
func TestConnectionQueriesTreatBootedConnectionAsGone(t *testing.T) {
	ctx, target := bootedConnectionContext()
	cases := []struct {
		name string
		args []types.Value
	}{
		{"connection_info", []types.Value{target}},
		{"connected_seconds", []types.Value{target}},
		{"idle_seconds", []types.Value{target}},
		{"connection_name", []types.Value{target}},
		{"connection_name_lookup", []types.Value{target}},
		{"set_connection_option", []types.Value{target, types.NewStr("binary"), types.NewInt(1)}},
		{"connection_options", []types.Value{target}},
	}
	for _, tc := range cases {
		res := admitTestBuiltin(tc.name, ctx, tc.args)
		if res.Flow != types.FlowException || res.Error != types.E_INVARG {
			t.Errorf("%s on a booted connection = %+v, want E_INVARG", tc.name, res)
		}
	}
}

func bootedConnectionContext() (*Execution, types.Value) {
	done := make(chan struct{})
	close(done)
	conn := &lifetimeOptionConn{
		stubConn: stubConn{id: 1, remote: "127.0.0.1:4000", listenerPort: 7777, listenerPortSet: true},
		done:     done,
	}
	ctx := ctxWithConnManager(&stubConnManager{conn: conn, listen: 7777})
	ctx.IsWizard = true
	return ctx, types.NewObj(-1)
}

// Toast bf_notify sends nothing to a disconnect_me handle and returns 1, so a
// line that would be invalid in binary mode still returns 1.
func TestNotifyToBootedConnectionReturnsOne(t *testing.T) {
	ctx, target := bootedConnectionContext()
	ctx.Session.setConnectionOption(1, nil, "binary", types.NewInt(1))
	res := admitTestBuiltin("notify", ctx, []types.Value{target, types.NewStr("~zz")})
	if res.IsError() || res.Val.Type() != types.TYPE_INT || res.Val.Int() != 1 {
		t.Fatalf("notify on a booted connection = %+v, want 1", res)
	}
}

// Toast bf_name_lookup checks permission before it looks the connection up.
func TestConnectionNameLookupChecksPermissionFirst(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{})
	ctx.Programmer = 5
	res := admitTestBuiltin("connection_name_lookup", ctx, []types.Value{types.NewObj(7)})
	if res.Flow != types.FlowException || res.Error != types.E_PERM {
		t.Fatalf("connection_name_lookup by a non-wizard on another player = %+v, want E_PERM", res)
	}
}

// Inside a transaction boot_player is a commit-deferred effect, but Toast
// sets disconnect_me at the call: the booting task's own next calls already see
// no connection, and connected_players no longer lists it.
func TestBootingTaskSeesItsPendingBoot(t *testing.T) {
	conn := &stubConn{id: 1, remote: "127.0.0.1:4000"}
	ctx := ctxWithConnManager(&stubConnManager{conn: conn, listen: 7777})
	ctx.IsWizard = true
	ctx.StoreTxn = ctx.Store.BeginSnapshot(0)
	t.Cleanup(ctx.StoreTxn.Release)
	target := types.NewObj(7)
	if res := admitTestBuiltin("boot_player", ctx, []types.Value{target}); res.IsError() {
		t.Fatalf("boot_player = %+v", res)
	}
	if len(ctx.PendingEffects) != 1 {
		t.Fatalf("boot_player queued %d effects, want 1 deferred boot", len(ctx.PendingEffects))
	}
	if res := admitTestBuiltin("connection_info", ctx, []types.Value{target}); res.Flow != types.FlowException || res.Error != types.E_INVARG {
		t.Errorf("connection_info after boot_player in the same task = %+v, want E_INVARG", res)
	}
	res := admitTestBuiltin("connected_players", ctx, nil)
	if res.IsError() || res.Val.Len() != 0 {
		t.Errorf("connected_players after boot_player in the same task = %+v, want {}", res)
	}
}
