package server

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
)

var errWebSocketInvalidInput = errors.New("invalid websocket input")

// wsConn is the subset of *websocket.Conn that WebSocketTransport depends on.
// It exists so the transport's read-interruption logic can be exercised
// in-process with a fake connection (no HTTP handshake / port bind).
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Close(code websocket.StatusCode, reason string) error
}

type WebSocketTransport struct {
	conn       wsConn
	remoteAddr string
	mu         sync.Mutex
	readMu     sync.Mutex
	deadline   time.Time
	// readCancel cancels the context of the read currently blocked in
	// conn.Read, or nil when no read is in flight. Guarded by mu. readMu
	// serializes readers, so there is at most one in-flight read at a time.
	readCancel context.CancelFunc
}

func NewWebSocketTransport(conn wsConn, remoteAddr string) *WebSocketTransport {
	return &WebSocketTransport{
		conn:       conn,
		remoteAddr: remoteAddr,
	}
}

func (t *WebSocketTransport) ReadLine() (string, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	ctx, cancel := t.beginRead()
	defer t.endRead(cancel)

	messageType, payload, err := t.conn.Read(ctx)
	if err != nil {
		return "", err
	}
	if messageType == websocket.MessageBinary {
		_ = t.conn.Close(websocket.StatusUnsupportedData, "binary messages are not MOO input")
		return "", errWebSocketInvalidInput
	}
	if messageType != websocket.MessageText {
		return "", errWebSocketInvalidInput
	}
	if !utf8.Valid(payload) {
		_ = t.conn.Close(websocket.StatusInvalidFramePayloadData, "invalid UTF-8")
		return "", errWebSocketInvalidInput
	}
	if bytes.ContainsAny(payload, "\r\n") {
		_ = t.conn.Close(websocket.StatusPolicyViolation, "embedded newlines are not MOO input")
		return "", errWebSocketInvalidInput
	}
	return string(payload), nil
}

func (t *WebSocketTransport) WriteLine(message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.Write(context.Background(), websocket.MessageText, []byte(message))
}

func (t *WebSocketTransport) Close() error {
	return t.conn.Close(websocket.StatusNormalClosure, "")
}

func (t *WebSocketTransport) RemoteAddr() string {
	return t.remoteAddr
}

func (t *WebSocketTransport) SetReadDeadline(deadline time.Time) error {
	t.mu.Lock()
	t.deadline = deadline
	t.mu.Unlock()
	return nil
}

// WakeReader interrupts a read currently blocked in conn.Read so that
// Connection.WakeInputReader / graceful shutdown can unblock it promptly.
//
// Setting the deadline field alone is insufficient: it is only consulted when
// a read begins (beginRead), so it cannot interrupt a read that is already
// parked inside conn.Read(ctx). coder/websocket's Read returns as soon as its
// context is cancelled, so the genuine interrupt is to cancel that context.
// We also stamp the deadline so any read that starts after the wake (but before
// readCancel is observed) returns immediately too — matching TCPTransport's
// SetReadDeadline(time.Now()) semantics.
func (t *WebSocketTransport) WakeReader() {
	t.mu.Lock()
	t.deadline = time.Now()
	cancel := t.readCancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// beginRead builds the context for the next conn.Read, honoring any deadline
// set via SetReadDeadline, and records its cancel func so WakeReader can
// interrupt the read while it is parked.
func (t *WebSocketTransport) beginRead() (context.Context, context.CancelFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ctx := context.Background()
	var cancel context.CancelFunc
	if t.deadline.IsZero() {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithDeadline(ctx, t.deadline)
	}
	t.readCancel = cancel
	return ctx, cancel
}

func (t *WebSocketTransport) endRead(cancel context.CancelFunc) {
	cancel()
	t.mu.Lock()
	t.readCancel = nil
	t.mu.Unlock()
}

func (t *WebSocketTransport) readDeadline() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.deadline
}
