package server

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/coder/websocket"
)

type recordingOutputWSConn struct {
	types    []websocket.MessageType
	payloads []string
}

func (*recordingOutputWSConn) Read(context.Context) (websocket.MessageType, []byte, error) {
	return websocket.MessageText, nil, io.EOF
}

func (c *recordingOutputWSConn) Write(_ context.Context, typ websocket.MessageType, payload []byte) error {
	c.types = append(c.types, typ)
	c.payloads = append(c.payloads, string(payload))
	return nil
}

func (*recordingOutputWSConn) Close(websocket.StatusCode, string) error { return nil }

func TestWebSocketOutputRejectsInvalidNotification(t *testing.T) {
	fake := &recordingOutputWSConn{}
	conn := NewConnection(1, NewWebSocketTransport(fake, "test"))
	defer conn.Close()
	if err := conn.SendNotification(kernel.PendingNotification{Message: "\xff", NoNewline: true}); !errors.Is(err, errWebSocketInvalidOutput) {
		t.Errorf("invalid UTF-8 notification error = %v, want invalid output", err)
	}
	if len(fake.payloads) != 0 {
		t.Fatalf("invalid UTF-8 reached WebSocket writer: %q", fake.payloads)
	}
	if err := conn.SendNotification(kernel.PendingNotification{Message: "valid \u00e9"}); err != nil {
		t.Fatalf("valid notification after rejection: %v", err)
	}
	if len(fake.payloads) != 1 || fake.payloads[0] != "valid \u00e9" || fake.types[0] != websocket.MessageText {
		t.Fatalf("valid text framing: types=%v payloads=%q", fake.types, fake.payloads)
	}
}

func TestWebSocketOutputRejectsInvalidBufferedNotification(t *testing.T) {
	fake := &recordingOutputWSConn{}
	conn := NewConnection(1, NewWebSocketTransport(fake, "test"))
	defer conn.Close()
	if err := conn.SendNotification(kernel.PendingNotification{Message: "\xff", NoFlush: true}); !errors.Is(err, errWebSocketInvalidOutput) {
		t.Errorf("invalid UTF-8 buffered notification error = %v, want invalid output", err)
	}
	if got := conn.BufferedOutputLength(); got != 0 {
		t.Errorf("invalid notification was queued: buffered=%d", got)
	}
	if len(fake.payloads) != 0 {
		t.Fatal("NoFlush notification reached WebSocket writer")
	}
	if err := conn.Flush(); err != nil {
		t.Fatalf("flush after rejected notification: %v", err)
	}
	if len(fake.payloads) != 0 {
		t.Fatalf("invalid buffered UTF-8 reached WebSocket writer: %q", fake.payloads)
	}
	if err := conn.SendNotification(kernel.PendingNotification{Message: "valid", NoFlush: true}); err != nil {
		t.Fatalf("valid buffered output after rejection: %v", err)
	}
	if err := conn.Flush(); err != nil {
		t.Fatalf("flush valid buffered output: %v", err)
	}
	if len(fake.payloads) != 1 || fake.payloads[0] != "valid" || fake.types[0] != websocket.MessageText {
		t.Fatalf("valid text framing: types=%v payloads=%q", fake.types, fake.payloads)
	}
}

func TestWebSocketOutputRejectsInvalidDirectWrite(t *testing.T) {
	fake := &recordingOutputWSConn{}
	transport := NewWebSocketTransport(fake, "test")
	if err := transport.WriteOutput("\xff", false); !errors.Is(err, errWebSocketInvalidOutput) {
		t.Errorf("invalid UTF-8 direct write error = %v, want invalid output", err)
	}
	if len(fake.payloads) != 0 {
		t.Fatalf("invalid UTF-8 reached WebSocket writer: %q", fake.payloads)
	}
}

func TestWebSocketOutputPreservesValidTextFraming(t *testing.T) {
	fake := &recordingOutputWSConn{}
	conn := NewConnection(1, NewWebSocketTransport(fake, "test"))
	defer conn.Close()
	for _, note := range []kernel.PendingNotification{
		{Message: "first", NoFlush: true},
		{Message: "\u00e9\nsecond", NoNewline: true},
		{Message: ""},
	} {
		if err := conn.SendNotification(note); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"first", "\u00e9\nsecond", ""}
	if len(fake.payloads) != len(want) {
		t.Fatalf("payloads=%q, want %q", fake.payloads, want)
	}
	for i, payload := range want {
		if fake.types[i] != websocket.MessageText || fake.payloads[i] != payload {
			t.Errorf("frame %d: type=%v payload=%q, want text %q", i, fake.types[i], fake.payloads[i], payload)
		}
	}
}
