package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunListsProfilesToConfiguredOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := DefaultConfig()
	cfg.ListProfiles = true
	cfg.ProfileRegistry = filepath.Join("..", "..", "profiles", "barn", "profiles.json")
	if err := Run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stdout.String(), "barn-linux-testdb-outbound-off") {
		t.Fatalf("profile output missing expected profile: %q", stdout.String())
	}
}

func TestRunStartupReturnsWrappedLoadError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "missing.db")
	cfg.DebugAddr = "off"
	err := Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "load database") {
		t.Fatalf("Run error = %v, want wrapped load database error", err)
	}
}

func TestRunInspectionReturnsErrorRatherThanExiting(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join("..", "..", "Test.db")
	cfg.ObjectInfo = "not-an-object"
	err := Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run inspection succeeded, want parse error")
	}
}
