package server

import (
	"errors"
	"log/slog"

	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

// handleUserSignal honors the hook's initial outcome. A hook that suspends or
// aborts suppresses the default even if a later resumption returns false.
func (s *Server) handleUserSignal(name string) error {
	result, err := s.runtime.RunServerVerbTask(0, "handle_signal", []types.Value{types.NewStr(name)}, -1)
	if err != nil && !errors.Is(err, engine.ErrServerVerbNotFound) {
		slog.Warn("#0:handle_signal() failed", slog.Any("err", err))
		return nil
	}
	if err == nil && ((result.Flow != types.FlowNormal && result.Flow != types.FlowReturn) || result.Val.Truthy()) {
		return nil
	}
	if name == "SIGUSR2" {
		slog.Info("CHECKPOINTING due to remote request signal.")
		return s.requestCheckpoint()
	}
	// Without a named log file, the USR1 default has no effect.
	return nil
}
