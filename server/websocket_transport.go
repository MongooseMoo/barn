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

type WebSocketTransport struct {
	conn       *websocket.Conn
	remoteAddr string
	mu         sync.Mutex
	readMu     sync.Mutex
	deadline   time.Time
}

func NewWebSocketTransport(conn *websocket.Conn, remoteAddr string) *WebSocketTransport {
	return &WebSocketTransport{
		conn:       conn,
		remoteAddr: remoteAddr,
	}
}

func (t *WebSocketTransport) ReadLine() (string, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	ctx := context.Background()
	if deadline := t.readDeadline(); !deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}

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

func (t *WebSocketTransport) WakeReader() {}

func (t *WebSocketTransport) readDeadline() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.deadline
}
