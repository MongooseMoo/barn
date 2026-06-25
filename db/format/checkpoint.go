package format

import (
	"fmt"
	"io"
	"os"

	"barn/db/store"
	"barn/task"
)

// WriteCheckpoint writes a database checkpoint to path and path+".new".
func WriteCheckpoint(path string, snapshot store.Snapshot, queuedTasks, suspendedTasks []task.Snapshot) error {
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

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(path)
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("rename temp to main: %w", err)
		}
	}

	if err := copyFile(path, path+".new"); err != nil {
		return fmt.Errorf("write sibling checkpoint: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
