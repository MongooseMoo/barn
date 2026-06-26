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

	// Toast behavior: with no listener do_login_command verb, do_login_task
	// leaves its result at the default TYPE_INT 0, so the login predicate
	// (result is a user object) is false and the connection is NOT logged in
	// — Toast assigns no player and never substitutes a default wizard.
	// See toaststunt/src/tasks.cc:884 (default result.type = TYPE_INT) and
	// :921 (login only when `result.type == TYPE_OBJ && is_user(result.v.obj)`).
	// Barn must therefore return a negative ObjID (login refused), not #2.
	if player >= 0 {
		t.Fatalf(
			"callDoLoginCommand with no do_login_command verb returned player #%d: "+
				"login must be refused (negative ObjID) when no handler exists, matching "+
				"ToastStunt do_login_task (tasks.cc:884,921). A non-negative result here "+
				"is a security hole — unauthenticated connections become a real player.",
			player,
		)
	}
}

// TestReview_ConnectionNameLookupFailedLookupDoesNotRewrite pins the true
// ToastStunt contract for connection_name_lookup when reverse DNS fails:
//   - the call returns the deterministic numeric fallback (the raw host) with
//     NO error surfaced to MOO (Toast: network.cc:985,1593 — get_ntop / keep
//     existing name; only a missing connection is an error), and
//   - the cached connection name is NOT rewritten on a failed lookup (Toast
//     gates the rewrite on lookup success, server.cc:2980 `status == 0`).
//
// The previous code silently discarded net.LookupAddr's error and rewrote the
// stored name to the numeric fallback on EVERY failure, corrupting the value
// later returned by connection_name(). This test fails against that code.
func TestReview_ConnectionNameLookupFailedLookupDoesNotRewrite(t *testing.T) {
	cm := NewConnectionManager(7777)
	conn := cm.NewConnectionFromTransport(badAddrTransport{})

	// The remote addr "not-a-valid-addr" makes net.SplitHostPort and the
	// reverse net.LookupAddr fail in-process (no network).
	name, err := cm.ConnectionNameLookup(types.ObjID(-conn.ID), true /* rewrite */)
	if err != nil {
		t.Fatalf(
			"ConnectionNameLookup surfaced an error for a failed reverse lookup: %v; "+
				"Toast falls back to the numeric host without raising to MOO "+
				"(only a missing connection is an error)",
			err,
		)
	}
	if name != "not-a-valid-addr" {
		t.Fatalf(
			"ConnectionNameLookup returned %q; expected the deterministic numeric "+
				"fallback %q on a failed reverse lookup", name, "not-a-valid-addr",
		)
	}

	// The decisive assertion: a FAILED lookup must not overwrite the cached
	// connection name (Toast skips the rewrite unless status == 0). Old code
	// called SetResolvedName(resolved) unconditionally, leaving it set here.
	if got := conn.GetResolvedName(); got != "" {
		t.Fatalf(
			"failed reverse lookup rewrote the cached connection name to %q: "+
				"net.LookupAddr's error was silently discarded and the rewrite ran "+
				"anyway (connection_manager.go), diverging from Toast which only "+
				"rewrites on a successful lookup (server.cc:2980)", got,
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
