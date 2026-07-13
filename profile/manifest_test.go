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
	if manifest.Features[config.FeaturePromoteNumbers] != false {
		t.Fatalf("PROMOTE_NUMBERS feature = %#v", manifest.Features[config.FeaturePromoteNumbers])
	}
	if manifest.Features["runtime.arch_bits"] == nil {
		t.Fatalf("missing runtime.arch_bits feature")
	}
}

func TestBuildManifestIncludesEnabledPromotion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mongoose.db")
	configPath := filepath.Join(dir, "mongoose-outbound-on.conf")
	if err := os.WriteFile(dbPath, []byte("mongoose fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("OUTBOUND_NETWORK = 1\nPROMOTE_NUMBERS = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := BuildManifest(BuildInput{
		ProfileID:         "barn-linux-mongoose-outbound-on",
		ImplementationRef: "abc123 tracked_dirty=false",
		DatabasePath:      dbPath,
		ConfigPath:        configPath,
		Options: config.Options{
			OutboundNetwork: true,
			PromoteNumbers:  true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if manifest.DatabaseFixture != "mongoose" {
		t.Fatalf("database_fixture = %q, want mongoose", manifest.DatabaseFixture)
	}
	if manifest.DatabaseChecksum == "" || manifest.ConfigChecksum == "" {
		t.Fatalf("missing checksums: %+v", manifest)
	}
	if manifest.Features[config.FeaturePromoteNumbers] != true {
		t.Fatalf("PROMOTE_NUMBERS feature = %#v", manifest.Features[config.FeaturePromoteNumbers])
	}
}

func TestCommittedToastOracleManifests(t *testing.T) {
	tests := []struct {
		file           string
		profileID      string
		fixture        string
		databaseSHA256 string
		implementation string
		configSHA256   string
		promoteNumbers bool
	}{
		{
			file:           "stock-wsl-testdb.json",
			profileID:      "toast-stock-wsl-testdb",
			fixture:        "testdb",
			databaseSHA256: "1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e",
			implementation: "aecc51e9449c6e7c95272f0f044b5ba38948459e",
			configSHA256:   "a88a8c6c37b66ca65a08a318988361827f131421edeff25e5b4af83fb3fa8036",
			promoteNumbers: false,
		},
		{
			file:           "mongoose-wsl-mongoose.json",
			profileID:      "toast-mongoose-wsl-mongoose",
			fixture:        "mongoose",
			databaseSHA256: "a9d167861eab56d62e9bd12ae1d47c5e6a858530020a5dcf174a0b104fb23db9",
			implementation: "72e3c7f96ce7a41fdeba793aef8818dc4408072e",
			configSHA256:   "6c855f6b1f2dd584ba949d42891018ca68eccd34bd75b7e2300428b9246724a9",
			promoteNumbers: true,
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			path := filepath.Join("..", "profiles", "toast", test.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}

			if manifest.ProfileID != test.profileID {
				t.Fatalf("profile_id = %q, want %q", manifest.ProfileID, test.profileID)
			}
			if manifest.Implementation != "toaststunt" {
				t.Fatalf("implementation = %q, want toaststunt", manifest.Implementation)
			}
			if manifest.ImplementationRef != test.implementation {
				t.Fatalf("implementation_ref = %q, want %q", manifest.ImplementationRef, test.implementation)
			}
			if manifest.RuntimeOS != "linux" || manifest.ArchBits != 64 {
				t.Fatalf("runtime identity = %s/%d, want linux/64", manifest.RuntimeOS, manifest.ArchBits)
			}
			if manifest.DatabaseFixture != test.fixture || manifest.DatabaseChecksum != test.databaseSHA256 {
				t.Fatalf("database identity = %s/%s", manifest.DatabaseFixture, manifest.DatabaseChecksum)
			}
			if manifest.ConfigChecksum != test.configSHA256 {
				t.Fatalf("config_checksum = %q, want %q", manifest.ConfigChecksum, test.configSHA256)
			}
			if manifest.Features[config.FeatureOutboundNetwork] != true {
				t.Fatalf("OUTBOUND_NETWORK feature = %#v", manifest.Features[config.FeatureOutboundNetwork])
			}
			if manifest.Features[config.FeaturePromoteNumbers] != test.promoteNumbers {
				t.Fatalf("PROMOTE_NUMBERS feature = %#v, want %v", manifest.Features[config.FeaturePromoteNumbers], test.promoteNumbers)
			}
			if manifest.SupportStatus != "supported" {
				t.Fatalf("support_status = %q, want supported", manifest.SupportStatus)
			}
		})
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
