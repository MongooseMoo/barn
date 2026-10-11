//go:build !windows

package server

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func listenSignals() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 16)
	sigs := []os.Signal{
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2,
		syscall.SIGILL, syscall.SIGQUIT, syscall.SIGSEGV, syscall.SIGBUS,
		syscall.SIGFPE, syscall.SIGPIPE, syscall.SIGALRM,
	}
	// Preserve an ignored HUP inherited from the launching process.
	if !signal.Ignored(syscall.SIGHUP) {
		sigs = append(sigs, syscall.SIGHUP)
	}
	// Notify delivers externally sent fault signals here. Synchronous faults
	// still belong to the Go runtime; exec.Cmd owns child waiting and reaping.
	signal.Notify(ch, sigs...)
	return ch, func() { signal.Stop(ch) }
}

func (s *Server) handleOSSignal(sig os.Signal) error {
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		s.mu.Lock()
		s.signalShutdown = true
		s.mu.Unlock()
		s.Shutdown("shutdown signal received")
	case syscall.SIGUSR1:
		return s.handleUserSignal("SIGUSR1")
	case syscall.SIGUSR2:
		return s.handleUserSignal("SIGUSR2")
	case syscall.SIGALRM:
		return s.requestCheckpoint()
	case syscall.SIGHUP, syscall.SIGILL, syscall.SIGQUIT, syscall.SIGSEGV, syscall.SIGBUS:
		s.connManager.CloseListeners()
		s.connManager.CloseConnections("server panic")
		slog.Info("PANIC-DUMPING")
		return s.Panic(fmt.Sprintf("Caught signal %d", sig))
	}
	return nil
}
