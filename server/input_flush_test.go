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
	rt.Registry().SetConnectionManager(cm)

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
	ctx.Registry = rt.Registry()

	if result, ok := rt.Registry().CallByName("set_connection_option", ctx, []types.Value{
		types.NewObj(player), types.NewStr("hold-input"), types.NewInt(1),
	}); !ok || result.IsError() {
		t.Fatalf("set hold-input: ok=%v result=%+v", ok, result)
	}
	if result, ok := rt.Registry().CallByName("set_connection_option", ctx, []types.Value{
		types.NewObj(player), types.NewStr("flush-command"), types.NewStr(".flush"),
	}); !ok || result.IsError() {
		t.Fatalf("set flush-command: ok=%v result=%+v", ok, result)
	}

	t.Cleanup(func() {
		processor.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: ".flush"})
		rt.Registry().CallByName("set_connection_option", ctx, []types.Value{
			types.NewObj(player), types.NewStr("hold-input"), types.NewInt(0),
		})
		rt.Registry().CallByName("set_connection_option", ctx, []types.Value{
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
