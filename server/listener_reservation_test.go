package server

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/listener"
)

func TestStartRetainsReservedListener(t *testing.T) {
	s, err := NewServer(filepath.Join(t.TempDir(), "unused.db"), []listener.Spec{{
		Protocol: listener.ProtocolTCP, Interface: "127.0.0.1", Port: 0,
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.ReserveListeners()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	port := s.connManager.GetListenPort()
	if port == 0 {
		t.Fatal("reservation did not bind a real port")
	}
	// Use the concrete allocated port so an attempted rebind in Start fails.
	s.listenerSpecs[0].Port = int64(port)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if ln, err := net.Listen("tcp", addr); err == nil {
		ln.Close()
		t.Fatal("game port is not held during startup")
	}
	// Exercise actual Start and shutdown against a disposable in-memory world.
	s.store = dbstore.NewStore()
	addTestObject(t, s.store, 0, dbstore.FlagWizard)
	s.runtime = engine.NewRuntime(s.store)
	s.input = NewInputProcessor(s.store, s.runtime)
	s.input.SetConnectionManager(s.connManager)
	s.cancel()
	if err := s.Start(); err != nil {
		t.Fatalf("Start did not retain reserved socket: %v", err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("shutdown did not release reservation: %v", err)
	}
	ln.Close()
}
