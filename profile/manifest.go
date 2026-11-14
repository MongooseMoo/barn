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
	// feature.ast: the Mongoose fork's syntax-tree builtins; conformance tests
	// that need them declare `requires: features: [ast]`.
	features["feature.ast"] = astBuiltinsPresent(input.Registry)
	// Known differences from ToastStunt that Barn keeps on purpose; conformance
	// cases for Toast's behaviour declare `skip_if: "feature.<name>"`.
	for name := range KnownDifferences {
		features["feature."+name] = true
	}
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

// KnownDifferences names what Barn does differently from ToastStunt on
// purpose, each a conformance feature every Barn manifest reports.
var KnownDifferences = map[string]string{
	// parser/format.go prints `^' before a dot as `(^)': Toast's `l[^.x]' is a
	// syntax error on Toast and on Barn.
	"bracketed_first_index_before_property": "verb_code prints l[(^).x], not l[^.x]",
	// The formatter keeps the parsed program so formatted text reparses to the
	// same IR (docs/parser-fuzzing.md); Toast folds `- -5' to `5'.
	"unfolded_double_negation": "verb_code prints - -5 as --5, not 5",
	// The parser does not accept the fork's waif property spelling.
	"no_waif_colon_property": "the parser rejects x.:name",
}

// astBuiltinsPresent reports whether all four syntax-tree builtins are installed.
func astBuiltinsPresent(registry *builtins.Registry) bool {
	for _, name := range []string{"parse_ast", "verb_ast", "validate_ast", "unparse_ast"} {
		if !registry.Has(name) {
			return false
		}
	}
	return true
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
