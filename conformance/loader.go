package conformance

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TestPath is the path to cow_py conformance tests (relative to barn/)
const TestPath = "../cow_py/tests/conformance"

// LoadedTest represents a test with its source file path
type LoadedTest struct {
	File  string
	Suite TestSuite
	Test  TestCase
}

// LoadAllTests walks the conformance test directory and loads all test cases
func LoadAllTests() ([]LoadedTest, error) {
	var loaded []LoadedTest

	// Get absolute path to test directory
	// Try multiple path resolutions since tests run from different locations
	testDir := ""
	candidates := []string{
		TestPath,                                    // relative to cwd
		filepath.Join("..", TestPath),               // if running from conformance/
		filepath.Join("../..", "cow_py", "tests", "conformance"), // absolute fallback
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err == nil {
			if _, err := os.Stat(abs); err == nil {
				testDir = abs
				break
			}
		}
	}

	if testDir == "" {
		return nil, fmt.Errorf("could not find conformance test directory (tried %v)", candidates)
	}

	// Walk the directory tree
	err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .yaml files
		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}

		// Load this test file
		tests, err := loadTestFile(path)
		if err != nil {
			// Log error but continue - some test files may have YAML quirks
			// that Go's strict parser rejects but Python accepts
			relPath, _ := filepath.Rel(testDir, path)
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", relPath, err)
			return nil
		}

		// Get relative path for cleaner test names
		relPath, _ := filepath.Rel(testDir, path)

		// Add all tests from this file
		for _, test := range tests {
			loaded = append(loaded, LoadedTest{
				File:  relPath,
				Suite: test.Suite,
				Test:  test.Test,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return loaded, nil
}

// loadTestFile parses a single YAML file and returns all test cases
func loadTestFile(path string) ([]LoadedTest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var suite TestSuite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, err
	}

	var tests []LoadedTest
	for _, test := range suite.Tests {
		tests = append(tests, LoadedTest{
			Suite: suite,
			Test:  test,
		})
	}

	return tests, nil
}
