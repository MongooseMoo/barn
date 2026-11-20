package server

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/MongooseMoo/barn/internal/listener"
)

func TestReleaseUnstartedWebSocketReservation(t *testing.T) {
	s, err := NewServer(filepath.Join(t.TempDir(), "unused.db"), []listener.Spec{{
		Protocol: listener.ProtocolWebSocket, Interface: "127.0.0.1", Port: 0, Path: "/",
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.ReserveListeners()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.connManager.GetListenPort()))
	release()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("unstarted WebSocket reservation leaked: %v", err)
	}
	ln.Close()
}
