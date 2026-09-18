package server

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestFlushCommandReportsAndDiscardsPendingInput(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 424244, dbstore.FlagUser|dbstore.FlagWizard)

	rt := engine.NewRuntime(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)

	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
		t.Fatalf("switch player: %v", err)
	}

	ctx := kernel.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = true
	ctx.Store = store
	execution := rt.Session().NewExecution(ctx, nil)

	if result, ok := rt.Session().CallByNameWithExecution("set_connection_option", execution, []types.Value{
		types.NewObj(player), types.NewStr("hold-input"), types.NewInt(1),
	}); !ok || result.IsError() {
		t.Fatalf("set hold-input: ok=%v result=%+v", ok, result)
	}
	if result, ok := rt.Session().CallByNameWithExecution("set_connection_option", execution, []types.Value{
		types.NewObj(player), types.NewStr("flush-command"), types.NewStr(".flush"),
	}); !ok || result.IsError() {
		t.Fatalf("set flush-command: ok=%v result=%+v", ok, result)
	}

	t.Cleanup(func() {
		processor.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: ".flush"})
		rt.Session().CallByNameWithExecution("set_connection_option", execution, []types.Value{
			types.NewObj(player), types.NewStr("hold-input"), types.NewInt(0),
		})
		rt.Session().CallByNameWithExecution("set_connection_option", execution, []types.Value{
			types.NewObj(player), types.NewStr("flush-command"), types.NewStr(""),
		})
	})

	processor.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "auditflush first"})
	processor.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "auditflush second"})
	processor.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: ".FlUsH"})

	want := []string{
		">> Flushing the following pending input:",
		">>     auditflush first",
		">>     auditflush second",
		">> (Done flushing)",
	}
	if got := transport.writtenLines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("flush report = %q, want %q", got, want)
	}
}

func TestSupersededConnectionDisconnectPreservesReplacementHeldInput(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 424245, dbstore.FlagUser|dbstore.FlagWizard)

	rt := engine.NewRuntime(store)
	t.Cleanup(rt.Stop)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)

	oldConn := cm.NewConnectionFromTransport(newRecordingTransport("old"))
	if err := cm.SwitchPlayer(types.ObjID(-oldConn.ID), player); err != nil {
		t.Fatalf("switch old connection: %v", err)
	}
	replacement := cm.NewConnectionFromTransport(newRecordingTransport("replacement"))
	if err := cm.SwitchPlayer(types.ObjID(-replacement.ID), player); err != nil {
		t.Fatalf("switch replacement connection: %v", err)
	}

	ctx := kernel.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = true
	ctx.Store = store
	execution := rt.Session().NewExecution(ctx, nil)
	for _, option := range []struct {
		name  string
		value types.Value
	}{
		{name: "hold-input", value: types.NewInt(1)},
		{name: "flush-command", value: types.NewStr(".flush")},
	} {
		if result, ok := rt.Session().CallByNameWithExecution("set_connection_option", execution, []types.Value{
			types.NewObj(player), types.NewStr(option.name), option.value,
		}); !ok || result.IsError() {
			t.Fatalf("set %s: ok=%v result=%+v", option.name, ok, result)
		}
	}
	if handled, _ := rt.Session().HandleHeldInput(player, "queued after reconnect", false); !handled {
		t.Fatal("replacement input was not held")
	}

	processor.processDisconnect(command.InputEvent{ConnID: oldConn.ID})

	handled, flushed := rt.Session().HandleHeldInput(player, ".flush", false)
	if !handled || !reflect.DeepEqual(flushed, []string{"queued after reconnect"}) {
		t.Fatalf("flush after stale disconnect = handled %v, lines %q", handled, flushed)
	}
	if got := cm.GetConnection(player); got != replacement {
		t.Fatalf("active player connection = %v, want replacement %v", got, replacement)
	}
}
