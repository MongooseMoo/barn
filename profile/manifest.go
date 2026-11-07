package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Manifest struct {
	ProfileID         string         `json:"profile_id"`
	Implementation    string         `json:"implementation"`
	ImplementationRef string         `json:"implementation_ref"`
	BinaryPath        string         `json:"binary_path"`
	BuildSystem       string         `json:"build_system"`
	RuntimeOS         string         `json:"runtime_os"`
	ArchBits          int            `json:"arch_bits"`
	DatabaseFixture   string         `json:"database_fixture"`
	DatabaseChecksum  string         `json:"database_checksum"`
	ConfigFile        string         `json:"config_file"`
	ConfigChecksum    string         `json:"config_checksum"`
	Features          map[string]any `json:"features"`
	SupportStatus     string         `json:"support_status"`
	UnsupportedReason string         `json:"unsupported_reason"`
}

type BuildInput struct {
	ProfileID         string
	ImplementationRef string
	DatabasePath      string
	ConfigPath        string
	Options           config.Options
	Registry          *builtins.Registry
}

func BuildManifest(input BuildInput) (Manifest, error) {
	if input.ProfileID == "" {
		return Manifest{}, fmt.Errorf("profile_id is required")
	}
	if input.Registry == nil {
		return Manifest{}, fmt.Errorf("constructed builtin registry is required")
	}
	if input.ConfigPath == "" {
		return Manifest{}, fmt.Errorf("config_file is required")
	}
	if input.DatabasePath == "" {
		return Manifest{}, fmt.Errorf("database path is required")
	}
	if err := input.Options.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("invalid options: %w", err)
	}

	dbChecksum, err := checksumFile(input.DatabasePath)
	if err != nil {
		return Manifest{}, fmt.Errorf("database checksum: %w", err)
	}
	configChecksum, err := checksumFile(input.ConfigPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("config checksum: %w", err)
	}

	binaryPath, err := os.Executable()
	if err == nil {
		binaryPath, _ = filepath.Abs(binaryPath)
	}
	configPath, _ := filepath.Abs(input.ConfigPath)

	features := input.Options.FeatureMap()
	for name, value := range input.Registry.Presence() {
		features[name] = value
	}
	features["runtime.arch_bits"] = strconv.IntSize
	features["platform.path_separator"] = string(os.PathSeparator)
	features["platform.backslash_is_path_separator"] = os.PathSeparator == '\\'

	return Manifest{
		ProfileID:         input.ProfileID,
		Implementation:    "barn",
		ImplementationRef: input.ImplementationRef,
		BinaryPath:        binaryPath,
		BuildSystem:       "go",
		RuntimeOS:         runtime.GOOS,
		ArchBits:          strconv.IntSize,
		DatabaseFixture:   fixtureID(input.ProfileID, input.DatabasePath),
		DatabaseChecksum:  dbChecksum,
		ConfigFile:        configPath,
		ConfigChecksum:    configChecksum,
		Features:          features,
		SupportStatus:     "supported",
	}, nil
}

func WriteManifest(path string, manifest Manifest) error {
	if path == "" {
		return fmt.Errorf("manifest path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	var buffer [32 << 10]byte
	var readErr error
	for {
		n, err := file.Read(buffer[:])
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			readErr = err
			break
		}
	}
	if err := errors.Join(readErr, file.Close()); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fixtureID(profileID, dbPath string) string {
	lowerProfile := strings.ToLower(profileID)
	switch {
	case strings.Contains(lowerProfile, "-mongoose-"):
		return "mongoose"
	case strings.Contains(lowerProfile, "-testdb-"):
		return "testdb"
	}
	base := strings.ToLower(filepath.Base(dbPath))
	switch base {
	case "test.db", "test.db.new", "test_conf.db", "test_run.db":
		return "testdb"
	case "mongoose.db", "mongoose.db.new":
		return "mongoose"
	default:
		return base
	}
}
