package conformance

import (
	"fmt"
	"testing"
)

func TestConformance(t *testing.T) {
	// Load all test cases
	tests, err := LoadAllTests()
	if err != nil {
		t.Fatalf("Failed to load tests: %v", err)
	}

	if len(tests) == 0 {
		t.Fatal("No tests loaded")
	}

	// Create runner
	runner := NewRunner()

	// Run all tests
	results := runner.RunAll(tests)

	// Compute statistics
	stats := ComputeStats(results)

	// Group results by file for organized output
	fileGroups := make(map[string][]TestResult)
	for _, result := range results {
		fileGroups[result.Test.File] = append(fileGroups[result.Test.File], result)
	}

	// Run each test file as a subtest
	for file, fileResults := range fileGroups {
		t.Run(file, func(t *testing.T) {
			for _, result := range fileResults {
				testName := result.Test.Test.Name
				t.Run(testName, func(t *testing.T) {
					if result.Skipped {
						t.Skipf("Skipped: %s", result.SkipReason)
					} else if !result.Passed {
						if result.Error != nil {
							t.Errorf("Test failed: %v", result.Error)
						} else {
							t.Error("Test failed")
						}
					}
				})
			}
		})
	}

	// Print summary at the end
	t.Logf("\n=== Summary ===\n%s", FormatStats(stats))

	// For Phase 0, we expect 0 passed, all skipped
	if stats.Passed > 0 {
		t.Logf("Note: %d tests unexpectedly passed (interpreter not implemented)", stats.Passed)
	}
}

func TestLoadAllTests(t *testing.T) {
	tests, err := LoadAllTests()
	if err != nil {
		t.Fatalf("Failed to load tests: %v", err)
	}

	t.Logf("Loaded %d test cases from conformance suite", len(tests))

	// Verify we loaded the expected number of tests (approximately)
	// PLAN.md says 1,110 total tests
	if len(tests) < 1000 || len(tests) > 1200 {
		t.Errorf("Expected ~1110 tests, got %d", len(tests))
	}

	// Verify test structure
	if len(tests) > 0 {
		first := tests[0]
		if first.Test.Name == "" {
			t.Error("Test has no name")
		}
		if first.File == "" {
			t.Error("Test has no file path")
		}
	}

	// Count files
	files := make(map[string]bool)
	for _, test := range tests {
		files[test.File] = true
	}
	t.Logf("Found %d test files", len(files))

	// Expect around 27 YAML files per PLAN.md
	if len(files) < 20 || len(files) > 35 {
		t.Errorf("Expected ~27 test files, got %d", len(files))
	}

	// Print some files for debugging
	count := 0
	for file := range files {
		if count < 5 {
			t.Logf("  Example file: %s", file)
			count++
		}
	}
}

func TestYAMLParsing(t *testing.T) {
	// This test verifies that all YAML files parse without errors
	tests, err := LoadAllTests()
	if err != nil {
		t.Fatalf("YAML parsing failed: %v", err)
	}

	// Check for common YAML parsing issues
	for i, test := range tests {
		// Each test must have a name
		if test.Test.Name == "" {
			t.Errorf("Test %d in %s has no name", i, test.File)
		}

		// Each test must have an expectation
		if test.Test.Expect.Value == nil &&
			test.Test.Expect.Error == "" &&
			test.Test.Expect.Type == "" &&
			test.Test.Expect.Match == "" &&
			test.Test.Expect.Contains == nil {
			t.Errorf("Test %s in %s has no expectation", test.Test.Name, test.File)
		}

		// Each test must have code, statement, or verb
		if test.Test.Code == "" && test.Test.Statement == "" && test.Test.Verb == "" {
			t.Errorf("Test %s in %s has no code/statement/verb", test.Test.Name, test.File)
		}
	}

	t.Logf("All %d tests parsed successfully", len(tests))
}

// BenchmarkLoadAllTests measures test loading performance
func BenchmarkLoadAllTests(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := LoadAllTests()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Example of how to run a specific test file
func ExampleLoadAllTests() {
	tests, err := LoadAllTests()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Count tests by category
	categories := make(map[string]int)
	for _, test := range tests {
		// Extract category from file path (basic/, builtins/, etc.)
		category := "unknown"
		if len(test.File) > 0 {
			// Simple extraction - just use first path component
			for i, c := range test.File {
				if c == '/' || c == '\\' {
					category = test.File[:i]
					break
				}
			}
		}
		categories[category]++
	}

	fmt.Printf("Loaded %d tests\n", len(tests))
	fmt.Printf("Categories: %v\n", categories)
}
