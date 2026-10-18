package server

import (
	"log/slog"
	"os"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
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
	// Inventory is outside the timed checkpoint trials. This diagnostic run is
	// excluded from primary latency/RSS comparisons.
	database, err := dbformat.LoadDatabaseReadOnly(os.Getenv("BARN_CHECKPOINT_FIXTURE"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture objects=%d anonymous=%d queued=%d suspended=%d", len(database.Objects), len(database.AnonymousObjs), len(database.QueuedTasks), len(database.SuspendedTasks))
	TestMeasureMongooseCheckpoint(t)
}
