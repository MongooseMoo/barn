package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

type Registry struct {
	Profiles []RegistryEntry `json:"profiles"`
}

type RegistryEntry struct {
	ProfileID         string         `json:"profile_id"`
	Implementation    string         `json:"implementation"`
	RuntimeOS         string         `json:"runtime_os"`
	DatabaseFixture   string         `json:"database_fixture"`
	ConfigFile        string         `json:"config_file"`
	SupportStatus     string         `json:"support_status"`
	UnsupportedReason string         `json:"unsupported_reason"`
	ExpectedFeatures  map[string]any `json:"expected_features"`
	CommandTemplate   string         `json:"command_template"`
}

func LoadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, err
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return Registry{}, err
	}
	if err := registry.Validate(filepath.Dir(path)); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func (r Registry) Validate(baseDir string) error {
	seen := map[string]struct{}{}
	for i, entry := range r.Profiles {
		if entry.ProfileID == "" {
			return fmt.Errorf("profiles[%d]: profile_id is required", i)
		}
		if _, ok := seen[entry.ProfileID]; ok {
			return fmt.Errorf("profiles[%d]: duplicate profile_id %q", i, entry.ProfileID)
		}
		seen[entry.ProfileID] = struct{}{}
		if entry.Implementation == "" {
			return fmt.Errorf("%s: implementation is required", entry.ProfileID)
		}
		if entry.RuntimeOS == "" {
			return fmt.Errorf("%s: runtime_os is required", entry.ProfileID)
		}
		if entry.DatabaseFixture == "" {
			return fmt.Errorf("%s: database_fixture is required", entry.ProfileID)
		}
		if entry.ConfigFile == "" {
			return fmt.Errorf("%s: config_file is required", entry.ProfileID)
		}
		if entry.SupportStatus == "" {
			return fmt.Errorf("%s: support_status is required", entry.ProfileID)
		}
		switch entry.SupportStatus {
		case "supported", "diagnostic", "unsupported":
		default:
			return fmt.Errorf("%s: invalid support_status %q", entry.ProfileID, entry.SupportStatus)
		}
		configPath := entry.ConfigFile
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseDir, configPath)
		}
		if info, err := os.Stat(configPath); err != nil || info.IsDir() {
			return fmt.Errorf("%s: config_file %q is not readable", entry.ProfileID, entry.ConfigFile)
		}
		if len(entry.ExpectedFeatures) == 0 {
			return fmt.Errorf("%s: expected_features is required", entry.ProfileID)
		}
		if entry.CommandTemplate == "" {
			return fmt.Errorf("%s: command_template is required", entry.ProfileID)
		}
	}
	return nil
}

func (r Registry) Find(profileID string) (RegistryEntry, bool) {
	for _, entry := range r.Profiles {
		if entry.ProfileID == profileID {
			return entry, true
		}
	}
	return RegistryEntry{}, false
}

func (r Registry) SortedProfiles() []RegistryEntry {
	profiles := append([]RegistryEntry(nil), r.Profiles...)
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].ProfileID < profiles[j].ProfileID
	})
	return profiles
}

func ValidateManifestAgainstProfile(entry RegistryEntry, manifest Manifest) error {
	if entry.SupportStatus == "unsupported" {
		if entry.UnsupportedReason == "" {
			return fmt.Errorf("%s: unsupported profile has no reason", entry.ProfileID)
		}
		return fmt.Errorf("%s: unsupported profile: %s", entry.ProfileID, entry.UnsupportedReason)
	}
	if manifest.ProfileID != entry.ProfileID {
		return fmt.Errorf("profile_id mismatch: manifest %q registry %q", manifest.ProfileID, entry.ProfileID)
	}
	if manifest.Implementation != entry.Implementation {
		return fmt.Errorf("%s: implementation mismatch: manifest %q registry %q", entry.ProfileID, manifest.Implementation, entry.Implementation)
	}
	if manifest.DatabaseFixture != entry.DatabaseFixture {
		return fmt.Errorf("%s: database_fixture mismatch: manifest %q registry %q", entry.ProfileID, manifest.DatabaseFixture, entry.DatabaseFixture)
	}
	for key, expected := range entry.ExpectedFeatures {
		actual, ok := manifest.Features[key]
		if !ok {
			return fmt.Errorf("%s: manifest missing feature %q", entry.ProfileID, key)
		}
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("%s: feature %q mismatch: manifest %#v registry %#v", entry.ProfileID, key, actual, expected)
		}
	}
	return nil
}
