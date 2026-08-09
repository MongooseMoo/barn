//go:build !windows

package main

import (
	"os"
	"syscall"
)

func terminatePanicShutdown() {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
	// The signal normally cannot return. Preserve shell-level abort semantics if
	// a platform unexpectedly reports success without terminating the process.
	os.Exit(128 + int(syscall.SIGABRT))
}
