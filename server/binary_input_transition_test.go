package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
	"github.com/coder/websocket"
)

// The production TCP transport still owns framing and mode selection. This
// wrapper only exposes when its buffered reader reaches the actual socket.
type binaryTransitionConn struct {
	net.Conn
	reads chan struct{}
}

func (c *binaryTransitionConn) Read(data []byte) (int, error) {
	c.reads <- struct{}{}
	return c.Conn.Read(data)
}

func TestDisablingBinaryModeWakesBlockedReader(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	socket, peer := net.Pipe()
	t.Cleanup(func() {
		_ = socket.Close()
		_ = peer.Close()
	})
	observed := &binaryTransitionConn{Conn: socket, reads: make(chan struct{}, 8)}
	conn := cm.NewConnectionFromTransport(NewTCPTransport(observed))
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); return 1;`, "1")

	lines := make(chan string, 16)
	outputDone := make(chan struct{})
	readerDone := make(chan struct{})
	t.Cleanup(func() {
		_ = conn.Close()
		_ = peer.Close()
		for name, done := range map[string]<-chan struct{}{
			"input reader":  readerDone,
			"output reader": outputDone,
		} {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Errorf("%s did not join after connection close", name)
			}
		}
	})
	go func() {
		defer close(outputDone)
		defer close(lines)
		scanner := bufio.NewScanner(peer)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	processor.Start()
	go func() {
		defer close(readerDone)
		processor.HandleConnection(conn)
	}()

	select {
	case <-observed.reads:
	case <-time.After(3 * time.Second):
		t.Fatal("binary reader never reached the actual socket")
	}
	// This uses the real builtin, just as read_http callers restore text mode.
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 0); return 1;`, "1")
	select {
	case <-observed.reads:
		// A fresh socket read means the old binary read was interrupted and
		// HandleConnection has selected text mode before the next statement.
	case <-time.After(time.Second):
		t.Error("disabling binary mode left the old binary read blocked")
	}
	if err := peer.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(peer, "; return 42;\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case line, ok := <-lines:
		if !ok || line != "{1, 42}" {
			t.Fatalf("first text statement output = %q, open=%v; want {1, 42}", line, ok)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first text statement did not produce an eval result")
	}
}

// A socket read that has already passed its deadline check still returns
// bytes that arrive after SetReadDeadline(now). This conn models that read:
// it ignores read deadlines and returns whatever the peer writes next.
type deadlinePassedConn struct {
	binaryTransitionConn
}

func (c *deadlinePassedConn) SetReadDeadline(time.Time) error { return nil }

func TestStatementAfterBinaryOffIsTextWhenBinaryReadReturnsIt(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	socket, peer := net.Pipe()
	t.Cleanup(func() {
		_ = socket.Close()
		_ = peer.Close()
	})
	observed := &deadlinePassedConn{binaryTransitionConn{Conn: socket, reads: make(chan struct{}, 8)}}
	conn := cm.NewConnectionFromTransport(NewTCPTransport(observed))
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); return 1;`, "1")

	lines := make(chan string, 16)
	outputDone := make(chan struct{})
	readerDone := make(chan struct{})
	t.Cleanup(func() {
		_ = conn.Close()
		_ = peer.Close()
		for name, done := range map[string]<-chan struct{}{
			"input reader":  readerDone,
			"output reader": outputDone,
		} {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Errorf("%s did not join after connection close", name)
			}
		}
	})
	go func() {
		defer close(outputDone)
		defer close(lines)
		scanner := bufio.NewScanner(peer)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	processor.Start()
	go func() {
		defer close(readerDone)
		processor.HandleConnection(conn)
	}()

	select {
	case <-observed.reads:
	case <-time.After(3 * time.Second):
		t.Fatal("binary reader never reached the actual socket")
	}
	// The binary read is in flight. Turning binary off cannot stop it here, so
	// the statement sent next is returned by that same read.
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 0); return 1;`, "1")
	if err := peer.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(peer, "; return 42;\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case line, ok := <-lines:
		if !ok || line != "{1, 42}" {
			t.Fatalf("statement sent after binary off produced %q, open=%v; want {1, 42}", line, ok)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("statement sent after binary off did not produce an eval result")
	}
}

// Toast tests h->binary on the bytes each read() returns (network.cc
// pull_input), so bytes sent after binary is turned on are one binary chunk
// even when the text read that was waiting returns them.
func TestChunkAfterBinaryOnIsBinaryWhenTextReadReturnsIt(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	socket, peer := net.Pipe()
	t.Cleanup(func() {
		_ = socket.Close()
		_ = peer.Close()
	})
	observed := &deadlinePassedConn{binaryTransitionConn{Conn: socket, reads: make(chan struct{}, 8)}}
	conn := cm.NewConnectionFromTransport(NewTCPTransport(observed))
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}

	lines := make(chan string, 16)
	outputDone := make(chan struct{})
	readerDone := make(chan struct{})
	t.Cleanup(func() {
		_ = conn.Close()
		_ = peer.Close()
		for name, done := range map[string]<-chan struct{}{
			"input reader":  readerDone,
			"output reader": outputDone,
		} {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Errorf("%s did not join after connection close", name)
			}
		}
	})
	go func() {
		defer close(outputDone)
		defer close(lines)
		scanner := bufio.NewScanner(peer)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	processor.Start()
	go func() {
		defer close(readerDone)
		processor.HandleConnection(conn)
	}()

	select {
	case <-observed.reads:
	case <-time.After(3 * time.Second):
		t.Fatal("text reader never reached the actual socket")
	}
	// The text read is in flight. Turning binary on cannot stop it here, so
	// the bytes sent next, with no line terminator, are returned by that read.
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); return 1;`, "1")
	if err := peer.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(peer, "; return 42;"); err != nil {
		t.Fatal(err)
	}
	select {
	case line, ok := <-lines:
		if !ok || line != "{1, 42}" {
			t.Fatalf("chunk sent after binary on produced %q, open=%v; want {1, 42}", line, ok)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("chunk sent after binary on was not dispatched")
	}
}

type binaryTransitionWSConn struct {
	reads  chan context.Context
	input  chan []byte
	output chan string
	closed chan struct{}
	once   sync.Once
}

func (c *binaryTransitionWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	c.reads <- ctx
	select {
	case <-ctx.Done():
		return websocket.MessageText, nil, ctx.Err()
	case <-c.closed:
		return websocket.MessageText, nil, net.ErrClosed
	case payload := <-c.input:
		if err := ctx.Err(); err != nil {
			return websocket.MessageText, nil, err
		}
		return websocket.MessageText, payload, nil
	}
}

func (c *binaryTransitionWSConn) Write(ctx context.Context, _ websocket.MessageType, payload []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return net.ErrClosed
	case c.output <- string(payload):
		return nil
	}
}

func (c *binaryTransitionWSConn) Close(websocket.StatusCode, string) error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *binaryTransitionWSConn) CloseNow() error {
	return c.Close(websocket.StatusNormalClosure, "")
}

func TestBinaryModeDoesNotInterruptWebSocketReader(t *testing.T) {
	for _, value := range []int{0, 1} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			rt, cm, processor := connectionOptionIdentityFixture(t)
			peer := &binaryTransitionWSConn{
				reads:  make(chan context.Context, 4),
				input:  make(chan []byte, 1),
				output: make(chan string, 4),
				closed: make(chan struct{}),
			}
			t.Cleanup(func() { _ = peer.CloseNow() })
			conn := cm.NewConnectionFromTransport(NewWebSocketTransport(peer, "test:1234"))
			if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
				t.Fatal(err)
			}
			processor.Start()
			done := make(chan struct{})
			t.Cleanup(func() {
				_ = conn.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("WebSocket input reader did not join after close")
				}
			})
			go func() {
				defer close(done)
				processor.HandleConnection(conn)
			}()
			var readContext context.Context
			select {
			case readContext = <-peer.reads:
			case <-time.After(3 * time.Second):
				t.Fatal("WebSocket reader never reached the actual connection")
			}
			requireConnectionOptionEval(t, rt, fmt.Sprintf(`set_connection_option(#2, "binary", %d); return 1;`, value), "1")
			if err := readContext.Err(); err != nil {
				t.Errorf("binary option cancelled the unchanged WebSocket read: %v", err)
			}
			peer.input <- []byte("; return 42;")
			select {
			case line := <-peer.output:
				if line != "{1, 42}" {
					t.Fatalf("next WebSocket statement output = %q; want {1, 42}", line)
				}
			case <-done:
				t.Fatal("binary option disconnected the WebSocket before its next statement")
			case <-time.After(3 * time.Second):
				t.Fatal("next WebSocket statement did not produce an eval result")
			}
		})
	}
}
