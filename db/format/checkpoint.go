package format

import (
	"fmt"
	"os"

	"barn/db/store"
	"barn/task"
	"barn/types"
)

// WriteCheckpoint writes a database checkpoint to path+".new", never modifying path.
func WriteCheckpoint(
	path string,
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

	outPath := path + ".new"
	tempPath := path + ".tmp"
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

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tempPath, outPath); err != nil {
		return fmt.Errorf("rename temp to output: %w", err)
	}
	return nil
}
