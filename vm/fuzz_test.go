package vm

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/task"
)

func FuzzSourceExecution(f *testing.F) {
	fuzzseed.Add(f)
	f.Fuzz(func(t *testing.T, source string) {
		executeFuzzSource(t, source, false)
	})
}

// FuzzStructuredSourceExecution runs generated programs. Each is valid MOO,
// so every case reaches execution; a compile diagnostic fails the case.
func FuzzStructuredSourceExecution(f *testing.F) {
	fuzzseed.AddChoices(f)
	f.Fuzz(func(t *testing.T, choices []byte) {
		executeFuzzSource(t, fuzzseed.StructuredSource(t, choices), true)
	})
}

func executeFuzzSource(t *testing.T, source string, mustCompile bool) {
	// Core capabilities exclude optional network, SQLite and process input.
	// State and compiler caches are private to this input, with no server.
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		if mustCompile {
			t.Fatalf("generated program does not compile: %v\nsource:\n%s", diagnostics, source)
		}
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
	// A named while loop charges only unchecked ticks (Toast EOP_WHILE_ID), so
	// only the seconds limit stops it; that limit runs from this deadline.
	machine.Task.SetExecutionDeadline(time.Now().Add(100 * time.Millisecond))
	machine.TickLimit = context.TicksRemaining
	// Errors, exhaustion and yields are results; a Go panic fails the case.
	// Do not resume yields or run forked tasks: those belong to a scheduler.
	_ = machine.Run(program)
}
