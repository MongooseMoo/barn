//go:build !windows

package main

import "context"

// POSIX signals are dispatched by the server loop after database loading.
func interruptContext() (context.Context, context.CancelFunc) {
	return context.Background(), func() {}
}
