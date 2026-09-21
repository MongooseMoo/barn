package server

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

const (
	evalTestPrefix = "-=!-^-!=-"
	evalTestSuffix = "-=!-v-!=-"
)

func runIntrinsicEval(t *testing.T, source string) []string {
	t.Helper()
	return runIntrinsicEvalWithBuiltins(t, source)
}

func runIntrinsicEvalWithBuiltins(t *testing.T, source string, descriptors ...builtins.Descriptor) []string {
	t.Helper()

	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)

	registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), append(vm.Descriptors(), descriptors...))
	if err != nil {
		t.Fatal(err)
	}
	rt := engine.NewRuntimeWithRegistry(store, config.DefaultOptions(), registry)
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

	for _, line := range []string{
		"PREFIX " + evalTestPrefix,
		"SUFFIX " + evalTestSuffix,
		"eval " + source,
	} {
		processor.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: line})
	}
	return transport.writtenLines()
}

func TestIntrinsicEvalFramesNotificationsAndResultInToastOrder(t *testing.T) {
	got := runIntrinsicEval(t, `notify(player, "note"); return 7;`)

	want := []string{evalTestPrefix, "note", "{1, 7}", evalTestSuffix}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("intrinsic eval output = %q, want %q", got, want)
	}
}

func TestIntrinsicEvalFramesErrorResultsExactlyOnce(t *testing.T) {
	for _, test := range []struct {
		name         string
		source       string
		bodyContains string
	}{
		{name: "compile", source: "if (", bodyContains: "Parse error"},
		{name: "runtime", source: "return 1 / 0;", bodyContains: "{2, {E_DIV,"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := runIntrinsicEval(t, test.source)
			if len(got) != 3 {
				t.Fatalf("intrinsic eval output = %q, want prefix/body/suffix", got)
			}
			if got[0] != evalTestPrefix || got[2] != evalTestSuffix {
				t.Fatalf("intrinsic eval frame = %q, want %q/body/%q", got, evalTestPrefix, evalTestSuffix)
			}
			if !strings.Contains(got[1], test.bodyContains) {
				t.Fatalf("intrinsic eval body = %q, want substring %q", got[1], test.bodyContains)
			}
		})
	}
}

func TestIntrinsicEvalFramesRecoveredPanicExactlyOnce(t *testing.T) {
	got := runIntrinsicEvalWithBuiltins(t, `eval_test_panic(); return 1;`, builtins.Descriptor{
		Name: "eval_test_panic", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0, ArgTypes: []int64{}}, Visibility: builtins.Hidden, Effect: builtins.Transactional, Capability: config.Core,
		Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
			panic("induced eval panic")
		},
	})

	want := []string{evalTestPrefix, `{0, {"Internal error: induced eval panic"}}`, evalTestSuffix}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("intrinsic eval recovered-panic output = %q, want %q", got, want)
	}
}

func TestIntrinsicEvalAllowsInjectedInputOnItsOwnConnection(t *testing.T) {
	for _, code := range []string{
		`force_input(player, "eval #0.seen=7; return 0;"); suspend(0); return #0.seen;`,
		`force_input(player, "eval #0.seen=7; suspend(0); return 0;"); suspend(0); return #0.seen;`,
		`force_input(player, "injected"); return read();`,
	} {
		t.Run(code, func(t *testing.T) {
			store := dbstore.NewStore()
			addTestObject(t, store, 0, dbstore.FlagWizard)
			player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
			if err := store.DirectTxn().DefineProperty(0, "seen", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); err != types.E_NONE {
				t.Fatal(err)
			}
			rt := engine.NewRuntime(store)
			defer rt.Stop()
			processor := NewInputProcessor(store, rt)
			defer processor.Stop()
			cm := NewConnectionManager(7777)
			processor.SetConnectionManager(cm)
			setTestConnectionManager(rt.Session(), cm)
			host := rt.Session().Host()
			host.InputForcer = processor
			rt.Session().ConfigureHost(host)
			transport := newRecordingTransport("self-input")
			conn := cm.NewConnectionFromTransport(transport)
			if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
				t.Fatal(err)
			}
			processor.Start()
			firstSlice := make(chan struct{})
			processor.EnqueueInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "eval " + code, Done: firstSlice})
			select {
			case <-firstSlice:
			case <-time.After(time.Second):
				t.Fatal("suspended eval retained its connection lane")
			}
			want := "{1, 7}"
			if strings.Contains(code, "return read()") {
				want = `{1, "injected"}`
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				for _, line := range transport.writtenLines() {
					if line == want {
						return
					}
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatalf("injected input did not complete before continuation: %q, want %q", transport.writtenLines(), want)
		})
	}
}
