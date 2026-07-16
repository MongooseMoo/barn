package profile

import (
	"barn/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRegistryLoadsCommittedProfiles(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join("..", "profiles", "barn", "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	required := []string{
		"barn-linux-testdb-outbound-on",
		"barn-linux-testdb-outbound-off",
		"barn-linux-mongoose-outbound-on",
		"barn-linux-mongoose-outbound-off",
		"barn-windows-testdb-outbound-on",
		"barn-windows-testdb-outbound-off",
		"barn-windows-mongoose-outbound-on",
		"barn-windows-mongoose-outbound-off",
	}
	for _, id := range required {
		if _, ok := registry.Find(id); !ok {
			t.Fatalf("missing profile %s", id)
		}
	}
}

func TestCommittedMongooseProfilesEnablePromotion(t *testing.T) {
	registryPath := filepath.Join("..", "profiles", "barn", "profiles.json")
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}

	databasePath := filepath.Join(t.TempDir(), "mongoose.db")
	if err := os.WriteFile(databasePath, []byte("mongoose fixture"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		profileID  string
		configFile string
		outbound   bool
	}{
		{"barn-linux-mongoose-outbound-on", "mongoose-outbound-on.conf", true},
		{"barn-linux-mongoose-outbound-off", "mongoose-outbound-off.conf", false},
		{"barn-windows-mongoose-outbound-on", "mongoose-outbound-on.conf", true},
		{"barn-windows-mongoose-outbound-off", "mongoose-outbound-off.conf", false},
	}

	for _, test := range tests {
		t.Run(test.profileID, func(t *testing.T) {
			entry, ok := registry.Find(test.profileID)
			if !ok {
				t.Fatalf("missing profile %s", test.profileID)
			}
			if entry.ConfigFile != test.configFile {
				t.Fatalf("config_file = %q, want %q", entry.ConfigFile, test.configFile)
			}
			if !strings.Contains(entry.CommandTemplate, strings.TrimSuffix(test.configFile, ".conf")) {
				t.Fatalf("command template does not use %q: %s", test.configFile, entry.CommandTemplate)
			}

			configPath := filepath.Join(filepath.Dir(registryPath), entry.ConfigFile)
			options, err := config.LoadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if options.OutboundNetwork != test.outbound {
				t.Fatalf("OutboundNetwork = %v, want %v", options.OutboundNetwork, test.outbound)
			}
			if !options.PromoteNumbers {
				t.Fatal("PromoteNumbers = false, want true")
			}
			if entry.ExpectedFeatures[config.FeaturePromoteNumbers] != true {
				t.Fatalf("expected promotion feature = %#v", entry.ExpectedFeatures[config.FeaturePromoteNumbers])
			}

			manifest, err := BuildManifest(BuildInput{
				ProfileID:         entry.ProfileID,
				ImplementationRef: "test",
				DatabasePath:      databasePath,
				ConfigPath:        configPath,
				Options:           options,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateManifestAgainstProfile(entry, manifest); err != nil {
				t.Fatal(err)
			}

			delete(manifest.Features, config.FeaturePromoteNumbers)
			if err := ValidateManifestAgainstProfile(entry, manifest); err == nil {
				t.Fatal("expected missing promotion feature rejection")
			}
			manifest.Features[config.FeaturePromoteNumbers] = false
			if err := ValidateManifestAgainstProfile(entry, manifest); err == nil {
				t.Fatal("expected false promotion feature rejection")
			}
		})
	}
}

func TestRegistryRejectsMissingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	data := []byte(`{"profiles":[{
		"profile_id":"barn-linux-testdb-outbound-on",
		"implementation":"barn",
		"runtime_os":"linux",
		"database_fixture":"testdb",
		"config_file":"missing.conf",
		"support_status":"supported",
		"expected_features":{"option.OUTBOUND_NETWORK":true},
		"command_template":"barn"
	}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestValidateManifestAgainstProfileRejectsFeatureMismatch(t *testing.T) {
	entry := RegistryEntry{
		ProfileID:       "barn-linux-testdb-outbound-on",
		Implementation:  "barn",
		RuntimeOS:       "linux",
		DatabaseFixture: "testdb",
		SupportStatus:   "supported",
		ExpectedFeatures: map[string]any{
			config.FeatureOutboundNetwork: true,
		},
	}
	manifest := Manifest{
		ProfileID:       "barn-linux-testdb-outbound-on",
		Implementation:  "barn",
		DatabaseFixture: "testdb",
		Features: map[string]any{
			config.FeatureOutboundNetwork: false,
		},
	}
	if err := ValidateManifestAgainstProfile(entry, manifest); err == nil {
		t.Fatal("expected feature mismatch")
	}
}

func TestValidateManifestAgainstProfileAcceptsExpectedFeatures(t *testing.T) {
	entry := RegistryEntry{
		ProfileID:       "barn-linux-testdb-outbound-off",
		Implementation:  "barn",
		RuntimeOS:       "linux",
		DatabaseFixture: "testdb",
		SupportStatus:   "supported",
		ExpectedFeatures: map[string]any{
			config.FeatureOutboundNetwork: false,
		},
	}
	manifest := Manifest{
		ProfileID:       "barn-linux-testdb-outbound-off",
		Implementation:  "barn",
		DatabaseFixture: "testdb",
		Features: map[string]any{
			config.FeatureOutboundNetwork: false,
		},
	}
	if err := ValidateManifestAgainstProfile(entry, manifest); err != nil {
		t.Fatal(err)
	}
}
