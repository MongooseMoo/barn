package format

import (
	"errors"
	"fmt"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
)

// WriteCheckpoint writes a database checkpoint to path+".new", never modifying path.
func WriteCheckpoint(
	path string,
	objectStore *store.Store,
	queuedTasks, suspendedTasks []task.Snapshot,
	activeConnections []ActiveConnection,
) error {
	return writeCheckpoint(path+".new", path+".tmp", objectStore, queuedTasks, suspendedTasks, activeConnections)
}

// WritePanicCheckpoint writes an emergency database checkpoint to
// path+".new.PANIC", leaving both the input database and the ordinary
// checkpoint path untouched.
func WritePanicCheckpoint(
	path string,
	objectStore *store.Store,
	queuedTasks, suspendedTasks []task.Snapshot,
	activeConnections []ActiveConnection,
) error {
	return writeCheckpoint(path+".new.PANIC", path+".tmp.PANIC", objectStore, queuedTasks, suspendedTasks, activeConnections)
}

func writeCheckpoint(
	outPath, tempPath string,
	objectStore *store.Store,
	queuedTasks, suspendedTasks []task.Snapshot,
	activeConnections []ActiveConnection,
) (resultErr error) {
	if objectStore == nil {
		return fmt.Errorf("snapshot store is nil")
	}
	checkpoint, err := beginCheckpoint(outPath, tempPath)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, checkpoint.Close()) }()
	if err := checkpoint.Serialize(objectStore, queuedTasks, suspendedTasks, activeConnections); err != nil {
		return err
	}
	return checkpoint.Publish()
}
