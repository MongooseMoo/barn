package format

import (
	"fmt"
	"os"

	"barn/db/store"
	"barn/task"
)

// WriteCheckpoint writes a database checkpoint to path+".new", never modifying path.
func WriteCheckpoint(path string, snapshot store.Snapshot, queuedTasks, suspendedTasks []task.Snapshot) error {
	outPath := path + ".new"
	tempPath := path + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	writer := NewWriter(tempFile, snapshot)
	writer.SetTaskSnapshots(queuedTasks, suspendedTasks)
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
