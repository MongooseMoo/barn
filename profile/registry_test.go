package profile

import (
	"barn/config"
	"os"
	"path/filepath"
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
