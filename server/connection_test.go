package server

import (
	"io"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

type stubTransport struct{}

func (stubTransport) ReadLine() (string, error) { return "", io.EOF }
func (stubTransport) WriteLine(string) error    { return nil }
func (stubTransport) Close() error              { return nil }
func (stubTransport) RemoteAddr() string        { return "127.0.0.1:7777" }

func TestSwitchPlayerDisconnectRestoresPreviousConnection(t *testing.T) {
	cm := NewConnectionManager(7777)
	player := types.ObjID(8)

	mainConn := cm.NewConnectionFromTransport(stubTransport{})
	if err := cm.SwitchPlayer(types.ObjID(-mainConn.ID), player); err != nil {
		t.Fatalf("switch main connection: %v", err)
	}

	secondConn := cm.NewConnectionFromTransport(stubTransport{})
	if err := cm.SwitchPlayer(types.ObjID(-secondConn.ID), player); err != nil {
		t.Fatalf("switch second connection: %v", err)
	}

	if got := cm.GetConnection(player); got != secondConn {
		t.Fatalf("active connection = %v, want second connection", got)
	}

	cm.mu.Lock()
	delete(cm.connections, secondConn.ID)
	delete(cm.playerConns, player)
	restored := cm.restorePreviousPlayerConnLocked(player, secondConn)
	cm.mu.Unlock()

	if restored != mainConn {
		t.Fatalf("restored connection = %v, want main connection", restored)
	}
	if got := cm.GetConnection(player); got != mainConn {
		t.Fatalf("active connection after restore = %v, want main connection", got)
	}
}

// TestProxiedIPRewritesRemoteAddr: after a PROXY prelude is accepted,
// RemoteAddr reports the announced client IP with the real remote port
// (Toast proxy_rewrite semantics; the resolved name is set separately).
func TestProxiedIPRewritesRemoteAddr(t *testing.T) {
	conn := NewConnection(1, stubTransport{})
	if got := conn.RemoteAddr(); got != "127.0.0.1:7777" {
		t.Fatalf("pre-proxy RemoteAddr = %q, want 127.0.0.1:7777", got)
	}
	conn.SetProxiedIP("203.0.113.5")
	if got := conn.RemoteAddr(); got != "203.0.113.5:7777" {
		t.Fatalf("post-proxy RemoteAddr = %q, want 203.0.113.5:7777", got)
	}
}
