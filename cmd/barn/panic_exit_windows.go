//go:build windows

package main

import "os"

// Windows has no POSIX signal status. The managed conformance boundary accepts
// the Microsoft CRT abort status as the portable equivalent.
func terminatePanicShutdown() {
	os.Exit(3)
}
