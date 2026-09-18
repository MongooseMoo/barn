package server

import (
	"testing"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// loginTestSetup builds a store + input processor + connection manager wired so
// read()-based login can run through the input pipeline. The returned conn is a
// fresh unlogged connection registered with the manager.
func loginTestSetup(t *testing.T, loginVerb []string) (*InputProcessor, *Connection, *dbstore.Store) {
	t.Helper()
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	listener := addTestObject(t, store, 10, dbstore.FlagWizard)
	addTestVerb(store, listener, "do_login_command", loginVerb...)

	rt := engine.NewRuntime(store)
	t.Cleanup(rt.Stop)
	s := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	s.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)

	conn := cm.NewConnectionFromTransport(stubTransport{})
	conn.SetListener(10, 7789, true)
	return s, conn, store
}

// countReadingTasks returns the number of suspended tasks reading from player.
func countReadingTasks(rt *engine.Runtime, player types.ObjID) int {
	n := 0
	queued, suspended := rt.TaskSnapshots()
	for _, snapshot := range append(queued, suspended...) {
		if snapshot.State == task.TaskSuspended && snapshot.ReadingPlayer == player {
			n++
		}
	}
	return n
}

// countLiveLoginTasks returns login-hook tasks (Owner == the pre-login connID
// player) that are neither completed nor killed.
func countLiveLoginTasks(rt *engine.Runtime, player types.ObjID) int {
	n := 0
	queued, suspended := rt.TaskSnapshots()
	for _, snapshot := range append(queued, suspended...) {
		if snapshot.Owner != player {
			continue
		}
		if snapshot.State != task.TaskCompleted && snapshot.State != task.TaskKilled {
			n++
		}
	}
	return n
}

// TestReadBasedLoginResumesAndConnects covers the single-read happy path: the
// login verb prompts for a username via read(), then returns the player. No
// scheduler pump between lines — the resume must drive the task synchronously to
// completion.
func TestReadBasedLoginResumesAndConnects(t *testing.T) {
	s, conn, _ := loginTestSetup(t, []string{
		`notify(player, "Enter your username:");`,
		`name = read();`,
		`if (name == "q")`,
		`  return #2;`,
		`endif`,
		`return #-1;`,
	})
	connID := conn.ID
	preLoginPlayer := types.ObjID(-connID)

	deliver := func(line string) {
		s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: line})
	}

	deliver("connect")
	deliver("q")

	if got := conn.GetPlayer(); got != 2 {
		t.Fatalf("connection player = #%d, want #2 (read()-based login did not complete)", got)
	}
	if !conn.IsLoggedIn() {
		t.Fatalf("connection not logged in after read()-based login")
	}
}

// TestReadBasedLoginTwoReadsUsernameThenPassword covers the actual mongoose
// flow: read() the username, read() the password, THEN connect. Each read()
// suspends and resumes the SAME task across two input round-trips.
func TestReadBasedLoginTwoReadsUsernameThenPassword(t *testing.T) {
	s, conn, _ := loginTestSetup(t, []string{
		`notify(player, "Enter your username:");`,
		`name = read();`,
		`notify(player, "Password");`,
		`pass = read();`,
		`if (name == "q" && pass == "canefan")`,
		`  return #2;`,
		`endif`,
		`return #-1;`,
	})
	connID := conn.ID
	preLoginPlayer := types.ObjID(-connID)

	deliver := func(line string) {
		s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: line})
	}

	deliver("connect") // -> read() username, suspends
	deliver("q")       // -> resumes, read() password, suspends again
	deliver("canefan") // -> resumes, returns #2, connects

	if got := conn.GetPlayer(); got != 2 {
		t.Fatalf("connection player = #%d, want #2 (two-read login did not complete)", got)
	}
	if !conn.IsLoggedIn() {
		t.Fatalf("connection not logged in after two-read login")
	}
	if n := countReadingTasks(s.runtime, preLoginPlayer); n != 0 {
		t.Fatalf("after login completed, %d task(s) still reading from #%d, want 0", n, preLoginPlayer)
	}
}

// TestReadBasedLoginNoParallelSpawnRace reproduces the analyst's race: several
// input lines arrive with NO scheduler pump between them. Only ONE login task
// may exist at a time — a line arriving while a login task is in flight must be
// routed to it (as the answer to its read()), never spawn a parallel
// do_login_command.
func TestReadBasedLoginNoParallelSpawnRace(t *testing.T) {
	s, conn, _ := loginTestSetup(t, []string{
		`notify(player, "Enter your username:");`,
		`name = read();`,
		`notify(player, "Password");`,
		`pass = read();`,
		`if (name == "q" && pass == "canefan")`,
		`  return #2;`,
		`endif`,
		`return #-1;`,
	})
	connID := conn.ID
	preLoginPlayer := types.ObjID(-connID)

	deliver := func(line string) {
		s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: line})
	}

	deliver("connect")
	if n := countLiveLoginTasks(s.runtime, preLoginPlayer); n > 1 {
		t.Fatalf("after first line, %d live login tasks for #%d, want <=1 (parallel spawn)", n, preLoginPlayer)
	}
	deliver("q")
	if n := countLiveLoginTasks(s.runtime, preLoginPlayer); n > 1 {
		t.Fatalf("after second line, %d live login tasks for #%d, want <=1 (parallel spawn)", n, preLoginPlayer)
	}
	deliver("canefan")

	if got := conn.GetPlayer(); got != 2 {
		t.Fatalf("connection player = #%d, want #2 (race broke login)", got)
	}
	if n := countLiveLoginTasks(s.runtime, preLoginPlayer); n != 0 {
		t.Fatalf("after login, %d live login tasks for #%d, want 0", n, preLoginPlayer)
	}
}

// TestReadBasedLoginDisconnectMidReadLeavesNoOrphan verifies that disconnecting
// while the login task is suspended on read() cancels the task, so no orphan
// lingers to swallow input for a connID later reused by another connection.
func TestReadBasedLoginDisconnectMidReadLeavesNoOrphan(t *testing.T) {
	s, conn, _ := loginTestSetup(t, []string{
		`notify(player, "Enter your username:");`,
		`name = read();`,
		`return (name == "q") ? #2 | #-1;`,
	})
	cm := s.connManager
	connID := conn.ID
	preLoginPlayer := types.ObjID(-connID)

	// First line: login task runs and suspends on read().
	s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: "connect"})
	if n := countReadingTasks(s.runtime, preLoginPlayer); n != 1 {
		t.Fatalf("after connect, %d reading tasks, want 1", n)
	}

	// Disconnect mid-read.
	s.processInput(command.InputEvent{ConnID: connID, IsDisconnect: true})

	if n := countReadingTasks(s.runtime, preLoginPlayer); n != 0 {
		t.Fatalf("after disconnect, %d task(s) still reading from #%d, want 0 (orphan)", n, preLoginPlayer)
	}

	// A new connection reusing the same connID must not have its first input
	// swallowed by a dead orphan. Simulate connID reuse.
	conn2 := NewConnection(connID, stubTransport{})
	conn2.SetListener(10, 7789, true)
	cm.mu.Lock()
	cm.connections[connID] = conn2
	cm.playerConns[types.ObjID(-connID)] = conn2
	cm.mu.Unlock()

	// The reused connection drives a fresh login: first line starts
	// do_login_command (which read()s), second line answers the read with "q".
	// If the orphan had survived, the FIRST line below would be swallowed by it
	// instead of starting a new do_login_command, and login would never complete.
	s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: "start"})
	s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: "q"})
	if got := conn2.GetPlayer(); got != 2 {
		t.Fatalf("reused-connID connection player = #%d, want #2 (orphan swallowed first input)", got)
	}
}

// TestReadBasedLoginVerbRaiseMidFlow verifies that a login verb raising an
// uncaught error mid-flow (after a read()) clears the in-flight task without
// logging in and without leaving an orphan or hanging.
func TestReadBasedLoginVerbRaiseMidFlow(t *testing.T) {
	s, conn, _ := loginTestSetup(t, []string{
		`name = read();`,
		`return name[5];`, // E_RANGE when name shorter than 5 chars
	})
	connID := conn.ID
	preLoginPlayer := types.ObjID(-connID)

	s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: "connect"})
	s.processInput(command.InputEvent{ConnID: connID, Player: preLoginPlayer, Line: "q"})

	if conn.IsLoggedIn() {
		t.Fatalf("connection logged in despite login verb raising")
	}
	if got := conn.GetLoginTaskID(); got != 0 {
		t.Fatalf("loginTaskID = %d after verb raised, want 0", got)
	}
	if n := countLiveLoginTasks(s.runtime, preLoginPlayer); n != 0 {
		t.Fatalf("%d live login tasks for #%d after verb raised, want 0 (orphan/hang)", n, preLoginPlayer)
	}
}
