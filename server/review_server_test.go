package server

import (
	"io"
	"testing"
	"time"

	dbstore "barn/db/store"
	runtime "barn/scheduler"
	"barn/types"
)

// badAddrTransport is a Transport whose RemoteAddr returns an unparseable address.
// Used to probe error-handling paths in ConnectionNameLookup.
type badAddrTransport struct{}

func (badAddrTransport) ReadLine() (string, error) { return "", io.EOF }
func (badAddrTransport) WriteLine(string) error    { return nil }
func (badAddrTransport) Close() error              { return nil }
func (badAddrTransport) RemoteAddr() string        { return "not-a-valid-addr" }

// TestReview_FallbackLoginReturnsWizardWithNoLoginHandler demonstrates that
// callDoLoginCommand returns the wizard object (#2) when the listener handler
// has no do_login_command verb — giving every unauthenticated connection
// instant wizard access. The correct behavior is to refuse login, not grant it.
func TestReview_FallbackLoginReturnsWizardWithNoLoginHandler(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	// Listener object (#10) intentionally has NO do_login_command verb.
	addTestObject(t, store, 10, dbstore.FlagWizard)

	rt := runtime.NewScheduler(store)
	s := NewInputProcessor(store, rt)
	conn := NewConnection(2, stubTransport{})
	conn.SetListener(10, 7789, true)

	player, _ := s.callDoLoginCommand(conn, "connect wizard")

	// WANT: login refused (player < 0) when no handler is defined.
	// GOT: hardcoded return of player #2 (the wizard) — any connection becomes wizard.
	if player == 2 {
		t.Fatalf(
			"callDoLoginCommand with no do_login_command verb returned player #%d (wizard): "+
				"security hole — unauthenticated connections get instant wizard access "+
				"(input_login.go:47 hardcoded fallback `return types.ObjID(2), nil`)",
			player,
		)
	}
}

// TestReview_ConnectionNameLookupNeverErrors demonstrates that
// ConnectionNameLookup always returns a nil error, even when the connection's
// remote address is malformed and the lookup fails. Callers expecting error
// propagation get a silent partial result instead.
func TestReview_ConnectionNameLookupNeverErrors(t *testing.T) {
	cm := NewConnectionManager(7777)
	conn := cm.NewConnectionFromTransport(badAddrTransport{})

	// The remote addr "not-a-valid-addr" will cause net.SplitHostPort to fail
	// and net.LookupAddr to fail. ConnectionNameLookup should surface that error
	// rather than always returning (resolved, nil).
	_, err := cm.ConnectionNameLookup(types.ObjID(-conn.ID), false)
	if err == nil {
		t.Fatal(
			"ConnectionNameLookup returned nil error for an invalid remote address: " +
				"errors from net.SplitHostPort and net.LookupAddr are silently discarded " +
				"(connection_manager.go:739 always returns `resolved, nil`)",
		)
	}
}

// TestReview_WebSocketWakeInputReaderDoesNotSetDeadline demonstrates that
// WakeInputReader on a WebSocket-backed connection fails to interrupt a
// blocking read. WebSocketTransport.WakeReader() is a no-op, which prevents
// the fallback to SetReadDeadline(time.Now()) in WakeInputReader.
// Consequence: graceful per-connection shutdown cannot unblock a WS reader.
func TestReview_WebSocketWakeInputReaderDoesNotSetDeadline(t *testing.T) {
	// NewWebSocketTransport with nil conn — we test only deadline-setting behavior,
	// not actual network I/O.
	wst := NewWebSocketTransport(nil, "test:1234")
	conn := NewConnection(2, wst)

	// After WakeInputReader, the WebSocket transport's deadline should be set to
	// time.Now() (or past), so that any in-progress conn.Read(ctx) times out.
	// If WakeReader() is a no-op and falls through to SetReadDeadline is skipped,
	// the deadline stays zero and blocking reads are never interrupted.
	conn.WakeInputReader()

	deadline := wst.readDeadline()
	if deadline.IsZero() {
		t.Fatal(
			"WakeInputReader on a WebSocketTransport left readDeadline at zero: " +
				"WakeReader() is a no-op (websocket_transport.go:85) and satisfies the " +
				"WakeReader interface, preventing fallthrough to SetReadDeadline(time.Now()). " +
				"Result: conn.Close() cannot interrupt a blocked WebSocket read.",
		)
	}
	// Deadline should be <= now (in the past or present) to actually wake a reader.
	if deadline.After(time.Now().Add(time.Second)) {
		t.Fatalf("readDeadline %v is in the future — would not interrupt a current read", deadline)
	}
}
