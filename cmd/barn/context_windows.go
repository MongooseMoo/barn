//go:build windows

package main

import (
	"context"
	"os/signal"
	"syscall"
)

func interruptContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
