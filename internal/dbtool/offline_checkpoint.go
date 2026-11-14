package dbtool

import (
	"errors"
	"fmt"
	"io"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/vm"
)

// WriteOfflineCheckpoint loads the database at path and writes it as an
// ordinary checkpoint, path+".new" and its WAIF identity sidecar, through the
// server's checkpoint writer. Saved tasks are restored into a runtime that is
// never started and written from its task snapshots, as a running server's
// checkpoint writes them; the file's connection records are written back as
// loaded. No task runs, no listener opens and no database verb is called. The
// input file is never changed. With omitSuspended, suspended and interrupted
// tasks are left out of the written file; queued (forked) tasks stay.
func WriteOfflineCheckpoint(out io.Writer, path string, options config.Options, omitSuspended bool) (resultErr error) {
	database, err := dbformat.LoadDatabaseReadOnly(path)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}
	registry, err := builtins.NewRegistryFromDescriptors(options.Capabilities(), vm.Descriptors())
	if err != nil {
		return fmt.Errorf("construct builtin registry: %w", err)
	}
	database.LowerUnknownBuiltinCalls(registry.Compiler())
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		return fmt.Errorf("construct store from database: %w", err)
	}
	runtime := engine.NewRuntimeWithRegistry(store, options, registry)
	defer func() {
		runtime.Stop()
		resultErr = errors.Join(resultErr, runtime.Session().Close())
	}()
	runtime.Session().LoadServerOptionsFromStore(store)
	runtime.LoadQueuedTasks(database.QueuedTasks)
	if err := runtime.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		return fmt.Errorf("restore suspended tasks: %w", err)
	}
	queued, suspended := runtime.TaskSnapshots()
	fmt.Fprintf(out, "read: %d queued, %d suspended or interrupted tasks, %d connections\n",
		len(database.QueuedTasks), len(database.SuspendedTasks), len(database.ActiveConnections))
	if omitSuspended {
		suspended = nil
	}
	if err := dbformat.WriteCheckpoint(path, store, queued, suspended, database.ActiveConnections); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	fmt.Fprintf(out, "wrote %s: %d queued, %d suspended or interrupted tasks, %d connections\n",
		path+".new", len(queued), len(suspended), len(database.ActiveConnections))
	return nil
}
