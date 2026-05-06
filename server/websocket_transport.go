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
	input      chan websocketReadResult
	mu         sync.Mutex
	readMu     sync.Mutex
	deadline   time.Time
}

type websocketReadResult struct {
	messageType websocket.MessageType
	payload     []byte
	err         error
}

type websocketTimeoutError struct{}

func (websocketTimeoutError) Error() string   { return "websocket read timeout" }
func (websocketTimeoutError) Timeout() bool   { return true }
func (websocketTimeoutError) Temporary() bool { return true }

func NewWebSocketTransport(conn *websocket.Conn, remoteAddr string) *WebSocketTransport {
	transport := &WebSocketTransport{
		conn:       conn,
		remoteAddr: remoteAddr,
		input:      make(chan websocketReadResult, 16),
	}
	go transport.readLoop()
	return transport
}

func (t *WebSocketTransport) readLoop() {
	for {
		messageType, payload, err := t.conn.Read(context.Background())
		t.input <- websocketReadResult{
			messageType: messageType,
			payload:     payload,
			err:         err,
		}
		if err != nil {
			return
		}
	}
}

func (t *WebSocketTransport) ReadLine() (string, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	if deadline := t.readDeadline(); !deadline.IsZero() {
		wait := time.Until(deadline)
		if wait <= 0 {
			return "", websocketTimeoutError{}
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case result := <-t.input:
			return t.validateReadResult(result)
		case <-timer.C:
			return "", websocketTimeoutError{}
		}
	}

	return t.validateReadResult(<-t.input)
}

func (t *WebSocketTransport) validateReadResult(result websocketReadResult) (string, error) {
	if result.err != nil {
		return "", result.err
	}
	if result.messageType == websocket.MessageBinary {
		_ = t.conn.Close(websocket.StatusUnsupportedData, "binary messages are not MOO input")
		return "", errWebSocketInvalidInput
	}
	if result.messageType != websocket.MessageText {
		return "", errWebSocketInvalidInput
	}
	if !utf8.Valid(result.payload) {
		_ = t.conn.Close(websocket.StatusInvalidFramePayloadData, "invalid UTF-8")
		return "", errWebSocketInvalidInput
	}
	if bytes.ContainsAny(result.payload, "\r\n") {
		_ = t.conn.Close(websocket.StatusPolicyViolation, "embedded newlines are not MOO input")
		return "", errWebSocketInvalidInput
	}
	return string(result.payload), nil
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
