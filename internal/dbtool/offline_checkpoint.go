package dbtool

import (
	"errors"
	"fmt"
	"io"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/vm"
)

// WriteOfflineCheckpoint loads the database at path and writes it as an
// ordinary checkpoint, path+".new" and its WAIF identity sidecar, through the
// server's checkpoint writer. Saved tasks are restored into a runtime that is
// never started and written from its task snapshots, as a running server's
// checkpoint writes them; the file's connection records are written back as
// loaded. No task runs, no listener opens and no database verb is called. The
// input file is never changed. With omitSuspended, suspended and interrupted
// tasks are left out of the written file; queued (forked) tasks stay. With
// toastLoadable, only the tasks kept in Barn's frame form (no Toast form,
// Runtime.TaskSnapshotsWithRefusals) are left out, so ToastStunt can load the
// file; each is printed with its verb, object and reason.
func WriteOfflineCheckpoint(out io.Writer, path string, options config.Options, omitSuspended, toastLoadable bool) (resultErr error) {
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
	queued, suspended, refused := runtime.TaskSnapshotsWithRefusals()
	fmt.Fprintf(out, "read: %d queued, %d suspended or interrupted tasks, %d connections\n",
		len(database.QueuedTasks), len(database.SuspendedTasks), len(database.ActiveConnections))
	if omitSuspended {
		suspended = nil
	}
	if !toastLoadable {
		if err := dbformat.WriteCheckpoint(path, store, queued, suspended, database.ActiveConnections); err != nil {
			return fmt.Errorf("write checkpoint: %w", err)
		}
		fmt.Fprintf(out, "wrote %s: %d queued, %d suspended or interrupted tasks, %d connections\n",
			path+".new", len(queued), len(suspended), len(database.ActiveConnections))
		return nil
	}
	byID := make(map[int64]task.Snapshot, len(suspended))
	for _, snapshot := range suspended {
		byID[snapshot.ID] = snapshot
	}
	omitted, err := dbformat.WriteToastLoadableCheckpoint(path, store, queued, suspended, database.ActiveConnections)
	if err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	for _, id := range omitted {
		snapshot := byID[id]
		reason := "kept in Barn's frame form"
		if err := refused[id]; err != nil {
			reason = err.Error()
		}
		fmt.Fprintf(out, "omitted task %d: #%d:%s, this #%d: %s\n", id, snapshot.VerbLoc, snapshot.VerbName, snapshot.This, reason)
	}
	fmt.Fprintf(out, "wrote %s: %d queued, %d suspended or interrupted tasks, %d connections, %d tasks omitted for Toast\n",
		path+".new", len(queued), len(suspended)-len(omitted), len(database.ActiveConnections), len(omitted))
	return nil
}
