package format

import (
	"bufio"
	"bytes"
	"testing"
	"time"

	"barn/db/store"
	"barn/task"
	"barn/types"
)

func TestQueuedTaskWriterReaderPreservesToastHeaderFields(t *testing.T) {
	var dump bytes.Buffer
	writer := NewWriter(&dump, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{{
		ID:         517,
		Owner:      2,
		StartTime:  time.Unix(123, 0),
		Programmer: 3,
		VerbLoc:    6,
		VerbName:   "tick",
		This:       4,
		CallStack: []task.ActivationFrame{{
			This:       4,
			ThisValue:  types.NewObj(4),
			Player:     2,
			Programmer: 3,
			VerbLoc:    6,
			Verb:       "tick",
			StoredVerb: "tick",
		}},
		Fork: &task.ForkSnapshot{
			VariableNames: []string{"x"},
			FirstLine:     9,
			Variables: map[string]types.Value{
				"x": types.NewInt(1),
			},
			SourceLines: []string{"x = 1;"},
		},
	}}, nil)

	if err := writer.writeQueuedTasks(); err != nil {
		t.Fatalf("writeQueuedTasks: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	database := &Database{}
	if err := database.readQueuedTasks(bufio.NewReader(bytes.NewReader(dump.Bytes()))); err != nil {
		t.Fatalf("readQueuedTasks: %v", err)
	}
	if len(database.QueuedTasks) != 1 {
		t.Fatalf("queued task count = %d, want 1", len(database.QueuedTasks))
	}
	got := database.QueuedTasks[0]
	if got.StartTime != 123 {
		t.Errorf("start time = %d, want 123", got.StartTime)
	}
	if got.ID != 517 {
		t.Errorf("task id = %d, want 517", got.ID)
	}
}
