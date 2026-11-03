package server

import (
	"slices"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

// A command whose slice has run long enough that yin() yields it for 0
// seconds must resume through the server's own dispatch loop with no further
// input, as Toast resumes a suspend(0) task on its next scheduler pass.
func TestYieldedCommandResumesWithoutFurtherInput(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
	store.AddVerb(player, dbstore.NewVerb([]string{"spin"}, player,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{
			"start = ticks_left();",
			"while (start - ticks_left() < 12000) endwhile",
			"yin();",
			`notify(player, "after yin");`,
		}))
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
		t.Fatal(err)
	}
	// scripts/bench_players.py's completion probe: an out-of-band command forks
	// a task that yields while the connection's last input task is alive.
	store.AddVerb(0, dbstore.NewVerb([]string{"do_out_of_band_command"}, player,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			`input_task = queue_info(player)["last_input_task_id"];`,
			"fork (0)",
			"while (input_task > 0 && typeof(`task_stack(input_task) ! E_INVARG') == LIST)",
			"suspend(0);",
			"endwhile",
			`notify(player, "probe done");`,
			"endfork",
		}))
	processor.Start()

	done := make(chan struct{})
	processor.EnqueueInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "spin", Done: done})
	processor.EnqueueInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "#$#probe"})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("command's first slice did not return")
	}
	waitForOutput(t, transport, "after yin")
}

// A line that arrives after a login logs its connection in is a command, even
// when the login finished in a slice that resumed after yin() yielded it.
// Toast picks do_command_task or do_login_task when the queued line runs, from
// the connection's player at that moment (tasks.cc run_ready_tasks), not when
// the line was read.
func TestLineAfterYieldedLoginIsACommand(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
	store.AddVerb(0, dbstore.NewVerb([]string{"do_login_command"}, player,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			`if (args == {"connect"})`,
			"start = ticks_left();",
			"while (start - ticks_left() < 12000) endwhile",
			"yin();",
			"switch_player(player, #2);",
			"endif",
			"return 0;",
		}))
	store.AddVerb(player, dbstore.NewVerb([]string{"ping"}, player,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{`notify(player, "pong");`}))
	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	processor.Start()
	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	go processor.HandleConnection(conn)
	defer transport.Close()

	transport.readLine <- "connect"
	deadline := time.Now().Add(5 * time.Second)
	for !conn.IsLoggedIn() {
		if time.Now().After(deadline) {
			t.Fatal("yielded login never switched the connection to its player")
		}
		time.Sleep(time.Millisecond)
	}
	transport.readLine <- "ping"
	waitForOutput(t, transport, "pong")
}

func waitForOutput(t *testing.T, transport *recordingTransport, line string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Contains(transport.writtenLines(), line) {
		if time.Now().After(deadline) {
			t.Fatalf("no %q; output = %q", line, transport.writtenLines())
		}
		time.Sleep(time.Millisecond)
	}
}
