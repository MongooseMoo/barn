//go:build !windows

package main

import (
	"os"
	"syscall"
)

func terminatePanicShutdown() {
	// The Go runtime owns SIGABRT and translates it to exit status 2. Replace the
	// process with a POSIX shell whose default SIGABRT disposition preserves the
	// signal status expected by supervisors and the conformance harness.
	_ = syscall.Exec("/bin/sh", []string{"sh", "-c", "kill -ABRT $$"}, os.Environ())
	// Exec normally cannot return. Preserve shell-level abort semantics if the
	// platform lacks the standard POSIX shell or rejects the replacement.
	os.Exit(128 + int(syscall.SIGABRT))
}
