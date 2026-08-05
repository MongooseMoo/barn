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
	addTestVerb(store, 0, "server_started", `server_log("ORACLE SERVER STARTED");`)
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
	started := strings.Index(output, "ORACLE SERVER STARTED")
	if started < 0 || started >= entered {
		t.Fatalf("server_started did not precede startup finalization:\n%s", output)
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

func TestStartMatchesToastLifecycleAndPerTypeFinalizationOrder(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	addTestVerb(store, 0, "user_disconnected", `server_log("ORDER disconnected");`)
	addTestVerb(store, 0, "server_started", `server_log("ORDER server_started");`)

	addRecycle := func(id types.ObjID, marker string, shutdown bool) {
		addTestObject(t, store, id, 0)
		code := []string{`server_log("ORDER ` + marker + `");`}
		if shutdown {
			code = append(code, "shutdown();", `server_log("ORDER shutdown_returned");`)
		}
		store.AddVerb(id, dbstore.NewVerb(":recycle", []string{":recycle", "recycle"}, 2,
			dbstore.VerbRead|dbstore.VerbExecute,
			dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, code))
	}
	addRecycle(11, "waif_1", false)
	addRecycle(12, "waif_2", true)
	addRecycle(13, "anon_1", false)
	addRecycle(14, "anon_2", false)
	anon1, errCode := store.CreateObject([]types.ObjID{13}, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anon 1: %s", errCode)
	}
	anon2, errCode := store.CreateObject([]types.ObjID{14}, 2, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anon 2: %s", errCode)
	}
	store.SetPendingFinalizations([]types.Value{
		types.NewWaif(11, 2), types.NewAnon(anon1),
		types.NewWaif(12, 2), types.NewAnon(anon2),
	})

	dbPath := filepath.Join(t.TempDir(), "toast-startup-order.db")
	file, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("create source database: %v", err)
	}
	writer := dbformat.NewWriter(file, store.Snapshot())
	writer.SetActiveConnections([]dbformat.ActiveConnection{{Player: -7, Listener: 0}})
	if err := writer.WriteDatabase(); err != nil {
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
	markers := []string{
		"ORDER disconnected",
		"ORDER server_started",
		"ORDER anon_2",
		"ORDER anon_1",
		"ORDER waif_1",
		"ORDER waif_2",
		"ORDER shutdown_returned",
	}
	previous := -1
	for _, marker := range markers {
		index := strings.Index(output, marker)
		if index < 0 || index <= previous {
			t.Fatalf("startup marker %q missing or out of order:\n%s", marker, output)
		}
		previous = index
	}
	reloaded, err := dbformat.LoadDatabase(dbPath + ".new")
	if err != nil {
		t.Fatalf("load final checkpoint: %v", err)
	}
	if got := len(reloaded.PendingFinalizations); got != 0 {
		t.Fatalf("final checkpoint pending roots = %v, want zero", reloaded.PendingFinalizations)
	}
}
