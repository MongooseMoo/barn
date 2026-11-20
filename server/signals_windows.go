//go:build windows

package server

import "os"

// Windows cancellation is supplied by the command's interrupt context.
func listenSignals() (<-chan os.Signal, func()) { return nil, func() {} }

func (s *Server) handleOSSignal(os.Signal) error { return nil }
