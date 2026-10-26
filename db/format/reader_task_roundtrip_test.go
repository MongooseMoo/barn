package format

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestTaskSourceTruncationReturnsEOF(t *testing.T) {
	raw, err := os.ReadFile("testdata/toastcore.db")
	if err != nil {
		t.Fatal(err)
	}
	database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(raw)), nil)
	if err != nil || database == nil || len(database.QueuedTasks) == 0 {
		t.Fatalf("original fixture: database=%v err=%v", database, err)
	}
	dot := bytes.Index(raw, []byte("\n.\n"))
	if dot < 0 {
		t.Fatal("fixture has no source terminator")
	}
	truncated := &taskSourceEOFReader{Reader: bytes.NewReader(raw[:dot+1])}
	_, err = parseDatabaseWithWaifIdentities(bufio.NewReader(truncated), nil)
	if !errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "read queued task source") {
		t.Fatalf("truncated queued source: %v, want contextual EOF", err)
	}
	t.Logf("tracked fixture truncated at %d bytes: %v", dot+1, err)

	// Use the real queued program as the VM writer's source, then truncate
	// its emitted frame at the program terminator, before environment metadata.
	var frame bytes.Buffer
	w := NewWriter(&frame, store.Snapshot{})
	var snapshot task.VMFrameSnapshot
	snapshot.Program.Source = database.QueuedTasks[0].Code
	if err := w.writeVMFrame(snapshot, types.ActivationFrame{}); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	dot = bytes.Index(frame.Bytes(), []byte("\n.\n"))
	if dot < 0 {
		t.Fatal("writer emitted no source terminator")
	}
	truncated = &taskSourceEOFReader{Reader: bytes.NewReader(frame.Bytes()[:dot+1])}
	_, _, err = database.readVMFrame(bufio.NewReader(truncated))
	if !errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "read program") {
		t.Fatalf("truncated VM source: %v, want contextual EOF", err)
	}
	t.Logf("writer frame truncated at %d bytes: %v", dot+1, err)
}

// Bound the regression probe even when run against the old EOF-swallowing
// helper: a second EOF becomes a distinct error rather than growing forever.
type taskSourceEOFReader struct {
	*bytes.Reader
	sawEOF bool
}

func (r *taskSourceEOFReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		if r.sawEOF {
			return 0, errors.New("source reader ignored EOF")
		}
		r.sawEOF = true
	}
	return n, err
}

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
		CallStack: []types.ActivationFrame{{
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
