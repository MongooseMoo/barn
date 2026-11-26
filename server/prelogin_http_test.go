package server

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

func TestPreloginBinaryInputCompletesHTTPRead(t *testing.T) {
	store := dbstore.NewStore()
	listener := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard|dbstore.FlagProgrammer)
	addTestVerb(store, listener, "do_blank_command", "return 1;")
	addTestVerb(store, listener, "do_login_command",
		`set_connection_option(player, "hold-input", 1);`,
		`set_connection_option(player, "binary", 1);`,
		`request = read_http("request", player);`,
		`set_connection_option(player, "binary", 0);`,
		`notify(player, request["method"] + " " + request["uri"]);`,
		`return 0;`,
	)
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	manager := NewConnectionManager(7777)
	processor.SetConnectionManager(manager)
	setTestConnectionManager(rt.Session(), manager)

	socket, peer := net.Pipe()
	conn := manager.NewConnectionFromTransport(NewTCPTransport(socket))
	processor.Start()
	defer processor.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		processor.HandleConnection(conn)
	}()
	defer func() {
		_ = peer.Close()
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("connection reader did not stop")
		}
	}()

	deadline := time.Now().Add(3 * time.Second)
	for !rt.Session().HasPendingHTTPRead(types.ObjID(-conn.ID)) {
		if time.Now().After(deadline) {
			t.Fatal("login hook did not suspend in read_http")
		}
		time.Sleep(time.Millisecond)
	}
	if conn.IsLoggedIn() {
		t.Fatal("login hook attached a player before reading HTTP")
	}
	if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(peer, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if response, err := bufio.NewReader(peer).ReadString('\n'); err != nil || response != "GET /\r\n" {
		t.Fatalf("login hook did not return the parsed HTTP request: got %q, error %v", response, err)
	}
	// Let the server finish its deferred notification write before closing the pipe.
	deadline = time.Now().Add(3 * time.Second)
	for conn.GetLoginTaskID() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("login hook did not finish after returning the parsed request")
		}
		time.Sleep(time.Millisecond)
	}
}
