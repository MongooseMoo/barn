package server

import (
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

type stubTransport struct{}

func (stubTransport) ReadLine() (string, error) { return "", io.EOF }
func (stubTransport) WriteLine(string) error    { return nil }
func (stubTransport) Close() error              { return nil }
func (stubTransport) RemoteAddr() string        { return "127.0.0.1:7777" }

type failOnceTransport struct {
	failAt int
	writes []string
}

func (t *failOnceTransport) ReadLine() (string, error) { return "", io.EOF }
func (t *failOnceTransport) WriteLine(line string) error {
	t.writes = append(t.writes, line)
	if len(t.writes) == t.failAt {
		return errors.New("transient write failure")
	}
	return nil
}
func (t *failOnceTransport) Close() error       { return nil }
func (t *failOnceTransport) RemoteAddr() string { return "127.0.0.1:7777" }

func TestFlushResumesAfterPartialWriteFailure(t *testing.T) {
	transport := &failOnceTransport{failAt: 3}
	conn := NewConnection(1, transport)
	for _, line := range []string{"one", "two", "three", "four"} {
		conn.Buffer(line)
	}

	if err := conn.Flush(); err == nil {
		t.Fatal("first Flush() error = nil, want transient write failure")
	}
	if got := conn.BufferedOutputLength(); got != 2 {
		t.Fatalf("buffered lines after failed Flush() = %d, want 2", got)
	}

	if err := conn.Flush(); err != nil {
		t.Fatalf("second Flush(): %v", err)
	}
	if got := conn.BufferedOutputLength(); got != 0 {
		t.Fatalf("buffered lines after successful retry = %d, want 0", got)
	}
	wantWrites := []string{"one", "two", "three", "three", "four"}
	if !reflect.DeepEqual(transport.writes, wantWrites) {
		t.Fatalf("WriteLine() calls = %q, want %q", transport.writes, wantWrites)
	}
}

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

func TestBufferedOutputLengthTracksQueuedLines(t *testing.T) {
	conn := NewConnection(1, stubTransport{})

	if err := conn.Send("immediate"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := conn.BufferedOutputLength(); got != 0 {
		t.Fatalf("BufferedOutputLength after Send = %d, want 0", got)
	}

	conn.Buffer("first")
	conn.Buffer("second")
	if got := conn.BufferedOutputLength(); got != 2 {
		t.Fatalf("BufferedOutputLength before Flush = %d, want 2", got)
	}

	if err := conn.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := conn.BufferedOutputLength(); got != 0 {
		t.Fatalf("BufferedOutputLength after Flush = %d, want 0", got)
	}
}
