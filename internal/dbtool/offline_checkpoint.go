package dbtool

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/vm"
)

// runIdleWait is how far ahead a task's wake may lie for --run to wait for it;
// a task due later stays in the written file.
const runIdleWait = time.Second

// WriteOfflineCheckpoint loads the database at path and writes it as an
// ordinary checkpoint, path+".new" and its WAIF identity sidecar, through the
// server's checkpoint writer. Saved tasks are restored into a runtime that is
// never started and written from its task snapshots, as a running server's
// checkpoint writes them; the file's connection records are written back as
// loaded. No listener opens and no database verb is called. The input file is
// never changed. With omitSuspended, suspended and interrupted tasks are left
// out of the written file; queued (forked) tasks stay. With toastLoadable,
// only the tasks kept in Barn's frame form (no Toast form,
// Runtime.TaskSnapshotsWithRefusals) are left out, so ToastStunt can load the
// file; each is printed with its verb, object and reason.
//
// With run, the program in that file first runs as a background task of the
// database's first wizard, and every task that becomes due within runIdleWait
// runs too (restored tasks included), until none is; the checkpoint is then
// written with the tasks still waiting. Without run, no task runs.
func WriteOfflineCheckpoint(out io.Writer, path string, options config.Options, omitSuspended, toastLoadable bool, run string) (resultErr error) {
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
	fmt.Fprintf(out, "read: %d queued, %d suspended or interrupted tasks, %d connections\n",
		len(database.QueuedTasks), len(database.SuspendedTasks), len(database.ActiveConnections))
	if run != "" {
		data, err := os.ReadFile(run)
		if err != nil {
			return fmt.Errorf("read --run program: %w", err)
		}
		program, diagnostics := registry.Compiler().CompileMOO(SourceLines(data))
		if len(diagnostics) != 0 {
			return fmt.Errorf("compile --run program: %v", diagnostics[0])
		}
		wizard, ok := firstWizard(store)
		if !ok {
			return errors.New("database has no wizard to run the program as")
		}
		runtime.CreateBackgroundTask(wizard, program, 0)
		for {
			if runtime.ProcessReadyTasks() > 0 {
				continue
			}
			wake := runtime.NextTaskWake()
			if wake.IsZero() || time.Until(wake) > runIdleWait {
				break
			}
			time.Sleep(time.Until(wake))
		}
	}
	queued, suspended, refused := runtime.TaskSnapshotsWithRefusals()
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
