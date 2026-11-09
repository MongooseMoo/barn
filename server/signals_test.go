package server

import (
	"context"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

func TestUserSignalHookOutcomeAndContext(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		checkpoint bool
	}{
		{"absent", "", true},
		{"false_integer", "return 0;", true},
		{"false_string", `return "";`, true},
		{"false_list", "return {};", true},
		{"true_integer", "return 1;", false},
		{"true_string", `return "handled";`, false},
		{"true_list", "return {1};", false},
		{"aborted", "raise(E_PERM);", false},
		{"suspended", "suspend(10); return 0;", false},
	} {
		for _, name := range []string{"SIGUSR1", "SIGUSR2"} {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				store := dbstore.NewStore()
				system := addTestObject(t, store, 0, dbstore.FlagWizard)
				addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
				if code := store.DirectTxn().DefineProperty(system, "signal_context", dbstore.NewProperty(types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
					t.Fatalf("define context: %v", code)
				}
				if tc.code != "" {
					store.AddVerb(system, dbstore.NewVerb([]string{"handle_signal"}, 2,
						dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
						dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
						[]string{`#0.signal_context = {args, player, this, verb, task_perms()};`, tc.code}))
				}
				rt := engine.NewRuntime(store)
				t.Cleanup(rt.Stop)
				s := &Server{store: store, runtime: rt, ctx: context.Background(), checkpointChan: make(chan struct{}, 1)}
				for range 2 {
					if err := s.handleUserSignal(name); err != nil {
						t.Fatalf("signal: %v", err)
					}
					want := 0
					if tc.checkpoint && name == "SIGUSR2" {
						want = 1
					}
					if got := len(s.checkpointChan); got != want {
						t.Fatalf("checkpoint requests = %d, want %d", got, want)
					}
					if want != 0 {
						<-s.checkpointChan
					}
				}
				if tc.code != "" {
					got, code := store.DirectTxn().PropertyValue(system, "signal_context")
					want := types.NewList([]types.Value{
						types.NewList([]types.Value{types.NewStr(name)}),
						types.NewObj(-1), types.NewObj(0), types.NewStr("handle_signal"), types.NewObj(2),
					})
					if code != types.E_NONE || !got.Equal(want) {
						t.Fatalf("context = %s (%v), want %s", got.String(), code, want.String())
					}
				}
			})
		}
	}
}
