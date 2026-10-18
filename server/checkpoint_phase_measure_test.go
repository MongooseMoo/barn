package server

import (
	"log/slog"
	"os"
	"testing"
)

// Keep diagnostic logging out of the primary latency comparison. This separate
// opt-in run reports admission/lifecycle waits and capture/materialize/file phases.
func TestMeasureMongooseCheckpointPhases(t *testing.T) {
	if os.Getenv("BARN_CHECKPOINT_PHASES") != "1" {
		t.Skip("BARN_CHECKPOINT_PHASES unset")
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	TestMeasureMongooseCheckpoint(t)
}
