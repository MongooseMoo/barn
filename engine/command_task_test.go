package engine

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestExecuteVerbTaskSyncReturnsRecoveredTaskPanic(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	store.AddVerb(2, dbstore.NewVerb("panic-command", []string{"panic-command"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{"panic_for_command_test();"}))

	runtime := NewRuntime(store)
	runtime.Registry().Register("panic_for_command_test", func(_ *builtins.Execution, _ []types.Value) types.Result {
		panic("command task exploded")
	})

	cmd := command.ParseCommand("panic-command")
	match := command.FindVerb(store, 2, 2, cmd)
	if match == nil {
		t.Fatal("command verb was not found")
	}

	err := runtime.ExecuteVerbTaskSync(2, match, cmd, "")
	if err == nil || !strings.Contains(err.Error(), "internal panic: command task exploded") {
		t.Fatalf("ExecuteVerbTaskSync error = %v, want recovered panic", err)
	}
}
