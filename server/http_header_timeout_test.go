package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/coder/websocket"
)

func TestWebSocketHeaderTimeoutPreservesUpgradedSession(t *testing.T) {
	cm := NewConnectionManager(0)
	defer cm.CloseConnections("test complete")
	defer cm.CloseListeners()
	cm.setConnectionHandler(func(conn *Connection) {
		defer conn.Close()
		line, err := conn.ReadLine()
		if err == nil {
			_ = conn.Send(line)
		}
	})
	if err := cm.BindListeners([]listener.Spec{{Protocol: listener.ProtocolWebSocket, Interface: "127.0.0.1", Path: "/moo"}}); err != nil {
		t.Fatal(err)
	}
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprint(cm.GetListenPort()))
	cm.StartAccepting()
	ctx, cancel := context.WithTimeout(context.Background(), listener.HTTPReadHeaderTimeout+5*time.Second)
	defer cancel()
	upgraded, _, err := websocket.Dial(ctx, "ws://"+addr+"/moo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.CloseNow()
	peer, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err := peer.SetDeadline(time.Now().Add(listener.HTTPReadHeaderTimeout + 3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(peer, "GET /moo HTTP/1.1\r\nHost:"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, peer); err != nil {
		t.Fatalf("incomplete upgrade headers retained connection: %v", err)
	}
	// The upgraded session was already idle while the other socket exhausted
	// its header allowance. Its next message must still be delivered.
	if err := upgraded.Write(ctx, websocket.MessageText, []byte("still connected")); err != nil {
		t.Fatal(err)
	}
	_, payload, err := upgraded.Read(ctx)
	if err != nil || string(payload) != "still connected" {
		t.Fatalf("idle upgraded session: payload=%q err=%v", payload, err)
	}
}
