//go:build elle && linux

package isolationprobe

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func init() {
	// In the helper process, pin the main goroutine (parked while tests run)
	// to the thread group leader, so the stopping code runs on another thread,
	// as Barn's checkpoint goroutine does. A process-directed SIGSTOP is then
	// delivered to the sleeping leader first.
	if os.Getenv("BARN_CHECKPOINT_STOP_MARKER") != "" {
		runtime.LockOSThread()
	}
}

// TestCheckpointStopNoCodeRunsAfterStopHelper does work right after the stop
// point returns, the way a checkpoint goes on to its next rename.
func TestCheckpointStopNoCodeRunsAfterStopHelper(t *testing.T) {
	marker := os.Getenv("BARN_CHECKPOINT_STOP_MARKER")
	if marker == "" {
		return
	}
	if err := InstallCheckpointStop(os.Stderr); err != nil {
		t.Fatal(err)
	}
	Pause(CheckpointJournalPublished, "out.db.new")
	if err := os.WriteFile(marker, []byte("ran"), 0600); err != nil {
		os.Exit(4)
	}
	os.Exit(3)
}

// The stop must take effect before the stopping code returns: anything the
// checkpoint does after its stop point would change the files the owner
// classifies while the process is stopped (CI run 42964137644 froze every
// rename stop at both renames done).
func TestCheckpointStopNoCodeRunsAfterStop(t *testing.T) {
	for attempt := 0; attempt < 25; attempt++ {
		marker := filepath.Join(t.TempDir(), "after-stop")
		command := exec.Command(os.Args[0], "-test.run=^TestCheckpointStopNoCodeRunsAfterStopHelper$")
		command.Env = append(os.Environ(), "BARN_CHECKPOINT_STOP_MARKER="+marker,
			"BARN_ELLE_CHECKPOINT_STOP="+CheckpointJournalPublished+":1")
		stderr, err := command.StderrPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		if _, err := bufio.NewReader(stderr).ReadString('\n'); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatal(err)
		}
		waitStopped(t, command.Process.Pid)
		time.Sleep(20 * time.Millisecond)
		_, statErr := os.Stat(marker)
		if err := command.Process.Signal(syscall.SIGCONT); err != nil {
			t.Fatal(err)
		}
		err = command.Wait()
		if statErr == nil {
			t.Fatalf("attempt %d: code after the stop point ran before SIGCONT", attempt)
		}
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 {
			t.Fatalf("attempt %d: resumed helper: %v", attempt, err)
		}
	}
}

func waitStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
		if fields[0] == "T" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process state %s, want stopped", fields[0])
		}
		time.Sleep(time.Millisecond)
	}
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
