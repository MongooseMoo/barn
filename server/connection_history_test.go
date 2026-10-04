package server

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func historyTestConnection(t *testing.T, cm *ConnectionManager, player types.ObjID) *Connection {
	t.Helper()
	conn := cm.NewConnectionFromTransport(stubTransport{})
	conn.SetPlayer(player)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func assertHistorySlotsCleared(t *testing.T, backing []*Connection) {
	t.Helper()
	for i, conn := range backing {
		if conn != nil {
			t.Errorf("discarded history backing slot %d still holds connection %d", i, conn.ID)
		}
	}
}

func TestRemovePlayerHistoryClearsDiscardedBackingSlots(t *testing.T) {
	for _, survivors := range []bool{false, true} {
		t.Run(map[bool]string{false: "last entry", true: "survivors"}[survivors], func(t *testing.T) {
			cm := NewConnectionManager(7777)
			removed := historyTestConnection(t, cm, 8)
			backing := []*Connection{removed, nil, removed}
			var want []*Connection
			if survivors {
				want = []*Connection{historyTestConnection(t, cm, 8), historyTestConnection(t, cm, 8)}
				backing = []*Connection{want[0], removed, nil, want[1], removed}
				cm.playerConns[8] = want[1]
			}
			cm.playerConnHistory[8] = backing
			cm.mu.Lock()
			cm.removePlayerHistoryConnLocked(8, removed)
			cm.mu.Unlock()
			if !slices.Equal(cm.playerConnHistory[8], want) {
				t.Fatal("removal changed survivor order")
			}
			if _, exists := cm.playerConnHistory[8]; exists != survivors {
				t.Fatalf("history exists=%v, want %v", exists, survivors)
			}
			if cm.connections[removed.ID] != removed || survivors && cm.GetConnection(8) != want[1] {
				t.Fatal("history removal changed connection or active-player mappings")
			}
			assertHistorySlotsCleared(t, backing[len(want):])
		})
	}
}

func TestRestorePlayerHistoryClearsPoppedBackingSlots(t *testing.T) {
	cm := NewConnectionManager(7777)
	older := historyTestConnection(t, cm, 8)
	prior := historyTestConnection(t, cm, 8)
	closing := historyTestConnection(t, cm, 8)
	stale := historyTestConnection(t, cm, 8)
	delete(cm.connections, stale.ID)
	unlogged := cm.NewConnectionFromTransport(stubTransport{})
	t.Cleanup(func() { _ = unlogged.Close() })
	wrongPlayer := historyTestConnection(t, cm, 9)
	cm.playerConns[8] = closing
	backing := []*Connection{older, prior, stale, closing, nil, unlogged, wrongPlayer}
	cm.playerConnHistory[8] = backing
	cm.mu.Lock()
	restored := cm.restorePreviousPlayerConnLocked(8, closing)
	cm.mu.Unlock()
	if restored != prior || cm.GetConnection(8) != prior || !slices.Equal(cm.playerConnHistory[8], []*Connection{older}) {
		t.Fatal("restoration did not select the newest eligible connection and retain older history")
	}
	assertHistorySlotsCleared(t, backing[1:])
	cm.mu.Lock()
	restored = cm.restorePreviousPlayerConnLocked(8, prior)
	cm.mu.Unlock()
	if restored != older || cm.GetConnection(8) != older {
		t.Fatal("second restoration lost reconnect order")
	}
	if _, exists := cm.playerConnHistory[8]; exists {
		t.Fatal("empty history remains in the map")
	}
	assertHistorySlotsCleared(t, backing)
}

func TestRestorePlayerHistoryClearsExhaustedStaleSlots(t *testing.T) {
	cm := NewConnectionManager(7777)
	closing := historyTestConnection(t, cm, 8)
	stale := historyTestConnection(t, cm, 8)
	delete(cm.connections, stale.ID)
	backing := []*Connection{stale, nil, closing}
	cm.playerConnHistory[8] = backing
	cm.playerConns[8] = closing
	cm.mu.Lock()
	restored := cm.restorePreviousPlayerConnLocked(8, closing)
	cm.mu.Unlock()
	if restored != nil || cm.GetConnection(8) != closing {
		t.Fatal("exhausted history changed the existing active mapping")
	}
	if _, exists := cm.playerConnHistory[8]; exists {
		t.Fatal("exhausted history remains in the map")
	}
	assertHistorySlotsCleared(t, backing)
}
