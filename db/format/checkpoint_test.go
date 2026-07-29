package format

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCheckpointWritesOnlyToNewFile(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "checkpoint.db")
	if err := WriteCheckpoint(path, loaded.NewStoreFromDatabase().Snapshot(), nil, nil, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}

	// Input path must not be created or modified.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("input path %s should not exist after checkpoint", path)
	}

	// Output path+".new" must be written and loadable.
	if _, err := LoadDatabase(path + ".new"); err != nil {
		t.Fatalf("load output checkpoint: %v", err)
	}
}
