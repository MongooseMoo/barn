package server

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	dbstore "barn/db/store"
	runtime "barn/scheduler"
	"barn/types"

	"github.com/coder/websocket"
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

// blockingWSConn is an in-process stand-in for *websocket.Conn whose Read
// parks until its context is cancelled — exactly how coder/websocket's real
// Read behaves. It lets us assert the wake contract without an HTTP handshake
// or a real port bind.
type blockingWSConn struct {
	reading chan struct{} // closed when Read first parks
	once    sync.Once
}

func (c *blockingWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	c.once.Do(func() { close(c.reading) })
	<-ctx.Done()
	return websocket.MessageText, nil, ctx.Err()
}

func (c *blockingWSConn) Write(context.Context, websocket.MessageType, []byte) error { return nil }
func (c *blockingWSConn) Close(websocket.StatusCode, string) error                   { return nil }

// TestReview_WebSocketWakeInputReaderInterruptsBlockedRead asserts the real
// lifecycle contract: after WakeInputReader, a WebSocket read that is parked in
// conn.Read MUST return promptly so conn.Close / graceful shutdown can proceed.
//
// Against the old no-op WakeReader() this test FAILS: WakeInputReader takes the
// WakeReader path (the transport satisfies the interface), the no-op does
// nothing, the in-flight read never unblocks, and the test times out.
func TestReview_WebSocketWakeInputReaderInterruptsBlockedRead(t *testing.T) {
	fake := &blockingWSConn{reading: make(chan struct{})}
	wst := NewWebSocketTransport(fake, "test:1234")
	conn := NewConnection(2, wst)

	done := make(chan error, 1)
	go func() {
		_, err := wst.ReadLine()
		done <- err
	}()

	// Ensure the read is actually parked inside conn.Read(ctx) before we wake it.
	select {
	case <-fake.reading:
	case <-time.After(2 * time.Second):
		t.Fatal("read never started")
	}

	conn.WakeInputReader()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked WebSocket read returned nil error after WakeInputReader; expected interruption")
		}
	case <-time.After(2 * time.Second):
		t.Fatal(
			"WakeInputReader did not interrupt the blocked WebSocket read: " +
				"WakeReader() failed to cancel the in-flight read context, so " +
				"conn.Close()/graceful shutdown cannot unblock a parked WS reader.",
		)
	}

	// Secondary: the deadline is stamped so a read starting after the wake also
	// returns immediately (matches TCPTransport's SetReadDeadline(time.Now())).
	if d := wst.readDeadline(); d.IsZero() {
		t.Fatal("WakeReader left readDeadline at zero")
	}
}
