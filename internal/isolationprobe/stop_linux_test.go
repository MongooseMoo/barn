//go:build elle && linux

package isolationprobe

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCheckpointStopHelper(t *testing.T) {
	if os.Getenv("BARN_CHECKPOINT_STOP_HELPER") != "1" {
		return
	}
	if err := InstallCheckpointStop(os.Stderr); err != nil {
		t.Fatal(err)
	}
	Pause(CheckpointJournalPublished, "out.db.new")
	os.Exit(3) // reached only after a SIGCONT
}

func TestCheckpointStopRecordsThenStopsOwnProcess(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestCheckpointStopHelper$")
	command.Env = append(os.Environ(), "BARN_CHECKPOINT_STOP_HELPER=1", "BARN_ELLE_CHECKPOINT_STOP="+CheckpointJournalPublished+":1")
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	line, err := bufio.NewReader(stderr).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Point string `json:"point"`
		PID   int    `json:"pid"`
	}
	if err := json.Unmarshal([]byte(line), &record); err != nil || record.Point != CheckpointJournalPublished || record.PID != command.Process.Pid {
		t.Fatalf("boundary record %q: %v", line, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(record.PID) + "/stat")
		if err != nil {
			t.Fatal(err)
		}
		// The state field follows the parenthesised command name.
		fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
		if fields[0] == "T" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("process state %s, want stopped", fields[0])
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := command.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 {
		t.Fatalf("resumed helper: %v", err)
	}
}
