package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/parser"
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
	// CommandDispatch is true when the server has a listener whose
	// connections dispatch typed commands.
	CommandDispatch bool
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
	for name, value := range suiteFeatures(input) {
		features[name] = value
	}
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

// suiteFeatures derives the conformance feature names that whole suite files
// declare in `requires: features`. ToastStunt compiles every one of them in and
// lists none in server_version("features") (src/version.cc:141-146), so a
// profile manifest is the only place either engine states them, and Barn's
// MOO-visible server_version stays as it is.
func suiteFeatures(input BuildInput) map[string]any {
	registry := input.Registry
	_, readsToastCore := format.HeaderVersion(toastCoreFixtureHeader)
	// `fork' and `forking' (the fork statement) are not declared yet: rows
	// they gate fail on Barn (agent-notes/fix-run-skipped-conformance-rows.md,
	// "Names held back").
	return map[string]any{
		// `^' and `$' inside index and range expressions.
		"feature.index_extensions": parses("x = {1}; return {x[^], x[$], x[^..$]};"),
		// Anonymous objects are made only by create(parent, 1).
		"feature.anonymous": registry.Has("create"),
		// Task inspection and control.
		"feature.tasks":          allBuiltins(registry, "task_id", "queued_tasks", "kill_task", "suspend", "resume"),
		"feature.exec_support":   registry.Has("exec"),
		"feature.server_options": registry.Has("load_server_options"),
		// Typed commands reach verbs only through a listener's connections.
		"feature.command_dispatch": input.CommandDispatch,
		// The suite's ToastCore startup fixture is a Format Version 17 database.
		"feature.toastcore_fixture": readsToastCore,
	}
}

const toastCoreFixtureHeader = "** LambdaMOO Database, Format Version 17 **"

// parses reports whether Barn's parser accepts source.
func parses(source string) bool {
	_, err := parser.NewParser(source).ParseProgram()
	return err == nil
}

func allBuiltins(registry *builtins.Registry, names ...string) bool {
	for _, name := range names {
		if !registry.Has(name) {
			return false
		}
	}
	return true
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
