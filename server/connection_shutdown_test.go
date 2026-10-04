package server

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type shutdownPeer struct {
	stalled       bool
	writeErr      error
	written       chan string
	closed        chan struct{}
	closeOnce     sync.Once
	writeExit     chan struct{}
	writeCount    atomic.Int32
	closeCount    atomic.Int32
	activeWrites  atomic.Int32
	normalCloses  atomic.Int32
	handshakeGate chan struct{}
}

func newShutdownPeer() *shutdownPeer {
	return &shutdownPeer{written: make(chan string, 64), closed: make(chan struct{})}
}

func (p *shutdownPeer) write(payload []byte) (int, error) {
	p.writeCount.Add(1)
	p.activeWrites.Add(1)
	defer p.activeWrites.Add(-1)
	p.written <- string(payload)
	if p.stalled {
		<-p.closed
		if p.writeExit != nil {
			<-p.writeExit
		}
		return 0, net.ErrClosed
	}
	return len(payload), p.writeErr
}

func (p *shutdownPeer) close() error {
	p.closeCount.Add(1)
	p.closeOnce.Do(func() { close(p.closed) })
	return nil
}

type shutdownTCPConn struct{ *shutdownPeer }

func (c shutdownTCPConn) Read([]byte) (int, error)       { <-c.closed; return 0, net.ErrClosed }
func (c shutdownTCPConn) Write(b []byte) (int, error)    { return c.write(b) }
func (c shutdownTCPConn) Close() error                   { return c.close() }
func (shutdownTCPConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (shutdownTCPConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (shutdownTCPConn) SetDeadline(time.Time) error      { return nil }
func (shutdownTCPConn) SetReadDeadline(time.Time) error  { return nil }
func (shutdownTCPConn) SetWriteDeadline(time.Time) error { return nil }

type shutdownWSConn struct{ *shutdownPeer }

func (c shutdownWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return websocket.MessageText, nil, ctx.Err()
	case <-c.closed:
		return websocket.MessageText, nil, net.ErrClosed
	}
}
func (c shutdownWSConn) Write(_ context.Context, typ websocket.MessageType, b []byte) error {
	if typ != websocket.MessageText {
		return errors.New("expected text frame")
	}
	_, err := c.write(b)
	return err
}
func (c shutdownWSConn) Close(websocket.StatusCode, string) error {
	c.normalCloses.Add(1)
	if c.handshakeGate != nil {
		<-c.handshakeGate
	}
	return c.close()
}
func (c shutdownWSConn) CloseNow() error { return c.close() }

func shutdownTransport(protocol string, peer *shutdownPeer) Transport {
	if protocol == "TCP" {
		return NewTCPTransport(shutdownTCPConn{peer})
	}
	return NewWebSocketTransport(shutdownWSConn{peer}, "test")
}

func awaitShutdown[T any](t *testing.T, ch <-chan T, action string) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(3 * time.Second):
		t.Fatalf("shutdown did not finish %s within the overall bound", action)
		var zero T
		return zero
	}
}

func TestCloseConnectionsBoundsAllBannerWrites(t *testing.T) {
	for _, protocol := range []string{"TCP", "WebSocket"} {
		t.Run(protocol, func(t *testing.T) {
			cm := NewConnectionManager(7777)
			readResults := make(chan error, 10)
			cm.setConnectionHandler(func(conn *Connection) {
				_, err := conn.ReadLine()
				readResults <- err
			})
			var peers []*shutdownPeer
			for i := 0; i < 10; i++ {
				peer := newShutdownPeer()
				peer.stalled = i < 8
				if i == 9 {
					peer.writeErr = io.ErrUnexpectedEOF
				}
				peers = append(peers, peer)
				conn := cm.NewConnectionFromTransport(shutdownTransport(protocol, peer))
				cm.connectionWG.Add(1)
				go cm.handleConnection(conn)
			}
			done := make(chan struct{})
			go func() { cm.CloseConnections("Maintenance"); close(done) }()
			t.Cleanup(func() {
				for _, peer := range peers {
					_ = peer.close()
				}
				<-done
			})
			awaitShutdown(t, done, "eight stalled, healthy, and failing clients")
			for i, peer := range peers {
				want := "*** Shutting down: Maintenance ***"
				if protocol == "TCP" {
					want += "\r\n"
				}
				if got := awaitShutdown(t, peer.written, "banner attempt"); got != want {
					t.Fatalf("peer %d banner = %q, want %q", i, got, want)
				}
				if peer.closeCount.Load() != 1 || peer.activeWrites.Load() != 0 {
					t.Fatalf("peer %d closes=%d active writers=%d", i, peer.closeCount.Load(), peer.activeWrites.Load())
				}
				if err := awaitShutdown(t, readResults, "connection reader completion"); !errors.Is(err, net.ErrClosed) {
					t.Fatalf("connection reader error = %v, want closed", err)
				}
			}
		})
	}
}

func TestCloseConnectionsJoinsBannerAndConnectionWorkers(t *testing.T) {
	cm := NewConnectionManager(7777)
	peer := newShutdownPeer()
	peer.stalled = true
	peer.writeExit = make(chan struct{})
	conn := cm.NewConnectionFromTransport(shutdownTransport("TCP", peer))
	handlerExit := make(chan struct{})
	cm.setConnectionHandler(func(conn *Connection) { _, _ = conn.ReadLine(); <-handlerExit })
	cm.connectionWG.Add(1)
	go cm.handleConnection(conn)
	done := make(chan struct{})
	go func() { cm.CloseConnections("Maintenance"); close(done) }()
	var releaseWriter, releaseHandler sync.Once
	t.Cleanup(func() {
		_ = peer.close()
		releaseWriter.Do(func() { close(peer.writeExit) })
		releaseHandler.Do(func() { close(handlerExit) })
		<-done
	})
	awaitShutdown(t, peer.written, "stalled write entry")
	awaitShutdown(t, peer.closed, "transport closure")
	select {
	case <-done:
		t.Fatal("shutdown abandoned an unjoined banner writer")
	default:
	}
	releaseWriter.Do(func() { close(peer.writeExit) })
	select {
	case <-done:
		t.Fatal("shutdown abandoned an unjoined connection worker")
	case <-time.After(100 * time.Millisecond):
	}
	releaseHandler.Do(func() { close(handlerExit) })
	awaitShutdown(t, done, "worker joins")
}

func TestCloseConnectionsIsIdempotent(t *testing.T) {
	for _, protocol := range []string{"TCP", "WebSocket"} {
		t.Run(protocol, func(t *testing.T) {
			cm := NewConnectionManager(7777)
			peer := newShutdownPeer()
			cm.NewConnectionFromTransport(shutdownTransport(protocol, peer))
			cm.CloseConnections("first")
			var callers sync.WaitGroup
			for i := 0; i < 16; i++ {
				callers.Add(1)
				go func() { defer callers.Done(); cm.CloseConnections("again") }()
			}
			callers.Wait()
			if peer.writeCount.Load() != 1 || peer.closeCount.Load() != 1 {
				t.Fatalf("repeated shutdown writes=%d closes=%d, want one each", peer.writeCount.Load(), peer.closeCount.Load())
			}
		})
	}
}

func TestCloseConnectionsDoesNotWaitForWebSocketCloseHandshake(t *testing.T) {
	cm := NewConnectionManager(7777)
	peer := newShutdownPeer()
	peer.handshakeGate = make(chan struct{})
	cm.NewConnectionFromTransport(shutdownTransport("WebSocket", peer))
	done := make(chan struct{})
	go func() { cm.CloseConnections("Maintenance"); close(done) }()
	t.Cleanup(func() { close(peer.handshakeGate); _ = peer.close(); <-done })
	awaitShutdown(t, done, "WebSocket closure without a peer acknowledgment")
	if peer.normalCloses.Load() != 0 || peer.closeCount.Load() != 1 {
		t.Fatalf("normal closes=%d immediate closes=%d", peer.normalCloses.Load(), peer.closeCount.Load())
	}
}

func TestCloseConnectionsInterruptsExistingOutput(t *testing.T) {
	for _, protocol := range []string{"TCP", "WebSocket"} {
		t.Run(protocol, func(t *testing.T) {
			cm := NewConnectionManager(7777)
			peer := newShutdownPeer()
			peer.stalled = true
			conn := cm.NewConnectionFromTransport(shutdownTransport(protocol, peer))
			writeDone := make(chan error, 1)
			var workers sync.WaitGroup
			workers.Add(1)
			go func() { defer workers.Done(); writeDone <- conn.Send("existing output") }()
			t.Cleanup(func() { _ = peer.close(); workers.Wait() })
			awaitShutdown(t, peer.written, "existing output entry")
			done := make(chan struct{})
			workers.Add(1)
			go func() { defer workers.Done(); cm.CloseConnections("Maintenance"); close(done) }()
			awaitShutdown(t, done, "banner queued behind existing output")
			if err := awaitShutdown(t, writeDone, "existing output completion"); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("existing output error = %v, want closed", err)
			}
			if peer.activeWrites.Load() != 0 {
				t.Fatal("shutdown left an active output operation")
			}
		})
	}
}
