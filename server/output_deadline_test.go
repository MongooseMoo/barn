package server

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Keep the real pipe's deadline short while observing the production allowance.
// Without a transport deadline, the pipe remains blocked until test cleanup.
type shortWriteDeadlineConn struct {
	net.Conn
	deadlines []time.Time
	writes    int
	armErr    error
	clearErr  error
	closeErr  error
}

func (c *shortWriteDeadlineConn) SetWriteDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	if !deadline.IsZero() {
		if c.armErr != nil {
			return c.armErr
		}
		deadline = time.Now().Add(100 * time.Millisecond)
	} else if c.clearErr != nil {
		return c.clearErr
	}
	return c.Conn.SetWriteDeadline(deadline)
}

func (c *shortWriteDeadlineConn) Write(payload []byte) (int, error) {
	c.writes++
	return c.Conn.Write(payload)
}

func (c *shortWriteDeadlineConn) Close() error {
	err := c.Conn.Close()
	if c.closeErr != nil {
		return c.closeErr
	}
	return err
}

func awaitOutputDeadline(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("output did not return after the peer stopped reading")
		return nil
	}
}

func checkOutputDeadline(t *testing.T, deadline, started time.Time) {
	t.Helper()
	if !deadline.After(started) || deadline.After(started.Add(time.Minute)) {
		t.Errorf("write deadline = %v, want a finite active-write allowance", deadline)
	}
}

func TestTCPOutputDeadlineInterruptsPartialWriteWithoutReplay(t *testing.T) {
	local, remote := net.Pipe()
	peer := &shortWriteDeadlineConn{
		Conn:     local,
		clearErr: errors.New("deadline cleanup failed"),
		closeErr: errors.New("socket close failed"),
	}
	transport := NewTCPTransport(peer)
	defer remote.Close()
	done := make(chan error, 1)
	finished := make(chan struct{})
	readDone := make(chan error, 1)
	readFinished := make(chan struct{})
	go func() {
		defer close(readFinished)
		_, err := transport.ReadLine()
		readDone <- err
	}()
	started := time.Now()
	go func() { defer close(finished); done <- transport.WriteOutput("hello", true) }()
	t.Cleanup(func() { _ = local.Close(); <-finished; <-readFinished })
	got := make([]byte, 2)
	if _, err := io.ReadFull(remote, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "he" {
		t.Fatalf("partial output = %q, want he", got)
	}
	err := awaitOutputDeadline(t, done)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("write error = %v, want deadline exceeded", err)
	}
	if len(peer.deadlines) != 1 {
		t.Fatalf("write deadlines = %v, want the failed stream closed without rearming", peer.deadlines)
	}
	checkOutputDeadline(t, peer.deadlines[0], started)
	select {
	case err := <-readDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("reader after output failure = %v, want closed socket", err)
		}
	case <-time.After(time.Second):
		t.Fatal("output failure left the connection reader blocked")
	}
	if err := remote.SetReadDeadline(time.Now().Add(time.Second)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("peer deadline after output failure = %v, want closed pipe", err)
	}
	if _, err := remote.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("peer after partial output failure = %v, want EOF", err)
	}
	writes := peer.writes
	if err := transport.WriteOutput("next", true); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write after partial timeout = %v, want retired socket", err)
	}
	if peer.writes != writes {
		t.Fatal("a failed partial output was replayed to the peer")
	}
}

func TestTCPOutputDeadlinePreservesFramingAndClearsAfterSuccess(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	peer := &shortWriteDeadlineConn{Conn: local}
	transport := NewTCPTransport(peer)
	done := make(chan error, 1)
	go func() { done <- transport.WriteOutput("hello", true) }()
	got := make([]byte, len("hello\r\n"))
	if _, err := io.ReadFull(remote, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello\r\n" {
		t.Fatalf("output = %q, want CRLF framing", got)
	}
	if err := awaitOutputDeadline(t, done); err != nil {
		t.Fatal(err)
	}
	if len(peer.deadlines) != 2 || !peer.deadlines[1].IsZero() {
		t.Fatalf("successful write deadlines = %v, want allowance then clearing", peer.deadlines)
	}
}

func TestTCPOutputDeadlineSetupFailureClosesConnection(t *testing.T) {
	for _, test := range []struct {
		name     string
		armErr   error
		clearErr error
	}{
		{name: "arming", armErr: errors.New("cannot set write deadline")},
		{name: "clearing", clearErr: errors.New("cannot clear write deadline")},
	} {
		t.Run(test.name, func(t *testing.T) {
			local, remote := net.Pipe()
			defer local.Close()
			defer remote.Close()
			peer := &shortWriteDeadlineConn{
				Conn: local, armErr: test.armErr, clearErr: test.clearErr,
				closeErr: errors.New("socket close failed"),
			}
			readDone := make(chan error, 1)
			if test.armErr == nil {
				go func() {
					_, err := io.ReadFull(remote, make([]byte, len("hello")))
					readDone <- err
				}()
			}
			want := test.armErr
			if want == nil {
				want = test.clearErr
			}
			if err := NewTCPTransport(peer).WriteOutput("hello", false); !errors.Is(err, want) {
				t.Fatalf("deadline error = %v, want original failure %v", err, want)
			}
			if test.armErr == nil {
				if err := awaitOutputDeadline(t, readDone); err != nil {
					t.Fatal(err)
				}
			}
			if err := remote.SetReadDeadline(time.Now().Add(time.Second)); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("peer deadline after output failure = %v, want closed pipe", err)
			}
			if _, err := remote.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("peer after deadline failure = %v, want EOF", err)
			}
		})
	}
}

type deadlineOutputWSConn struct {
	recordingOutputWSConn
	ctx      context.Context
	writeErr error
	onWrite  func()
	closes   int
}

func (c *deadlineOutputWSConn) Write(ctx context.Context, typ websocket.MessageType, payload []byte) error {
	c.ctx = ctx
	if c.onWrite != nil {
		c.onWrite()
	}
	_ = c.recordingOutputWSConn.Write(ctx, typ, payload)
	return c.writeErr
}

func (c *deadlineOutputWSConn) Close(websocket.StatusCode, string) error {
	c.closes++
	return nil
}

func TestWebSocketOutputDeadlineIsBoundedAndReleased(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{{"success", nil}, {"timeout", context.DeadlineExceeded}} {
		t.Run(test.name, func(t *testing.T) {
			peer := &deadlineOutputWSConn{writeErr: test.err}
			transport := NewWebSocketTransport(peer, "test")
			started := time.Now()
			if err := transport.WriteOutput("hello", true); !errors.Is(err, test.err) {
				t.Fatalf("write error = %v, want %v", err, test.err)
			}
			deadline, ok := peer.ctx.Deadline()
			if !ok {
				t.Fatal("WebSocket write context has no deadline")
			}
			checkOutputDeadline(t, deadline, started)
			if !errors.Is(peer.ctx.Err(), context.Canceled) {
				t.Fatalf("completed write context error = %v, want canceled", peer.ctx.Err())
			}
			if len(peer.payloads) != 1 || peer.payloads[0] != "hello" || peer.types[0] != websocket.MessageText {
				t.Fatalf("WebSocket output types=%v payloads=%q, want one text frame", peer.types, peer.payloads)
			}
		})
	}
}

func TestTCPOutputDeadlineLetsBootAndRecycleClose(t *testing.T) {
	for _, action := range []string{"boot", "recycle"} {
		t.Run(action, func(t *testing.T) {
			local, remote := net.Pipe()
			defer remote.Close()
			peer := &shortWriteDeadlineConn{Conn: local}
			cm := NewConnectionManager(0)
			conn := cm.NewConnectionFromTransport(NewTCPTransport(peer))
			conn.SetPlayer(42)
			cm.playerConns[42] = conn
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				if action == "boot" {
					done <- cm.BootPlayer(42)
				} else {
					done <- cm.RecyclePlayer(42)
				}
			}()
			t.Cleanup(func() { _ = local.Close(); <-finished })
			err := awaitOutputDeadline(t, done)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := remote.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("read after %s = %v, want closed peer", action, err)
			}
		})
	}
}

func TestWebSocketOutputDeadlinePreservesAdministrativeDisconnectHook(t *testing.T) {
	for _, action := range []string{"boot", "recycle"} {
		t.Run(action, func(t *testing.T) {
			peer := &deadlineOutputWSConn{writeErr: context.DeadlineExceeded}
			cm := NewConnectionManager(0)
			conn := cm.NewConnectionFromTransport(NewWebSocketTransport(peer, "test"))
			conn.SetPlayer(42)
			cm.playerConns[42] = conn
			wantHook := "user_disconnected"
			if action == "recycle" {
				wantHook = ""
			}
			// A real timed-out WebSocket write closes the socket and can wake
			// reader cleanup before Write returns. Observe the hook at that point.
			peer.onWrite = func() {
				if got := conn.getDisconnectHook(); got != wantHook {
					t.Errorf("hook during %s write = %q, want %q", action, got, wantHook)
				}
			}
			var err error
			if action == "boot" {
				err = cm.BootPlayer(42)
			} else {
				err = cm.RecyclePlayer(42)
			}
			if err != nil {
				t.Fatal(err)
			}
			if peer.closes != 1 {
				t.Fatalf("close count = %d, want one", peer.closes)
			}
		})
	}
}
