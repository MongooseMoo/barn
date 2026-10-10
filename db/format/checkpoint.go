package format

import (
	"fmt"
	"os"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
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
) error {
	if objectStore == nil {
		return fmt.Errorf("snapshot store is nil")
	}
	release, err := lockCheckpointPath(outPath)
	if err != nil {
		return err
	}
	defer release()
	fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
	// Recover before reusing the staging names, which may still be linked to
	// the saved generation of an interrupted first publication.
	if err := recoverCheckpointPair(outPath, fs); err != nil {
		return err
	}

	taskRoots := make([]types.Value, 0)
	collectRoot := func(value types.Value) types.Value {
		taskRoots = append(taskRoots, value)
		return value
	}
	for index := range queuedTasks {
		queuedTasks[index].TransformPersistenceValues(collectRoot)
	}
	for index := range suspendedTasks {
		suspendedTasks[index].TransformPersistenceValues(collectRoot)
	}
	snapshot, rewriter := objectStore.SnapshotWithRoots(taskRoots)
	for index := range queuedTasks {
		queuedTasks[index].TransformPersistenceValues(rewriter.Rewrite)
	}
	for index := range suspendedTasks {
		suspendedTasks[index].TransformPersistenceValues(rewriter.Rewrite)
	}

	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	writer := NewWriter(tempFile, snapshot)
	writer.SetTaskSnapshots(queuedTasks, suspendedTasks)
	writer.SetActiveConnections(activeConnections)
	if err := writer.WriteDatabase(); err != nil {
		tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write database: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	sidecarTempPath := tempPath + waifIdentitySidecarSuffix
	if err := writeWaifIdentitySidecar(sidecarTempPath, tempPath, writer.waifIdentities); err != nil {
		_ = os.Remove(tempPath)
		_ = os.Remove(sidecarTempPath)
		return err
	}

	return publishCheckpointPair(outPath, tempPath, fs)
}
