//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestTerminatePanicShutdownUsesPortableAbortContract(t *testing.T) {
	if os.Getenv("BARN_TEST_PANIC_ABORT") == "1" {
		terminatePanicShutdown()
		t.Fatal("terminatePanicShutdown returned")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestTerminatePanicShutdownUsesPortableAbortContract$")
	command.Env = append(os.Environ(), "BARN_TEST_PANIC_ABORT=1")
	err := command.Run()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("panic child error = %v, want process termination", err)
	}
	if code := exitError.ExitCode(); code != 3 {
		t.Fatalf("Windows panic status = %d, want CRT abort status 3", code)
	}
}
