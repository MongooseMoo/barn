package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/task"
)

func FuzzSourceExecution(f *testing.F) {
	fuzzseed.Add(f)
	f.Fuzz(func(t *testing.T, source string) {
		// Core capabilities exclude optional network, SQLite and process input.
		// State and compiler caches are private to this input, with no server.
		registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
		if err != nil {
			t.Fatal(err)
		}
		program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
		if len(diagnostics) != 0 {
			return
		}
		session := newTestSessionWithTaskManager(registry)
		defer func() {
			if err := session.Close(); err != nil {
				t.Error(err)
			}
		}()
		context := task.NewContext()
		context.TicksRemaining = 100
		machine := NewVM(dbstore.NewStore(), session)
		machine.Context = context
		machine.Task = task.NewState(1, context.Player, context.TicksRemaining, 1)
		machine.TickLimit = context.TicksRemaining
		// Errors, exhaustion and yields are results; a Go panic fails the case.
		// Do not resume yields or run forked tasks: those belong to a scheduler.
		_ = machine.Run(program)
	})
}
