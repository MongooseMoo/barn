package engine

import (
	"errors"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Issue #297: #0:do_command dispatches `home`, whose move() runs a room
// enterfunc that suspends. Toast runs do_command as a task
// (tasks.cc do_command_task), so the suspend is honored, the hook resumes
// through the scheduler, and the command counts as handled
// (outcome != OUTCOME_DONE || is_true(result)).
func TestDoCommandTaskResumesAfterEnterfuncSuspend(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	const (
		room   types.ObjID = 10
		start  types.ObjID = 11
		player types.ObjID = 2
	)
	addServerVerbTestObject(t, store, room, 0)
	addServerVerbTestObject(t, store, start, 0)
	if errCode := store.DirectTxn().MoveObject(player, start, 0); errCode != types.E_NONE {
		t.Fatalf("initial move: %s", errCode)
	}
	if errCode := store.DirectTxn().DefineProperty(0, "trace", dbstore.NewProperty(
		types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true,
	)); errCode != types.E_NONE {
		t.Fatalf("define trace: %s", errCode)
	}
	addMoveHookTestVerb(t, store, 0, "do_command",
		"caller && raise(E_PERM);",
		`#0.trace = {@#0.trace, {"hook", verb, argstr}};`,
		"x = `player:(args[1])(@args) ! ANY => 0';",
		"return x;",
	)
	addMoveHookTestVerb(t, store, player, "home",
		`#0.trace = {@#0.trace, "home-start"};`,
		"move(player, #10);",
		`#0.trace = {@#0.trace, "home-done"};`,
		"return 1;",
	)
	addMoveHookTestVerb(t, store, room, "enterfunc", "this:loc_enter(args[1], this);")
	addMoveHookTestVerb(t, store, room, "loc_enter",
		`#0.trace = {@#0.trace, "enter-before"};`,
		"suspend(0);",
		`#0.trace = {@#0.trace, "enter-after"};`,
	)

	rt := NewRuntime(store)
	var startedID int64
	args := []types.Value{types.NewStr("home")}
	res, err := rt.RunServerVerbTaskWithArgstr(0, "do_command", args, player, "home", func(id int64) { startedID = id })
	if err != nil {
		t.Fatalf("RunServerVerbTaskWithArgstr: %v", err)
	}
	if startedID == 0 {
		t.Fatalf("onStart was not given the task ID")
	}
	// The hook reached suspend(0): the caller sees a blocked outcome, and the
	// task is still registered for the scheduler.
	if res.Flow != types.FlowSuspend {
		t.Fatalf("result flow = %v, want FlowSuspend", res.Flow)
	}

	// Drive the scheduler until the suspended hook finishes.
	for i := 0; i < 10 && rt.ProcessReadyTasks() > 0; i++ {
	}

	got, errCode := store.DirectTxn().PropertyValue(0, "trace")
	if errCode != types.E_NONE {
		t.Fatalf("read trace: %s", errCode)
	}
	want := types.NewList([]types.Value{
		types.NewList([]types.Value{types.NewStr("hook"), types.NewStr("do_command"), types.NewStr("home")}),
		types.NewStr("home-start"),
		types.NewStr("enter-before"),
		types.NewStr("enter-after"),
		types.NewStr("home-done"),
	})
	if !got.Equal(want) {
		t.Fatalf("trace = %s, want %s", got.String(), want.String())
	}
	if loc, _ := store.DirectTxn().Location(player); loc != room {
		t.Fatalf("player location = #%d, want #%d", loc, room)
	}
}

func TestRunServerVerbTaskWithArgstrReportsMissingVerb(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	rt := NewRuntime(store)
	_, err := rt.RunServerVerbTaskWithArgstr(0, "do_command", nil, 0, "", nil)
	if !errors.Is(err, ErrServerVerbNotFound) {
		t.Fatalf("err = %v, want ErrServerVerbNotFound", err)
	}
}
