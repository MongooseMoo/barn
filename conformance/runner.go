package conformance

import (
	"fmt"
)

// TestResult represents the outcome of running a single test
type TestResult struct {
	Test   LoadedTest
	Passed bool
	Skipped bool
	SkipReason string
	Error  error
}

// Runner executes conformance tests
type Runner struct {
	// In Phase 0, we have no interpreter yet
	// This will be populated in later phases
}

// NewRunner creates a new test runner
func NewRunner() *Runner {
	return &Runner{}
}

// Run executes a single test case
func (r *Runner) Run(test LoadedTest) TestResult {
	// Check if test should be skipped
	if skipped, reason := test.Test.IsSkipped(); skipped {
		return TestResult{
			Test:       test,
			Skipped:    true,
			SkipReason: reason,
		}
	}

	// For Phase 0, we skip all tests (no interpreter yet)
	return TestResult{
		Test:       test,
		Skipped:    true,
		SkipReason: "no interpreter (Phase 0)",
	}
}

// RunAll executes all loaded tests
func (r *Runner) RunAll(tests []LoadedTest) []TestResult {
	results := make([]TestResult, len(tests))
	for i, test := range tests {
		results[i] = r.Run(test)
	}
	return results
}

// SummaryStats computes statistics from test results
type SummaryStats struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
}

// ComputeStats generates statistics from test results
func ComputeStats(results []TestResult) SummaryStats {
	stats := SummaryStats{Total: len(results)}
	for _, r := range results {
		if r.Skipped {
			stats.Skipped++
		} else if r.Passed {
			stats.Passed++
		} else {
			stats.Failed++
		}
	}
	return stats
}

// FormatStats returns a human-readable summary
func FormatStats(stats SummaryStats) string {
	return fmt.Sprintf("%d passed, %d failed, %d skipped (%d total)",
		stats.Passed, stats.Failed, stats.Skipped, stats.Total)
}
