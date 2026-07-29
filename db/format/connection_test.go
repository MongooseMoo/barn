package format

import (
	"bufio"
	"bytes"
	"testing"

	"barn/db/store"
	"barn/types"
)

func TestActiveConnectionsWriterReaderPreservesPlayerListenerPairs(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetActiveConnections([]ActiveConnection{
		{Player: -7, Listener: 0},
		{Player: 3, Listener: 4},
	})

	if err := writer.writeActiveConnections(); err != nil {
		t.Fatalf("writeActiveConnections: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got, want := buf.String(), "2 active connections with listeners\n-7 0\n3 4\n"; got != want {
		t.Fatalf("active connection section = %q, want %q", got, want)
	}

	database := &Database{}
	if err := database.readActiveConnections(bufio.NewReader(bytes.NewReader(buf.Bytes()))); err != nil {
		t.Fatalf("readActiveConnections: %v", err)
	}
	want := []ActiveConnection{
		{Player: types.ObjID(-7), Listener: 0},
		{Player: 3, Listener: 4},
	}
	if len(database.ActiveConnections) != len(want) {
		t.Fatalf("active connection count = %d, want %d", len(database.ActiveConnections), len(want))
	}
	for i := range want {
		if database.ActiveConnections[i] != want[i] {
			t.Fatalf("active connection %d = %#v, want %#v", i, database.ActiveConnections[i], want[i])
		}
	}
}
