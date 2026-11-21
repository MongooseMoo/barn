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
	_, err := writeCheckpoint(path+".new", path+".tmp", objectStore, queuedTasks, suspendedTasks, activeConnections, false)
	return err
}

// WriteToastLoadableCheckpoint is WriteCheckpoint with OmitBarnFrameTasks: it
// leaves out every task in Barn's frame form, so ToastStunt can load the file,
// and returns their ids.
func WriteToastLoadableCheckpoint(
	path string,
	objectStore *store.Store,
	queuedTasks, suspendedTasks []task.Snapshot,
	activeConnections []ActiveConnection,
) ([]int64, error) {
	return writeCheckpoint(path+".new", path+".tmp", objectStore, queuedTasks, suspendedTasks, activeConnections, true)
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
	_, err := writeCheckpoint(path+".new.PANIC", path+".tmp.PANIC", objectStore, queuedTasks, suspendedTasks, activeConnections, false)
	return err
}

func writeCheckpoint(
	outPath, tempPath string,
	objectStore *store.Store,
	queuedTasks, suspendedTasks []task.Snapshot,
	activeConnections []ActiveConnection,
	toastLoadable bool,
) (omitted []int64, resultErr error) {
	if objectStore == nil {
		return nil, fmt.Errorf("snapshot store is nil")
	}
	checkpoint, err := beginCheckpoint(outPath, tempPath)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, checkpoint.Close()) }()
	if toastLoadable {
		checkpoint.OmitBarnFrameTasks()
	}
	if err := checkpoint.Capture(objectStore, queuedTasks, suspendedTasks, activeConnections); err != nil {
		return nil, err
	}
	if err := checkpoint.Serialize(); err != nil {
		return nil, err
	}
	return checkpoint.Omitted(), checkpoint.Publish()
}
