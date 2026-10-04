package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type blockedOutputWSConn struct {
	reads        chan context.Context
	writes       chan string
	readRelease  chan struct{}
	writeRelease chan struct{}
	activeWrites atomic.Int32
	overlap      atomic.Bool
}

func (c *blockedOutputWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	c.reads <- ctx
	select {
	case <-ctx.Done():
		return websocket.MessageText, nil, ctx.Err()
	case <-c.readRelease:
		return websocket.MessageText, []byte("input"), nil
	}
}

func (c *blockedOutputWSConn) Write(_ context.Context, typ websocket.MessageType, payload []byte) error {
	if c.activeWrites.Add(1) != 1 {
		c.overlap.Store(true)
	}
	defer c.activeWrites.Add(-1)
	if typ != websocket.MessageText {
		return errors.New("expected text frame")
	}
	c.writes <- string(payload)
	<-c.writeRelease
	return nil
}

func (*blockedOutputWSConn) Close(websocket.StatusCode, string) error { return nil }

func newBlockedOutputWS(t *testing.T) (*WebSocketTransport, *blockedOutputWSConn, *sync.WaitGroup) {
	t.Helper()
	fake := &blockedOutputWSConn{
		reads: make(chan context.Context, 64), writes: make(chan string, 64),
		readRelease: make(chan struct{}), writeRelease: make(chan struct{}),
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		// Release both I/O directions before joining even on an assertion failure.
		close(fake.writeRelease)
		close(fake.readRelease)
		wg.Wait()
	})
	return NewWebSocketTransport(fake, "test"), fake, &wg
}

func awaitWS[T any](t *testing.T, ch <-chan T, action string) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(time.Second):
		t.Fatalf("%s blocked behind unrelated output", action)
		var zero T
		return zero
	}
}

func TestWebSocketReadLifecycleDuringBlockedWrite(t *testing.T) {
	for _, action := range []string{"wake active read", "set deadline and begin read", "begin and finish read"} {
		t.Run(action, func(t *testing.T) {
			transport, fake, wg := newBlockedOutputWS(t)
			readDone := make(chan error, 1)
			startRead := func() {
				wg.Add(1)
				go func() {
					defer wg.Done()
					line, err := transport.ReadLine()
					if err == nil && line != "input" {
						err = errors.New("read payload changed")
					}
					readDone <- err
				}()
			}
			if action == "wake active read" {
				startRead()
				awaitWS(t, fake.reads, "initial read")
			}
			writeDone := make(chan error, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				writeDone <- transport.WriteOutput("output", true)
			}()
			if got := awaitWS(t, fake.writes, "write entry"); got != "output" {
				t.Fatalf("write payload = %q", got)
			}
			switch action {
			case "wake active read":
				wakeDone := make(chan struct{})
				wg.Add(1)
				go func() { defer wg.Done(); transport.WakeReader(); close(wakeDone) }()
				awaitWS(t, wakeDone, "WakeReader")
				if err := awaitWS(t, readDone, "cancelled read completion"); !errors.Is(err, context.Canceled) {
					t.Fatalf("read error = %v, want cancellation", err)
				}
			case "set deadline and begin read":
				deadlineDone := make(chan error, 1)
				wg.Add(1)
				go func() {
					defer wg.Done()
					deadlineDone <- transport.SetReadDeadline(time.Now().Add(-time.Second))
				}()
				if err := awaitWS(t, deadlineDone, "SetReadDeadline"); err != nil {
					t.Fatal(err)
				}
				startRead()
				awaitWS(t, fake.reads, "deadline read entry")
				if err := awaitWS(t, readDone, "deadline read completion"); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("read error = %v, want deadline exceeded", err)
				}
			case "begin and finish read":
				startRead()
				ctx := awaitWS(t, fake.reads, "read entry")
				// Supply input without releasing output; endRead must also complete.
				fake.readRelease <- struct{}{}
				if err := awaitWS(t, readDone, "read completion"); err != nil {
					t.Fatal(err)
				}
				if ctx.Err() != context.Canceled {
					t.Fatal("completed read context was not cancelled")
				}
			}
			select {
			case err := <-writeDone:
				t.Fatalf("output returned before release: %v", err)
			default:
			}
		})
	}
}

func TestWebSocketConcurrentOutputRemainsSerialized(t *testing.T) {
	transport, fake, wg := newBlockedOutputWS(t)
	const writers = 32
	results := make(chan error, writers)
	startWrite := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- transport.WriteOutput("valid é", false)
		}()
	}
	startWrite()
	awaitWS(t, fake.writes, "first write entry")
	for i := 1; i < writers; i++ {
		startWrite()
	}
	select {
	case <-fake.writes:
		t.Fatal("second writer entered while first output was blocked")
	case <-time.After(100 * time.Millisecond):
	}
	// Let each serialized write finish without closing the cleanup-owned channel.
	for i := 0; i < writers; i++ {
		fake.writeRelease <- struct{}{}
		if err := awaitWS(t, results, "write completion"); err != nil {
			t.Fatal(err)
		}
	}
	if fake.overlap.Load() || fake.activeWrites.Load() != 0 {
		t.Fatal("concurrent writers overlapped")
	}
	for i := 1; i < writers; i++ {
		if got := awaitWS(t, fake.writes, "serialized frame"); got != "valid é" {
			t.Fatalf("payload = %q", got)
		}
	}
}
