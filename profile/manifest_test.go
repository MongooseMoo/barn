package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/vm"
	"os"
	"path/filepath"
	"testing"
)

func TestChecksumFile(t *testing.T) {
	data := bytes.Repeat([]byte("manifest checksum\x00\xff"), 8192)
	sum := sha256.Sum256(data)
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", []byte("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"multiple chunks", data, hex.EncodeToString(sum[:])},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.db")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := checksumFile(path)
			if err != nil || got != test.want {
				t.Fatalf("checksumFile = %q, %v, want %q", got, err, test.want)
			}
		})
	}
	if got, err := checksumFile(filepath.Join(t.TempDir(), "missing.db")); got != "" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing checksum = %q, %v, want native not-exist error", got, err)
	}
	if got, err := checksumFile(t.TempDir()); got != "" || err == nil {
		t.Fatalf("directory checksum = %q, %v, want native read error", got, err)
	} else {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("directory error lost native path error: %v", err)
		}
	}
}

func BenchmarkChecksumFile(b *testing.B) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"config", []byte("OUTBOUND_NETWORK = 1\n")},
		{"database", bytes.Repeat([]byte{0xa5}, 32<<20)},
	} {
		b.Run(test.name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "input")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				b.Fatal(err)
			}
			sum := sha256.Sum256(test.data)
			want := hex.EncodeToString(sum[:])
			b.SetBytes(int64(len(test.data)))
			b.ReportAllocs()
			for b.Loop() {
				got, err := checksumFile(path)
				if err != nil || got != want {
					b.Fatalf("checksumFile = %q, %v, want %q", got, err, want)
				}
			}
		})
	}
}

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
		Registry:          vm.BuildVMRegistry(),
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
	if manifest.Features["feature.ast"] != true {
		t.Fatalf("feature.ast = %#v, want true with the AST builtins installed", manifest.Features["feature.ast"])
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
		Registry:          vm.BuildVMRegistry(),
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
			databaseSHA256: "33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925",
			implementation: "72e3c7f96ce7a41fdeba793aef8818dc4408072e",
			configSHA256:   "6c855f6b1f2dd584ba949d42891018ca68eccd34bd75b7e2300428b9246724a9",
			promoteNumbers: true,
		},
		{
			file:           "mongoose-0ff6fd7-wsl-testdb.json",
			profileID:      "toast-mongoose-0ff6fd7-wsl-testdb",
			fixture:        "testdb",
			databaseSHA256: "1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e",
			implementation: "0ff6fd77659bd0ad001176dffdd007f854afa96b",
			configSHA256:   "d9739c2df81aaae6d1749d2515e2cb0feb972439af25e473b2da391282052d6c",
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
		Registry:          vm.BuildVMRegistry(),
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
		Registry:          vm.BuildVMRegistry(),
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
