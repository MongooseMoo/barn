package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type manifestAtListenHandler struct {
	slog.Handler
	path        string
	seen, valid bool
}

func (h *manifestAtListenHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "listening" {
		h.seen = true
		content, err := os.ReadFile(h.path)
		h.valid = err == nil && json.Valid(content)
	}
	return h.Handler.Handle(ctx, r)
}

func TestRunRejectsMissingStartupScriptBeforeProfileAndServer(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join(dir, "invalid.db")
	if err := os.WriteFile(cfg.DatabasePath, []byte("invalid database\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.ConfigPath = filepath.Join("..", "..", "profiles", "barn", "outbound-on.conf")
	cfg.ProfileID = "barn-linux-testdb-outbound-on"
	cfg.ProfileManifest = filepath.Join(dir, "profile.json")
	cfg.Listen = []string{"tcp://127.0.0.1:0"}
	cfg.DebugAddr, cfg.OperatorAddr = "off", "off"
	script := filepath.Join(dir, "missing.moo")
	cfg.StartScript = &script
	err := Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "read startup script") {
		t.Errorf("Run = %v, want native missing-script error before server/database work", err)
	}
	if _, err := os.Stat(cfg.ProfileManifest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing script created profile manifest: %v", err)
	}
}

func TestRunPublishesManifestBeforeGameSocket(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join(dir, "invalid.db")
	if err := os.WriteFile(cfg.DatabasePath, []byte("invalid database\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.ConfigPath = filepath.Join("..", "..", "profiles", "barn", "outbound-on.conf")
	cfg.ProfileID = "barn-linux-testdb-outbound-on"
	cfg.ProfileManifest = filepath.Join(dir, "profile.json")
	cfg.Listen = []string{"tcp://127.0.0.1:0"}
	cfg.DebugAddr, cfg.OperatorAddr = "off", "off"
	var logs bytes.Buffer
	handler := &manifestAtListenHandler{Handler: slog.NewTextHandler(&logs, nil), path: cfg.ProfileManifest}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(previous)
	err := Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "load database") {
		t.Fatalf("Run = %v, want database load failure", err)
	}
	if !handler.seen || !handler.valid {
		t.Fatalf("game socket exposed before complete manifest: listening=%v manifest_valid=%v", handler.seen, handler.valid)
	}
}

// A first checkpoint publication interrupted after its journal was published
// leaves no database at the output path, only the journal and the saved next
// pair. The manifest must name the database recovery restores.
func TestRunManifestNamesRecoveredFirstPublication(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join(dir, "input.db.new")
	generation := filepath.Join(dir, "input.db.new.recovery-1")
	next := filepath.Join(generation, "next")
	if err := os.MkdirAll(generation, 0700); err != nil {
		t.Fatal(err)
	}
	database := []byte("invalid database\n")
	databaseDigest := sha256.Sum256(database)
	sidecar := []byte(fmt.Sprintf("barn-waif-identities-v1 %x\n", databaseDigest))
	if err := os.WriteFile(next, database, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next+".waifids", sidecar, 0600); err != nil {
		t.Fatal(err)
	}
	journal, err := json.Marshal(map[string]any{
		"Version": 1, "Generation": filepath.Base(generation),
		"Next": map[string]any{"Database": databaseDigest, "Sidecar": sha256.Sum256(sidecar), "HasSidecar": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.DatabasePath+".publication", journal, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.ConfigPath = filepath.Join("..", "..", "profiles", "barn", "outbound-on.conf")
	cfg.ProfileID = "barn-linux-testdb-outbound-on"
	cfg.ProfileManifest = filepath.Join(dir, "profile.json")
	cfg.Listen = []string{"tcp://127.0.0.1:0"}
	cfg.DebugAddr, cfg.OperatorAddr = "off", "off"
	err = Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "load database") {
		t.Fatalf("Run = %v, want the restored invalid database to fail loading", err)
	}
	content, err := os.ReadFile(cfg.ProfileManifest)
	if err != nil {
		t.Fatalf("no manifest for a recovered first publication: %v", err)
	}
	var manifest struct {
		DatabaseChecksum string `json:"database_checksum"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%x", databaseDigest); manifest.DatabaseChecksum != want {
		t.Fatalf("manifest database_checksum = %q, want the restored next database %q", manifest.DatabaseChecksum, want)
	}
	if _, err := os.Lstat(cfg.DatabasePath + ".publication"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("publication journal remains after recovery: %v", err)
	}
}

func TestRunReservesGamePortBeforeHTTPEndpoints(t *testing.T) {
	for _, endpoint := range []string{"debug", "operator"} {
		t.Run(endpoint, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := ln.Addr().String()
			if err := ln.Close(); err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			cfg := DefaultConfig()
			cfg.DatabasePath = filepath.Join(t.TempDir(), "missing.db")
			cfg.Listen = []string{"tcp://" + addr}
			cfg.DebugAddr, cfg.OperatorAddr = "off", "off"
			if endpoint == "debug" {
				cfg.DebugAddr = addr
			} else {
				cfg.OperatorAddr = addr
			}
			// Force the exact allocation collision rather than waiting for the
			// kernel's ephemeral HTTP port choice to hit the released game port.
			err = Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "load database") {
				t.Fatalf("Run = %v, want database load failure", err)
			}
			if !strings.Contains(logs.String(), endpoint+" endpoint unavailable") {
				t.Fatalf("HTTP endpoint took the game port before database loading: %s", &logs)
			}
			// A failure before Start must release the reserved socket, too.
			ln, err = net.Listen("tcp", addr)
			if err != nil {
				t.Fatalf("game reservation leaked after load failure: %v", err)
			}
			ln.Close()
		})
	}
}
