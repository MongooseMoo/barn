package server

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barn/builtins"
	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/types"
)

func TestStartCompletesLoadedWaifRecycleShutdownBeforeService(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	class := addTestObject(t, store, 9, 0)
	store.AddVerb(class, dbstore.NewVerb(":recycle", []string{":recycle"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			`server_log("ORACLE WAIF RECYCLE ENTERED");`,
			"shutdown();",
			`server_log("ORACLE WAIF RECYCLE RETURNED");`,
		}))
	store.SetPendingFinalizations([]types.Value{types.NewWaif(class, 2)})

	dbPath := filepath.Join(t.TempDir(), "startup-finalization.db")
	file, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("create source database: %v", err)
	}
	if err := dbformat.NewWriter(file, store.Snapshot()).WriteDatabase(); err != nil {
		file.Close()
		t.Fatalf("write source database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close source database: %v", err)
	}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	s, err := NewServer(dbPath, []builtins.ListenerSpec{{Protocol: builtins.ListenerProtocolTCP, Port: 0}}, 1)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := s.LoadDatabase(); err != nil {
		t.Fatalf("LoadDatabase: %v", err)
	}
	t.Cleanup(s.scheduler.Stop)

	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	output := logs.String()
	entered := strings.Index(output, "ORACLE WAIF RECYCLE ENTERED")
	returned := strings.Index(output, "ORACLE WAIF RECYCLE RETURNED")
	if entered < 0 || returned < 0 || entered >= returned {
		t.Fatalf("startup recycle log order incorrect:\n%s", output)
	}
	if strings.Contains(output, "server started") {
		t.Fatalf("service reached server-started state after recycle requested shutdown:\n%s", output)
	}

	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running {
		t.Fatal("server still running after startup recycle shutdown")
	}
	reloaded, err := dbformat.LoadDatabase(dbPath + ".new")
	if err != nil {
		t.Fatalf("load final checkpoint: %v", err)
	}
	if got := len(reloaded.PendingFinalizations); got != 0 {
		t.Fatalf("final checkpoint pending roots = %v, want zero", reloaded.PendingFinalizations)
	}
}
