package conformance

// TestSuite represents a complete YAML test file
type TestSuite struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description,omitempty"`
	Requires    Requirements `yaml:"requires,omitempty"`
	Setup       *SetupBlock  `yaml:"setup,omitempty"`
	Teardown    *SetupBlock  `yaml:"teardown,omitempty"`
	Tests       []TestCase   `yaml:"tests"`
}

// Requirements specifies what features are needed for this test suite
type Requirements struct {
	Features []string `yaml:"features,omitempty"`
}

// SetupBlock contains setup or teardown code
type SetupBlock struct {
	Permission string `yaml:"permission,omitempty"` // programmer|wizard
	Code       string `yaml:"code,omitempty"`
	Statement  string `yaml:"statement,omitempty"`
	Verb       string `yaml:"verb,omitempty"`
}

// TestCase represents a single test within a suite
type TestCase struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Skip        interface{} `yaml:"skip,omitempty"`      // bool or string
	SkipIf      string      `yaml:"skip_if,omitempty"`
	Permission  string      `yaml:"permission,omitempty"` // programmer|wizard
	Code        string      `yaml:"code,omitempty"`       // expression (wrapped in return)
	Statement   string      `yaml:"statement,omitempty"`  // explicit statements
	Verb        string      `yaml:"verb,omitempty"`       // #0:verb_name
	Setup       *SetupBlock `yaml:"setup,omitempty"`
	Teardown    *SetupBlock `yaml:"teardown,omitempty"`
	Expect      Expectation `yaml:"expect"`
}

// Expectation defines what result is expected from a test
type Expectation struct {
	Value    interface{} `yaml:"value,omitempty"`    // exact match
	Error    string      `yaml:"error,omitempty"`    // E_TYPE, E_DIV, etc.
	Type     string      `yaml:"type,omitempty"`     // int, str, list, etc.
	Match    string      `yaml:"match,omitempty"`    // regex
	Contains interface{} `yaml:"contains,omitempty"` // list/string contains
	Range    []float64   `yaml:"range,omitempty"`    // min, max for floats
}

// IsSkipped returns true if this test should be skipped
func (tc *TestCase) IsSkipped() (bool, string) {
	// Check skip_if first - barn is a 64-bit implementation
	if tc.SkipIf != "" {
		// Skip tests that require 32-bit behavior
		if tc.SkipIf == "feature.64bit" || tc.SkipIf == "64bit" {
			return true, "skipped (64-bit implementation)"
		}
		// Run tests that require 64-bit behavior ("not feature.64bit" means skip on 32-bit only)
		// These tests should NOT be skipped on our 64-bit implementation
		if tc.SkipIf == "not feature.64bit" {
			// Don't skip - we are 64-bit
		}
	}

	if tc.Skip == nil {
		return false, ""
	}

	switch v := tc.Skip.(type) {
	case bool:
		if v {
			return true, "skipped"
		}
		return false, ""
	case string:
		return true, v
	default:
		return false, ""
	}
}
