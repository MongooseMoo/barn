package format

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCheckpointWritesMainAndSibling(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "checkpoint.db")
	if err := WriteCheckpoint(path, loaded.NewStoreFromDatabase().Snapshot(), nil, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}

	mainBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read main checkpoint: %v", err)
	}
	siblingBytes, err := os.ReadFile(path + ".new")
	if err != nil {
		t.Fatalf("read sibling checkpoint: %v", err)
	}
	if !bytes.Equal(mainBytes, siblingBytes) {
		t.Fatalf("sibling checkpoint does not match main checkpoint")
	}

	if _, err := LoadDatabase(path); err != nil {
		t.Fatalf("load main checkpoint: %v", err)
	}
	if _, err := LoadDatabase(path + ".new"); err != nil {
		t.Fatalf("load sibling checkpoint: %v", err)
	}
}
