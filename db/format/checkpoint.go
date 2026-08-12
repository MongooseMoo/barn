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
		os.Remove(tempPath)
		return fmt.Errorf("write database: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	sidecarTempPath := tempPath + waifIdentitySidecarSuffix
	if err := writeWaifIdentitySidecar(sidecarTempPath, tempPath, writer.waifIdentities); err != nil {
		os.Remove(tempPath)
		os.Remove(sidecarTempPath)
		return err
	}

	if err := os.Rename(tempPath, outPath); err != nil {
		os.Remove(sidecarTempPath)
		return fmt.Errorf("rename temp to output: %w", err)
	}
	if err := os.Rename(sidecarTempPath, outPath+waifIdentitySidecarSuffix); err != nil {
		return fmt.Errorf("rename WAIF identity sidecar to output: %w", err)
	}
	if err := syncParentDirectory(outPath); err != nil {
		return fmt.Errorf("sync output directory: %w", err)
	}
	return nil
}
