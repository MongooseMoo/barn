package conformance

import "fmt"

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
