package profile

import (
	"barn/config"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildManifestIncludesChecksumsAndFeatures(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "Test.db")
	configPath := filepath.Join(dir, "outbound-off.conf")
	if err := os.WriteFile(dbPath, []byte("db one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("OUTBOUND_NETWORK = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := BuildManifest(BuildInput{
		ProfileID:         "barn-linux-testdb-outbound-off",
		ImplementationRef: "abc123 tracked_dirty=false",
		DatabasePath:      dbPath,
		ConfigPath:        configPath,
		Options:           config.Options{OutboundNetwork: false},
	})
	if err != nil {
		t.Fatal(err)
	}

	if manifest.ProfileID != "barn-linux-testdb-outbound-off" {
		t.Fatalf("profile_id = %q", manifest.ProfileID)
	}
	if manifest.DatabaseFixture != "testdb" {
		t.Fatalf("database_fixture = %q, want testdb", manifest.DatabaseFixture)
	}
	if manifest.DatabaseChecksum == "" || manifest.ConfigChecksum == "" {
		t.Fatalf("missing checksums: %+v", manifest)
	}
	if manifest.Features[config.FeatureOutboundNetwork] != false {
		t.Fatalf("OUTBOUND_NETWORK feature = %#v", manifest.Features[config.FeatureOutboundNetwork])
	}
	if manifest.Features["runtime.arch_bits"] == nil {
		t.Fatalf("missing runtime.arch_bits feature")
	}
}

func TestManifestChecksumChangesWhenConfigChanges(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "Test.db")
	configPath := filepath.Join(dir, "outbound.conf")
	if err := os.WriteFile(dbPath, []byte("db one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("OUTBOUND_NETWORK = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := BuildManifest(BuildInput{
		ProfileID:         "barn-linux-testdb-outbound-on",
		ImplementationRef: "abc123 tracked_dirty=false",
		DatabasePath:      dbPath,
		ConfigPath:        configPath,
		Options:           config.Options{OutboundNetwork: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("OUTBOUND_NETWORK = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := BuildManifest(BuildInput{
		ProfileID:         "barn-linux-testdb-outbound-off",
		ImplementationRef: "abc123 tracked_dirty=false",
		DatabasePath:      dbPath,
		ConfigPath:        configPath,
		Options:           config.Options{OutboundNetwork: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ConfigChecksum == second.ConfigChecksum {
		t.Fatalf("config checksum did not change: %s", first.ConfigChecksum)
	}
}

func TestWriteManifestWritesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "manifest.json")
	manifest := Manifest{
		ProfileID:         "barn-linux-testdb-outbound-on",
		Implementation:    "barn",
		ImplementationRef: "abc123 tracked_dirty=false",
		Features: map[string]any{
			config.FeatureOutboundNetwork: true,
		},
		SupportStatus: "supported",
	}
	if err := WriteManifest(path, manifest); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProfileID != manifest.ProfileID {
		t.Fatalf("decoded profile_id = %q", decoded.ProfileID)
	}
}

func TestBuildManifestRejectsMissingProfileID(t *testing.T) {
	_, err := BuildManifest(BuildInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}
