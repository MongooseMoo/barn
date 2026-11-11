//go:build elle

package isolationprobe

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestCheckpointStopRejectsInvalidSelections(t *testing.T) {
	for _, spec := range []string{
		"checkpoint/capture",
		"checkpoint/capture:0",
		"checkpoint/capture:-1",
		"checkpoint/capture:x",
		"checkpoint/unknown:1",
		"commit/validated:1",
	} {
		if _, err := checkpointStop(spec, &bytes.Buffer{}, func() error { return nil }); err == nil {
			t.Errorf("%q accepted", spec)
		}
	}
}

func TestCheckpointStopStopsOnlyAtSelectedOccurrence(t *testing.T) {
	var record bytes.Buffer
	stops := 0
	fn, err := checkpointStop(CheckpointDatabaseRenamed+":2", &record, func() error {
		stops++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Other points, including store points, never count toward the selection.
	fn("commit/validated", nil)
	fn(CheckpointJournalPublished, []string{"journal"})
	fn(CheckpointPairRenamed, nil)
	fn(CheckpointDatabaseRenamed, []string{"first"})
	if stops != 0 || record.Len() != 0 {
		t.Fatalf("stopped before the selected occurrence: stops=%d record=%q", stops, record.String())
	}
	fn(CheckpointDatabaseRenamed, []string{"out.db.new", "out.db.tmp"})
	if stops != 1 {
		t.Fatalf("stops = %d at the selected occurrence, want 1", stops)
	}
	var got struct {
		Msg        string   `json:"msg"`
		Point      string   `json:"point"`
		Occurrence uint64   `json:"occurrence"`
		PID        int      `json:"pid"`
		Artifacts  []string `json:"artifacts"`
	}
	line := record.String()
	if err := json.Unmarshal([]byte(line), &got); err != nil || line[len(line)-1] != '\n' {
		t.Fatalf("boundary record %q: %v", line, err)
	}
	if got.Msg != "checkpoint stop" || got.Point != CheckpointDatabaseRenamed || got.Occurrence != 2 ||
		got.PID != os.Getpid() || !slices.Equal(got.Artifacts, []string{"out.db.new", "out.db.tmp"}) {
		t.Fatalf("boundary record = %+v", got)
	}
	fn(CheckpointDatabaseRenamed, nil)
	if stops != 1 || record.String() != line {
		t.Fatal("stopped again after the selected occurrence")
	}
}

func TestInstallCheckpointStopLeavesFaultsInactive(t *testing.T) {
	if os.Getenv("BARN_ELLE_FAULT") != "" {
		t.Skip("run without BARN_ELLE_FAULT")
	}
	t.Setenv("BARN_ELLE_CHECKPOINT_STOP", CheckpointPairRenamed+":1")
	err := InstallCheckpointStop(&bytes.Buffer{})
	defer SetPause(nil)
	if !canStopSelf {
		if err == nil {
			t.Fatal("checkpoint stop accepted without SIGSTOP")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if pause.Load() == nil {
		t.Fatal("checkpoint stop not installed")
	}
	// Unselected points pass straight through without stopping this process.
	Pause("commit/validated")
	Pause(CheckpointDatabaseRenamed, "out")
	if ActiveFault() != "" || Fault("commit-exclusion") || Fault("finished-task-index") {
		t.Fatal("checkpoint stop activated a fault")
	}
}
