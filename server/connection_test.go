package server

import (
	"io"
	"testing"

	"barn/types"
)

type stubTransport struct{}

func (stubTransport) ReadLine() (string, error) { return "", io.EOF }
func (stubTransport) WriteLine(string) error    { return nil }
func (stubTransport) Close() error              { return nil }
func (stubTransport) RemoteAddr() string        { return "127.0.0.1:7777" }

func TestSwitchPlayerDisconnectRestoresPreviousConnection(t *testing.T) {
	cm := NewConnectionManager(nil, 7777)
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
