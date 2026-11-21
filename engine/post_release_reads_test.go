package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Toast do_command_task (tasks.cc) runs the command task until it returns or
// suspends, then sends the connection's output suffix, before any resumption
// of that task can run. The suffix must therefore precede output from the
// task's resumed slice, even when a scheduler worker resumes the task as soon
// as its execution lease is released. The output flusher drains the scheduler
// before recording the suffix, so a flush issued after the lease release lets
// the resumed slice's notify land first.
func TestCommandSuffixPrecedesResumedSliceOutput(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	const player types.ObjID = 2
	addServerVerbTestObject(t, store, player, dbstore.FlagUser|dbstore.FlagWizard)
	if _, errCode := store.AddVerb(player, dbstore.NewVerb(
		[]string{"probe"}, player,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{`suspend(0);`, `notify(player, "after");`},
	)); errCode != types.E_NONE {
		t.Fatalf("add probe: %s", errCode)
	}
	verb, defObj, err := store.DirectTxn().FindCallableVerb(player, "probe")
	if err != nil {
		t.Fatalf("find probe: %v", err)
	}

	rt := NewRuntime(store)
	t.Cleanup(rt.Stop)
	conn := &evalCommandStubConn{id: 1}
	configureTestHost(rt.Session(), func(host *builtins.Host) {
		host.ConnManager = &evalCommandStubConnManager{player: player, conn: conn}
	})
	draining := false
	rt.SetTaskOutputFlusher(func(_ types.ObjID, suffix string) {
		if !draining {
			draining = true
			for i := 0; i < 10 && rt.ProcessReadyTasks() > 0; i++ {
			}
			draining = false
		}
		if suffix != "" {
			conn.sent = append(conn.sent, suffix)
		}
	})

	cmd := command.ParseCommand("probe")
	match := &command.VerbMatch{Verb: verb, This: player, VerbLoc: defObj}
	if err := rt.ExecuteVerbTaskSyncWithStart(player, match, cmd, "SUFFIX", nil); err != nil {
		t.Fatalf("ExecuteVerbTaskSyncWithStart: %v", err)
	}
	for i := 0; i < 10 && rt.ProcessReadyTasks() > 0; i++ {
	}
	if len(conn.sent) != 2 || conn.sent[0] != "SUFFIX" || conn.sent[1] != "after" {
		t.Fatalf("sent = %#v, want [SUFFIX after]", conn.sent)
	}
}
