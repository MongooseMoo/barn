// Package fuzzcrash keeps the crash output of Go fuzz workers.
//
// The fuzz coordinator starts each worker with stdout and stderr discarded
// (internal/fuzz/worker.go in the Go tree), so a worker that dies of a runtime
// fatal error or of a panic on another goroutine leaves only "fuzzing process
// hung or terminated unexpectedly: exit status 2". A fuzzed package's tests
// import this package for its side effect: when BARN_FUZZ_CRASH_DIR is set,
// every test process, workers included, also writes its fatal error output to
// crash-<pid>.txt in that directory.
package fuzzcrash

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
)

func init() {
	dir := os.Getenv("BARN_FUZZ_CRASH_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("crash-%d.txt", os.Getpid())))
	if err != nil {
		panic(fmt.Sprintf("fuzzcrash: %v", err))
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		panic(fmt.Sprintf("fuzzcrash: %v", err))
	}
	// SetCrashOutput duplicates the descriptor, so this one is not needed.
	f.Close()
}
