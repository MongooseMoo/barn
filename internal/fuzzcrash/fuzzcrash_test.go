package fuzzcrash_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/MongooseMoo/barn/internal/fuzzcrash"
)

// A panic on a goroutine other than the test's exits the process with
// status 2, as it does a fuzz worker; its text must reach the crash file.
func TestPanicOffTestGoroutineReachesCrashFile(t *testing.T) {
	if os.Getenv("FUZZCRASH_HELPER") == "1" {
		go panic("fuzzcrash helper panic")
		select {}
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPanicOffTestGoroutineReachesCrashFile$")
	cmd.Env = append(os.Environ(), "FUZZCRASH_HELPER=1", "BARN_FUZZ_CRASH_DIR="+dir)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("helper ended with %v, want exit status 2", err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "crash-*.txt"))
	if err != nil || len(files) != 1 {
		t.Fatalf("crash files %v (%v), want one", files, err)
	}
	text, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "panic: fuzzcrash helper panic") || !strings.Contains(string(text), "goroutine ") {
		t.Fatalf("crash file lacks the panic and its stack:\n%s", text)
	}
}
